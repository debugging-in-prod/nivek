package twitchbot

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
)

// Active channel list loading (issue #191, inconsistency B2).
//
// The channel list comes from core-api GET /bot/channels/active. It used to be
// a hard boot requirement: cmd/twitch-bot called log.Fatal when core-api was
// down or returned nothing, so systemd crash-looped the bot. It now follows the
// same pattern as the command loader (command_loader.go, #174):
//
//  1. At boot, retry with bounded exponential backoff (bootChannelBackoff,
//     ~2 min total), logging every attempt.
//  2. If every boot attempt fails (or the list is empty), start anyway with
//     only the permanent home channels (Config.ChannelsPending) and keep
//     retrying in a panic-recovered background goroutine. Once the list loads,
//     the channels are tracked and joined exactly as Start would have joined
//     them at boot. Nothing is said in chat on these joins.
//
// When core-api is healthy the first attempt succeeds and boot is unchanged.

// bootChannelBackoff is the wait before each boot-time retry after the first
// attempt: 1+2+4+8+16+30+30+30 = 121s worst case before the bot starts with
// only its home channels. Package vars so tests can shrink them.
var (
	bootChannelBackoff         = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second}
	backgroundChannelRetryBase = 1 * time.Second
	backgroundChannelRetryMax  = 30 * time.Second
	channelRetrySleep          = time.Sleep
)

// validTwitchLogin is the IRC-protocol shape of a Twitch login (4-25 chars,
// lowercase, [a-z0-9_]). The filter stays bot-side because it's an IRC concern,
// not something the API should gatekeep.
var validTwitchLogin = regexp.MustCompile(`^[a-z0-9_]{4,25}$`)

// fetchActiveChannels makes one attempt at loading the active channel list and
// filters it to valid Twitch logins. An empty result (before or after the
// filter) is an error so the caller keeps retrying. A panic inside the client
// is converted to an error so a retry loop survives.
func fetchActiveChannels(coreAPI api.CoreAPIClient) (channels []BotUser, err error) {
	defer func() {
		if r := recover(); r != nil {
			channels, err = nil, fmt.Errorf("panic fetching active channels: %v", r)
		}
	}()
	if coreAPI == nil {
		return nil, fmt.Errorf("no core-api client")
	}
	users, err := coreAPI.GetActiveChannels()
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, fmt.Errorf("no active users returned by core-api")
	}
	channels = make([]BotUser, 0, len(users))
	for _, u := range users {
		if u.TwitchLogin == nil || !validTwitchLogin.MatchString(*u.TwitchLogin) {
			continue
		}
		// hasPrivs is seeded later from Twitch's EventSub state (hydrateChatReadPrivs).
		channels = append(channels, BotUser{User: u})
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("no active users with valid Twitch logins returned by core-api")
	}
	return channels, nil
}

// LoadChannelsAtBoot fetches the active channel list, retrying with
// bootChannelBackoff. ok=false means every boot attempt failed: the caller
// should start with no tracked channels and set Config.ChannelsPending so Start
// keeps retrying in the background. It never exits the process.
func LoadChannelsAtBoot(coreAPI api.CoreAPIClient) (channels []BotUser, ok bool) {
	attempts := len(bootChannelBackoff) + 1
	for i := 0; i < attempts; i++ {
		channels, err := fetchActiveChannels(coreAPI)
		if err == nil {
			log.Printf("[CHANNELS] loaded %d active channels (attempt %d/%d)", len(channels), i+1, attempts)
			return channels, true
		}
		if i == attempts-1 {
			log.Printf("[CHANNELS] ERROR: unable to load active channels (attempt %d/%d): %s", i+1, attempts, err.Error())
			break
		}
		wait := bootChannelBackoff[i]
		log.Printf("[CHANNELS] ERROR: unable to load active channels (attempt %d/%d): %s; retrying in %s", i+1, attempts, err.Error(), wait)
		channelRetrySleep(wait)
	}
	log.Printf("[CHANNELS] ERROR: starting with only the permanent home channels; retrying core-api GET %s in the background", api.GetActiveChannels)
	return nil, false
}

// retryChannelsInBackground keeps retrying until the active channel list
// loads, then tracks and joins the channels and returns. It stops early when
// ctx is cancelled. Each attempt is panic-isolated by fetchActiveChannels; the
// loop itself is also guarded so a bug here can never take the process down.
func (b *Bot) retryChannelsInBackground(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[CHANNELS] ERROR: panic in background channel loader: %v", r)
		}
	}()
	wait := backgroundChannelRetryBase
	for attempt := 1; ; attempt++ {
		if ctx.Err() != nil {
			return
		}
		channelRetrySleep(wait)
		if ctx.Err() != nil {
			return
		}
		channels, err := fetchActiveChannels(b.coreAPI)
		if err == nil {
			n := b.addRecoveredChannels(ctx, channels)
			log.Printf("[CHANNELS] loaded %d active channels (background attempt %d); %d newly tracked", len(channels), attempt, n)
			return
		}
		wait = min(wait*2, backgroundChannelRetryMax)
		log.Printf("[CHANNELS] ERROR: background load failed (attempt %d): %s; retrying in %s", attempt, err.Error(), wait)
	}
}

// addRecoveredChannels merges a late-loaded channel list into config.Channels
// and starts each new channel the same way Start does at boot (join only if
// live, no chat message). Channels already tracked (e.g. added by !joinme while
// core-api was down) or with a !joinme in flight are skipped so nothing is
// tracked twice. Returns how many channels were added.
func (b *Bot) addRecoveredChannels(ctx context.Context, channels []BotUser) int {
	added := make([]BotUser, 0, len(channels))
	b.channelsMu.Lock()
	for _, c := range channels {
		if b.channelTrackedLocked(c) {
			continue
		}
		b.config.Channels = append(b.config.Channels, c)
		added = append(added, c)
	}
	b.channelsMu.Unlock()

	for _, c := range added {
		b.startTrackedChannel(c)
	}

	// The boot-time hydrate ran with an empty list; seed chat-read privileges
	// for the channels that just arrived. It only ever sets flags to true.
	if len(added) > 0 && b.twitchClient != nil {
		go b.hydrateChatReadPrivs(ctx)
	}
	return len(added)
}

// channelTrackedLocked reports whether c is already in config.Channels (by
// Twitch ID or login) or has a !joinme in flight. Caller holds channelsMu.
func (b *Bot) channelTrackedLocked(c BotUser) bool {
	if c.TwitchID != nil && b.joinInFlight[*c.TwitchID] {
		return true
	}
	for _, u := range b.config.Channels {
		if c.TwitchID != nil && u.TwitchID != nil && *u.TwitchID == *c.TwitchID {
			return true
		}
		if c.TwitchLogin != nil && u.TwitchLogin != nil && strings.EqualFold(*u.TwitchLogin, *c.TwitchLogin) {
			return true
		}
	}
	return false
}
