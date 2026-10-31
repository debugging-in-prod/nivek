package user

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/coreconfig"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/botclient"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/twitcheventsub"
	userlib "github.com/tim-the-toolman-taylor/nivek/internal/libraries/user"
)

// botOptInSyncTimeout bounds the background Twitch calls that follow a toggle,
// so a slow upstream can't leak goroutines (bot calls have their own 10s cap).
const botOptInSyncTimeout = 30 * time.Second

// permanentChannels mirrors twitchbot.isPermanentChannel (creator + bot's own
// channel): the bot never leaves these, so they can't be opted out either —
// same as !banish, which refuses there.
var permanentChannels = map[string]bool{
	"timallenfanclubofficial": true,
	"peanutbudderbot":         true,
}

// botSubscriptionTypes are the EventSub types the bot itself depends on (see
// twitcheventsub.SubscribeToAllWebhooks). Opt-out removes only these, leaving
// e.g. overlay relay subscriptions untouched.
var botSubscriptionTypes = map[string]bool{
	"stream.online":        true,
	"stream.offline":       true,
	"channel.chat.message": true,
}

// NewPutBotOptInEndpoint lets the logged-in broadcaster opt the bot in or out of
// their own chat. It is scoped to the session user (no login/id in the body),
// so a user can only ever change their own channel. bot_opt_in in the DB is the
// authoritative state and is written synchronously; Twitch subscriptions and
// the bot's realtime join/part follow best-effort in the background.
//
// Opt-in mirrors a fresh signup (and overrides a prior !banish): re-create the
// bot's EventSub subscriptions and, only if the channel is live right now, ask
// the bot to join. That join is a bare IRC JOIN — nothing is posted.
//
// Opt-out mirrors !banish minus the goodbye message: remove the bot's EventSub
// subscriptions and ask the bot to depart and stop tracking the channel.
func NewPutBotOptInEndpoint(svc nivek.NivekService) echo.HandlerFunc {
	return func(c echo.Context) error {
		u, ok := c.Get("user").(*userlib.User)
		if !ok || u == nil {
			return echo.NewHTTPError(http.StatusUnauthorized, "missing authenticated user")
		}
		if u.TwitchLogin == nil || strings.TrimSpace(*u.TwitchLogin) == "" || u.TwitchID == nil || *u.TwitchID == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "no linked twitch account"})
		}

		var req struct {
			OptedIn *bool `json:"opted_in"`
		}
		if err := c.Bind(&req); err != nil || req.OptedIn == nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "missing opted_in"})
		}

		login := strings.ToLower(strings.TrimSpace(*u.TwitchLogin))
		twitchID := *u.TwitchID
		optIn := *req.OptedIn

		if !optIn && permanentChannels[login] {
			return c.JSON(http.StatusConflict, map[string]string{"error": "the bot can't leave this channel"})
		}

		if err := userlib.NewService(svc).SetBotOptInByID(u.Id, optIn); err != nil {
			svc.Logger().Errorf("bot opt-in: update %s to %v failed: %s", login, optIn, err.Error())
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save opt-in"})
		}

		go reconcileBotOptIn(svc, u.Id, login, twitchID)

		c.Response().Header().Set(echo.HeaderCacheControl, "no-store")
		return c.JSON(http.StatusOK, map[string]bool{"opted_in": optIn})
	}
}

// reconcileLocks serializes background syncs per user id so rapid on/off
// toggles can't interleave their Twitch/bot calls.
var reconcileLocks sync.Map // int -> *sync.Mutex

// reconcileBotOptIn brings Twitch subscriptions and the bot in line with the
// user's bot_opt_in as stored *now* (not as requested), so whichever sync runs
// last always matches the final DB state even if toggles race.
func reconcileBotOptIn(svc nivek.NivekService, userID int, login, twitchID string) {
	logger := svc.Logger()
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("bot opt-in sync panic: %v", r)
		}
	}()

	mu, _ := reconcileLocks.LoadOrStore(userID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	current, err := userlib.NewService(svc).GetUserById(userID)
	if err != nil || current == nil {
		logger.Errorf("bot opt-in sync: reload user %s failed: %v", login, err)
		return
	}

	cfg, _ := svc.CustomConfig().(coreconfig.CoreAPIConfig)
	ctx, cancel := context.WithTimeout(context.Background(), botOptInSyncTimeout)
	defer cancel()
	if current.BotOptIn {
		syncOptIn(ctx, cfg, logger, login, twitchID)
	} else {
		syncOptOut(ctx, cfg, logger, login, twitchID)
	}
}

