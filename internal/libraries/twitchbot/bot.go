package twitchbot

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gempir/go-twitch-irc/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/hangman"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/twitcheventsub"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/user"
)

const botCreatorChannel = "timallenfanclubofficial"

type commandHandler func(b *Bot, message *chatMessageEvent)

// builtinRegistry is the compiled set of behaviors the bot can perform, keyed
// by the stable handler_key stored in nivek.command. This is the ONLY place a
// handler_key becomes code. Keys MUST match the handler_key column values
// seeded into the command table; a row whose handler_key is missing here is
// logged as an error and skipped by buildCommandMaps (never crashes the bot).
// Ship the handler before its seed row: a handler without a row stays inactive.
var builtinRegistry = map[string]commandHandler{
	"banish":      (*Bot).handleBanishCommand,
	"bread":       (*Bot).handleBreadCommand,
	"dad_roll":    (*Bot).handleDadCommand,
	"fish":        (*Bot).handleFishCommand,
	"join_me":     (*Bot).handleJoinCommand,
	"lurk":        (*Bot).handleLurkCommand,
	"pb_commands": (*Bot).handlePbCommandsCommand,
	"new_promo":   (*Bot).handleNewPromoCommand,
	"stalk":       (*Bot).handleStalkCommand,
	"hangman":     (*Bot).handleHangmanCommand,
	"so":          (*Bot).handleShoutoutCommand,
	"translate":   (*Bot).handleTranslateCommand,
	"uptime":      (*Bot).handleUptimeCommand,
	"followage":   (*Bot).handleFollowAgeCommand,
	"accountage":  (*Bot).handleAccountAgeCommand,
	"game":        (*Bot).handleGameCommand,
	"title":       (*Bot).handleTitleCommand,
	"quote":       (*Bot).handleQuoteCommand,
	"cmd":         (*Bot).handleCmdCommand,
}

type BotUser struct {
	user.User
	hasPrivs bool
}

// DefaultBotID is the chat bot's (peanutbudderbot) Twitch user id, used when
// Config.BotId is empty (env TWITCH_BOT_ID unset).
const DefaultBotID = twitcheventsub.DefaultBotUserID

type Config struct {
	BotUsername string
	// BotId is the bot's Twitch user id (env TWITCH_BOT_ID). Empty means
	// DefaultBotID; read it through botID().
	BotId           string
	BotOAuth        string
	ClientID        string    // Twitch app client id; Helix Send Chat Message requires it
	Channels        []BotUser // Changed from single Channel to multiple Channels
	StoragePath     string
	Timezone        string
	ExecutorWSURL   string // e.g. ws://192.168.1.X:8123/ws
	OverseerHmacKey string // hex-encoded HMAC key, shared with the executor

	// TokenProvider, when non-nil, supplies a fresh user access token (WITHOUT
	// the "oauth:" prefix) before each IRC (re)connect and as the fallback for
	// Helix Send Chat Message. Nil falls back to the static BotOAuth.
	TokenProvider func(context.Context) (string, error)

	// AppTokenProvider, when non-nil, supplies a Twitch app access token used
	// (in preference to the user token) for Helix Send Chat Message so the bot's
	// messages earn the Chat Bot Badge. Sends that the app token can't authorize
	// (channel hasn't granted channel:bot and the bot isn't a mod) fall back to
	// the user token. Nil keeps the user-token-only send path.
	AppTokenProvider func(context.Context) (string, error)

	// NoReplyLogins lists extra Twitch logins the bot never replies to or runs
	// commands for. They are added to the built-in defaultNoReplyLogins (see
	// const.go); the bot's own login is always ignored too.
	NoReplyLogins []string

	// ChannelsPending, when true, means the active channel list could not be
	// loaded from core-api at boot (Channels is empty or partial). Start then
	// joins only the permanent home channels and keeps retrying the list in the
	// background, joining the rest once it loads (see channel_loader.go). The
	// zero value keeps the normal boot.
	ChannelsPending bool

	// DeepLAPIKey is the DeepL key for !translate (env DEEPL_API_KEY). Empty
	// means !translate replies that translation isn't configured.
	DeepLAPIKey string

	// DiscordWebhookURL is where go-live notifications are posted (env
	// TWITCH_BOT_DISCORD_WEBHOOK). Empty disables them.
	DiscordWebhookURL string

	// WebhookListenAddress overrides the EventSub listener bind address (env
	// WEBHOOK_LISTEN_ADDRESS). Empty binds defaultWebhookListenAddress, falling
	// back to fallbackWebhookListenAddress if that isn't bindable.
	WebhookListenAddress string

	// EventSubSecret verifies EventSub delivery signatures (env
	// TWITCH_EVENTSUB_SECRET). Empty makes the listener reject every delivery.
	EventSubSecret string
}