func newEventSubClient(cfg coreconfig.CoreAPIConfig) (twitcheventsub.TwitchEventSubClient, error) {
	return twitcheventsub.NewClient(twitcheventsub.Config{
		ClientID:       cfg.TwitchClientID,
		ClientSecret:   cfg.TwitchClientSecret,
		EventSubSecret: cfg.TwitchEventSubSecret,
		CallbackURL:    cfg.TwitchEventSubCallbackURL,
	})
}

// newBotClient returns nil (realtime join/part skipped) when BOT_API_HMAC_KEY
// is not configured; the DB flag still governs the bot's next boot/go-live.
func newBotClient(cfg coreconfig.CoreAPIConfig, logger *logrus.Logger) *botclient.Client {
	if cfg.BotAPIHMACKey == "" {
		logger.Warn("bot opt-in: BOT_API_HMAC_KEY not set; skipping realtime bot join/part")
		return nil
	}
	bc, err := botclient.NewClient(cfg.BotInternalURL, cfg.BotAPIHMACKey)
	if err != nil {
		logger.Errorf("bot opt-in: create bot client: %s", err.Error())
		return nil
	}
	return bc
}

func syncOptIn(ctx context.Context, cfg coreconfig.CoreAPIConfig, logger *logrus.Logger, login, twitchID string) {
	if cfg.TwitchEventSubCallbackURL == "" {
		logger.Warn("bot opt-in: TWITCH_EVENTSUB_CALLBACK_URL is not configured; skipping eventsub subscription")
		return
	}
	es, err := newEventSubClient(cfg)
	if err != nil {
		logger.Errorf("bot opt-in: create eventsub client: %s", err.Error())
		return
	}
	// Idempotent: already-existing subscriptions are reported, not duplicated.
	es.SubscribeToAllWebhooks(ctx, login, twitchID)

	// Offline channels are joined by the normal stream.online flow. A channel
	// that is already live won't get that webhook, so join now, as at boot.
	live, err := es.IsStreamLive(ctx, twitchID)
	if err != nil {
		logger.Warnf("bot opt-in: live check for %s failed: %s", login, err.Error())
		return
	}
	if !live {
		return
	}
	if bc := newBotClient(cfg, logger); bc != nil {
		if err := bc.JoinChannel(login); err != nil {
			logger.Warnf("bot opt-in: join %s failed: %s", login, err.Error())
		}
	}
}

func syncOptOut(ctx context.Context, cfg coreconfig.CoreAPIConfig, logger *logrus.Logger, login, twitchID string) {
	if bc := newBotClient(cfg, logger); bc != nil {
		if err := bc.PartChannel(login); err != nil {
			logger.Warnf("bot opt-out: part %s failed: %s", login, err.Error())
		}
	}

	es, err := newEventSubClient(cfg)
	if err != nil {
		logger.Errorf("bot opt-out: create eventsub client: %s", err.Error())
		return
	}
	subs, err := es.ListEventSubSubscriptions(ctx)
	if err != nil {
		logger.Errorf("bot opt-out: list eventsub for %s failed: %s", login, err.Error())
		return
	}
	for _, s := range subs {
		if !isBotSubscriptionFor(s, twitchID, cfg.TwitchEventSubCallbackURL) {
			continue
		}
		if err := es.DeleteEventSubSubscription(ctx, s.ID); err != nil {
			logger.Warnf("bot opt-out: delete eventsub %s (%s) for %s failed: %s", s.ID, s.Type, login, err.Error())
		}
	}
}

// isBotSubscriptionFor reports whether s is one of the bot's own subscriptions
// for broadcaster twitchID. When the bot callback URL is configured, the
// subscription must also target it.
func isBotSubscriptionFor(s twitcheventsub.EventSubSubscription, twitchID, callbackURL string) bool {
	if s.Condition.BroadcasterUserID != twitchID || !botSubscriptionTypes[s.Type] {
		return false
	}
	return callbackURL == "" || s.Transport.Callback == callbackURL
}