// botID returns the bot's Twitch user id: BotId, or DefaultBotID when empty.
func (c Config) botID() string {
	if id := strings.TrimSpace(c.BotId); id != "" {
		return id
	}
	return DefaultBotID
}

type sayRequest struct {
	broadcasterId string
	message       string
	// announce routes the request through Helix Send Chat Announcement instead
	// of Send Chat Message, so Twitch renders it with the "Announcement" header.
	announce bool
}

type ircSayRequest struct {
	channel string
	message string
}

// Bot has no direct Postgres dependency. All persistent state goes through
// CoreAPIClient → HMAC-authed RPC → core-api → DB. That way a compromised Pi
// can't drain prod data: it only has bot-scoped API capability, not raw DB
// credentials.
type Bot struct {
	client         *twitch.Client
	config         Config
	counters       *CounterManager
	location       *time.Location
	coreAPI        api.CoreAPIClient
	httpClient     *http.Client
	twitchClient   twitcheventsub.TwitchEventSubClient
	overseerClient *overseer.Client
	sayQueue       chan sayRequest
	ircSayQueue    chan ircSayRequest

	// tokenProvider, when non-nil, returns a fresh IRC access token before each
	// (re)connect. See Config.TokenProvider.
	tokenProvider func(context.Context) (string, error)

	// appTokenProvider, when non-nil, returns a Twitch app access token for the
	// Helix Send Chat Message call (Chat Bot Badge). See Config.AppTokenProvider.
	appTokenProvider func(context.Context) (string, error)

	// channelsMu guards all access to config.Channels and the in-flight claim sets
	// below, so the self-heal and !joinme goroutines mutate the channel list
	// exactly once each and never race a concurrent read. healInFlight is keyed by
	// lowercased Twitch login; joinInFlight by Twitch user ID.
	channelsMu   sync.Mutex
	healInFlight map[string]bool
	joinInFlight map[string]bool

	// commands is the current global builtin set (trigger -> handler plus each
	// trigger's configured command.cooldown_secs, 0 / missing = default
	// windows). Read lock-free by the dispatch worker pool and swapped whole by
	// the background loader (see command_loader.go); nil = not loaded yet.
	// cooldowns is the central per-channel/per-user cooldown guard every
	// command dispatch goes through (see cooldown.go).
	commands  atomic.Pointer[commandSet]
	cooldowns *commandCooldowns

	// dadMu guards dadUsage, the per-stream/per-chatter !dad rate-limit counters.
	// Populated on stream.online, evicted on stream.offline (see dad_limit.go).
	// Keyed by lowercased channel login.
	dadMu    sync.Mutex
	dadUsage map[string]*dadStreamUsage

	autoShout map[string][]string

	// liveMu guards live, the set of channels currently broadcasting (keyed by
	// lowercased login). Seeded from IsLive at boot and flipped by the
	// stream.online/offline webhooks. The promo scheduler consults it so recurring
	// messages only post to live chats (see promoChannelLive in promo_gate.go).
	liveMu sync.Mutex
	live   map[string]bool

	// helixLive is the promo scheduler's fallback live check for channels with
	// no EventSub state: a Helix streams lookup cached ~60s. Errors → offline.
	helixLive *helixLiveCache

	// chatLines counts non-bot chat lines per channel for the promo activity
	// gate (see promo_gate.go). Thread-safe.
	chatLines *chatLineCounter

	hangmanGames map[string]*hangman.Game

	// translateMu guards translateCooldown, the per-chatter last-used timestamp
	// for !translate (keyed by Twitch user id). Throttles the paid DeepL API.
	translateMu       sync.Mutex
	translateCooldown map[string]time.Time

	// customMu guards customCommands, the per-channel custom ("channel"-scoped)
	// command sets. Outer key is the lowercased channel login (what handleMessage
	// dispatches on); inner key is the lowercased trigger. Loaded on stream.online
	// / boot-if-live, evicted on stream.offline (see custom_commands.go).
	customMu       sync.Mutex
	customCommands map[string]map[string]commands.Commands

	// stalkMu guards stalk, the per-channel !stalk watches. Outer key is the
	// lowercased channel login. A channel is present only when it has a
	// configured target — loaded on stream.online / boot-if-live, evicted on
	// stream.offline (see cmd_stalk.go). handleMessage records a last message
	// only for that target chatter, not for the rest of chat.
	stalkMu sync.Mutex
	stalk   map[string]*stalkWatch

	// seenChatIDs drops Twitch EventSub retries of the same channel.chat.message
	// (same event.message_id) so command handlers do not run twice.
	seenChatIDs *messageIDCache

	// seenIRCIDs drops duplicate deliveries of the same IRC PRIVMSG so the
	// chat-read nudge is sent once per received message.
	seenIRCIDs *messageIDCache

	// grantMu guards grantAt / grantInFlight, the per-channel throttle for
	// channel.chat.message subscribe attempts (not the nudge itself).
	grantMu       sync.Mutex
	grantAt       map[string]time.Time
	grantInFlight map[string]bool

	// noReply is the set of lowercased logins whose messages are ignored
	// entirely (other chat bots + the bot itself). See no_reply.go.
	noReply map[string]struct{}

	// info is the Helix reader behind !uptime/!followage/!accountage/!game/
	// !title (normally twitchClient; nil makes those commands reply "couldn't
	// check"). infoCache holds ~60s of successful responses and infoSlots
	// bounds concurrent lookups (see cmd_info.go). infoNow is the clock the
	// replies measure ages against (nil means time.Now; tests pin it).
	info      infoHelix
	infoCache *infoCache
	infoSlots chan struct{}
	infoNow   func() time.Time

	// usage counts dispatched commands per (channel, trigger, day) for the
	// usage analytics; flushed to core-api in the background (usage_tracker.go).
	usage *usageTracker
}

func NewBot(
	coreAPI api.CoreAPIClient,
	twitchClient twitcheventsub.TwitchEventSubClient,
	config Config,
) (*Bot, error) {

	// Load timezone
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %s: %w", config.Timezone, err)
	}

	// Create counter manager
	counters, err := NewCounterManager(config.StoragePath, loc)
	if err != nil {
		return nil, fmt.Errorf("failed to create counter manager: %w", err)
	}

	// overseer (DF Twitch-plays) WebSocket client to the executor
	hmacKey, err := hex.DecodeString(config.OverseerHmacKey)
	if err != nil {
		return nil, fmt.Errorf("invalid OVERSEER_HMAC_KEY hex: %w", err)
	}
	overseerCli := overseer.NewClient(config.ExecutorWSURL, hmacKey)

	// Create Twitch IRC client
	client := twitch.NewClient(config.BotUsername, config.BotOAuth)
	client.IrcAddress = "irc.chat.twitch.tv:6697"
	client.TLS = true

	bot := &Bot{
		client:           client,
		config:           config,
		counters:         counters,
		location:         loc,
		coreAPI:          coreAPI,
		httpClient:       &http.Client{Timeout: 10 * time.Second},
		twitchClient:     twitchClient,
		overseerClient:   overseerCli,
		tokenProvider:    config.TokenProvider,
		appTokenProvider: config.AppTokenProvider,
		healInFlight:     make(map[string]bool),
		joinInFlight:     make(map[string]bool),
		cooldowns:        newCommandCooldowns(),

		dadUsage:          make(map[string]*dadStreamUsage),
		hangmanGames:      make(map[string]*hangman.Game),
		translateCooldown: make(map[string]time.Time),
		live:              make(map[string]bool),
		chatLines:         newChatLineCounter(),
		customCommands:    make(map[string]map[string]commands.Commands),
		stalk:             make(map[string]*stalkWatch),
		seenChatIDs:       newMessageIDCache(chatMessageIDCacheSize),
		seenIRCIDs:        newMessageIDCache(chatMessageIDCacheSize),
		grantAt:           make(map[string]time.Time),
		grantInFlight:     make(map[string]bool),
		noReply:           buildNoReplySet(config.NoReplyLogins, config.BotUsername),
		infoCache:         newInfoCache(infoCacheTTL, infoCacheMax),
		infoSlots:         make(chan struct{}, infoMaxInFlight),
		usage:             newUsageTracker(),
	}
	if twitchClient != nil {
		bot.info = twitchClient
	}

	// Global builtin commands: retry at boot, and if core-api is still down,
	// start with none and keep retrying in the background (incident #172).
	if !bot.loadCommandsAtBoot() {
		go bot.retryCommandsInBackground()
	}

	var liveLookup func(context.Context, string) (bool, error)
	if twitchClient != nil {
		liveLookup = twitchClient.IsStreamLive
	}
	bot.helixLive = newHelixLiveCache(liveLookup)

	bot.sayQueue = make(chan sayRequest, 64)
	bot.ircSayQueue = make(chan ircSayRequest, 64)
	go bot.ircsenderLoop()
	go bot.senderLoop()

	// Register message handler
	client.OnPrivateMessage(bot.handleMessage)

	client.OnNoticeMessage(func(m twitch.NoticeMessage) {
		log.Printf("[NOTICE] [%s] msg-id=%s: %s", m.Channel, m.MsgID, m.Message)
	})

	// Log connection events
	client.OnConnect(func() {
		log.Printf("Connected to Twitch IRC as %s", config.BotUsername)
	})

	return bot, nil
}

func (b *Bot) Start(ctx context.Context) error {
	b.autoShout = make(map[string][]string, len(b.config.Channels))
	for _, channel := range b.config.Channels {
		b.startTrackedChannel(channel)
	}

	// The creator's channel and the bot's own channel are permanent: always
	// present, online or offline, and never departed/banished. Join is idempotent.
	b.client.Join(botCreatorChannel)
	b.client.Join(strings.ToLower(b.config.BotUsername))

	// core-api was unreachable at boot: keep retrying the channel list and join
	// the rest once it loads. Started after the loop above so it never races
	// the boot-time read of config.Channels.
	if b.config.ChannelsPending {
		go b.retryChannelsInBackground(ctx)
	}

	// Start reset timer
	go b.counters.StartResetTimer(ctx)

	// setup webhook listener
	NewWebhookListener(b)

	// Seed chat-read privileges from Twitch's live EventSub state so the "mod me"
	// nudge stays quiet in channels that already granted it. Off the boot path.
	go b.hydrateChatReadPrivs(ctx)

	// Start the DF welcome/orientation announcer in dfCommandChannel.
	// go b.runDFWelcomeLoop(ctx) // temporarily disabled while bot is migrated from Pi -> VPS

	go b.runPromotionMessageLoop(ctx)

	// Ship per-channel command-usage counters to core-api every ~60s. Off the
	// message path; failures are logged and dropped (usage_tracker.go).
	go b.usage.run(ctx, usageFlushInterval, b.usageSinkFn())

	// Start IRC client with panic recovery and auto-reconnect
	go b.connectWithPanicRecovery(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}

// startTrackedChannel seeds one tracked channel's live state and, if it is
// live, joins it over IRC and loads its per-stream state. Used for every
// channel at boot (Start) and for channels that arrive late because core-api
// was down at boot (addRecoveredChannels). It never sends a chat message.
func (b *Bot) startTrackedChannel(channel BotUser) {
	if channel.TwitchLogin != nil {
		// Seed live-state so the promo scheduler knows which channels are
		// broadcasting from the moment it starts, before any webhook fires.
		b.setLive(*channel.TwitchLogin, channel.IsLive)
	}
	if channel.TwitchLogin != nil && channel.IsLive {
		b.client.Join(*channel.TwitchLogin)
		log.Printf("Joining channel: %s", *channel.TwitchLogin)

		if channel.TwitchID != nil {
			go b.fetchAutoShoutChatters(channel.TwitchID, channel.TwitchLogin)
			// Restore per-stream !dad counters so a restart mid-stream doesn't
			// hand every chatter a fresh allotment.
			go b.rehydrateDadUsage(*channel.TwitchLogin, *channel.TwitchID)
			// This channel is already live at boot, so load its custom commands
			// now — a mid-stream restart won't get a fresh stream.online webhook.
			go b.loadCustomCommands(*channel.TwitchID, *channel.TwitchLogin)
		}
		// Same live-session load as custom commands: pull this channel's
		// !stalk target if one is configured.
		go b.loadStalkTarget(*channel.TwitchLogin)
	}
}

func (b *Bot) handleMessage(message twitch.PrivateMessage) {
	if b.isBotIRCUser(message) || b.isNoReplyLogin(message.User.Name) || b.isChatReadNudge(message.Message) {
		return
	}
	if message.ID != "" && b.seenIRCIDs != nil && b.seenIRCIDs.seen(message.ID) {
		return
	}

	// Count toward the promo activity gate. Channels that granted chat-read get
	// the same message via EventSub (counted there), so only count IRC lines
	// where EventSub won't deliver them — never double-count.
	if !b.channelHasPrivs(message.Channel) {
		b.recordChatLine(message.Channel, message.User.Name)
	}

	msg := strings.TrimSpace(strings.ToLower(message.Message))
	if isBanishCommand(msg) {
		// EventSub already handles !banish in channels that have granted chat-read.
		if !b.channelHasPrivs(message.Channel) && b.ircCommandAllowed(message, banishTrigger) {
			b.handleIRCBanish(message)
		}
		return
	}

	// IRC no longer executes other commands. One nudge per received PRIVMSG in a
	// channel that has not granted chat-read. Home channels are skipped.
	if b.isPermanentChannel(message.Channel) || b.channelHasPrivs(message.Channel) {
		return
	}

	// Check for commands
	commandSeen := false
	for msgword := range strings.SplitSeq(msg, " ") {
		if _, ok := b.lookupCommand(msgword); ok {
			commandSeen = true
			continue
		}
		// Per-channel custom commands (loaded while the channel is live). A global
		// builtin with the same trigger wins — handled above via continue — so a
		// channel can't shadow a builtin in v1.
		if cmd, ok := b.customCommandFor(message.Channel, msgword); ok {
			if cmd.ResponseTmpl != nil {
				commandSeen = true
			}
		}
	}

	if commandSeen {
		b.enqueueChatReadNudge(message.Channel)
		go b.tryGrantChatRead(message.Channel)
	}
}

func (b *Bot) ircsenderLoop() {
	for req := range b.ircSayQueue {
		b.client.Say(req.channel, req.message)
		time.Sleep(1500 * time.Millisecond)
	}
}

func (b *Bot) ircsay(channel, message string) {
	b.ircSayQueue <- ircSayRequest{channel, message}
}

// promoPollInterval is how often the scheduler re-reads the promo set from
// core-api and evaluates which are due. It bounds both how quickly a newly
// created/edited promo takes effect and the scheduling granularity, so it should
// stay well below the minimum promo interval (60s).
const promoPollInterval = 30 * time.Second

// runPromotionMessageLoop drives every recurring "promo" message. The DB (via
// core-api) is the single source of truth: each tick we fetch all enabled promos
// and post the ones that are due. Per-promo timing and chat-line marks are held
// in memory (promoSchedule) rather than the DB — losing them on restart only
// means each promo waits one fresh interval, never a burst.
//
// A promo posts only when its interval has elapsed, its channel is live, and
// enough non-bot chat happened since its last post (see promo_gate.go). While a
// channel is offline its clock is continuously reset, so promos start a fresh
// interval when the stream comes online instead of firing a backlog.
func (b *Bot) runPromotionMessageLoop(ctx context.Context) {
	ticker := time.NewTicker(promoPollInterval)
	defer ticker.Stop()

	st := newPromoSchedule()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			promos, err := b.coreAPI.GetActivePromos()
			if err != nil {
				log.Printf("[PROMO] failed to fetch promos: %v", err)
				continue
			}
			b.evaluatePromos(ctx, now, promos, st)
		}
	}
}

// isPermanentChannel reports whether the bot must always remain in the given
// channel (case-insensitive login match): the creator's channel and the bot's
// own channel. These are joined at boot regardless of live state, never departed
// on go-offline, and can never be banished.
func (b *Bot) isPermanentChannel(channel string) bool {
	channel = strings.ToLower(channel)
	return channel == botCreatorChannel || channel == strings.ToLower(b.config.BotUsername)
}

// isTrackedChannel reports whether login is currently tracked in config.Channels
// (an opted-in or !joinme'd channel). Used to gate go-live joins so a banished
// (opted-out, removed) channel is not rejoined when its lingering stream.online
// subscription fires.
func (b *Bot) isTrackedChannel(login string) bool {
	login = strings.ToLower(login)
	b.channelsMu.Lock()
	defer b.channelsMu.Unlock()
	for i := range b.config.Channels {
		c := b.config.Channels[i]
		if c.TwitchLogin != nil && *c.TwitchLogin == login {
			return true
		}
	}
	return false
}

// setLive records whether a channel (by lowercased login) is currently
// broadcasting. Called from the stream.online/offline webhooks and at boot.
func (b *Bot) setLive(login string, live bool) {
	login = strings.ToLower(login)
	b.liveMu.Lock()
	b.live[login] = live
	b.liveMu.Unlock()
}

// mentionsBot reports whether the message text @-mentions the bot by username.
func mentionsBot(message, botUsername string) bool {
	return strings.Contains(strings.ToLower(message), "@"+strings.ToLower(botUsername))
}

// isModOrBroadcaster reports whether the sender may run mod/broadcaster-gated
// commands in the channel the message came from.
func isModOrBroadcaster(message *chatMessageEvent) bool {
	if message.BroadcasterUserId == message.ChatterUserId {
		return true
	}

	for _, badge := range message.Badges {
		if badge.SetId == "moderator" {
			return true
		}
	}

	return false
}

func (b *Bot) connectWithPanicRecovery(ctx context.Context) {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("Recovered from Twitch IRC panic: %v", r)
				}
			}()
			// Refresh the IRC token before (re)connecting so an expired access
			// token is renewed automatically. On failure we log and connect with
			// whatever token the client already holds — never worse than before.
			if b.tokenProvider != nil {
				if tok, err := b.tokenProvider(ctx); err != nil {
					log.Printf("token refresh failed, using existing IRC token: %v", err)
				} else {
					b.client.SetIRCToken("oauth:" + tok)
				}
			}
			if err := b.client.Connect(); err != nil {
				log.Printf("Error connecting to Twitch: %v", err)
			}
		}()

		select {
		case <-ctx.Done():
			return
		default:
			log.Println("Reconnecting to Twitch IRC in 5 seconds...")
			time.Sleep(5 * time.Second)
		}
	}
}

func (b *Bot) Stop() {
	log.Println("Disconnecting from Twitch...")
	b.client.Disconnect()

	// Save counters one last time
	if err := b.counters.Save(); err != nil {
		log.Printf("Error saving counters on shutdown: %v", err)
	}
}

func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// abs returns |n|. Used by dfSuccessReply when formatting a Mine
// action's dimensions for chat — Region.Max can be either >= or < Min
// depending on which corner the chatter typed first. Duplicated rather
// than imported because the overseer package's copy is unexported and
// the helper is too small to justify exposing or sharing.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
