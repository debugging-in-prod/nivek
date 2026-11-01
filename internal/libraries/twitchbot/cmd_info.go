package twitchbot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/twitcheventsub"
)

// Info commands: !uptime, !followage [user], !accountage [user], !game, !title.
//
// They are ordinary builtins (builtinRegistry + nivek.command seed rows), so
// the central cooldown gate in handleWebhookMessage runs before any of these
// handlers. Each handler does at most a couple of Helix reads, bounded by
// infoLookupTimeout and cached for infoCacheTTL, and runs off the dispatch
// worker (see runInfoLookup) so a slow Helix can't stall a channel's chat.
// Every failure is logged and answered at most once per invocation; nothing
// here can stop the bot.

const (
	infoLookupTimeout = 5 * time.Second
	infoCacheTTL      = 60 * time.Second
	infoCacheMax      = 4096
	// infoMaxInFlight caps concurrent Helix lookups. A lookup that can't get a
	// slot is dropped (logged) instead of queueing behind a slow Helix.
	infoMaxInFlight = 8

	// followageUnavailableWindow throttles the "can't check" notice per
	// channel: a channel where the bot isn't a mod would otherwise answer every
	// (cooldown-passing) !followage with the same apology.
	followageUnavailableWindow = 10 * time.Minute
	followageUnavailableKey    = "!followage\x00unavailable"

	maxChatReplyLen = 450
)

const followageUnavailableMsg = "can't check follow age here (the bot needs to be a mod in this channel)"

// infoHelix is the slice of the Twitch client the info commands use. The
// production value is the bot's twitchClient; tests substitute a fake.
type infoHelix interface {
	GetStream(ctx context.Context, broadcasterID string) (twitcheventsub.StreamInfo, error)
	GetLastStreamEnd(ctx context.Context, broadcasterID string) (time.Time, bool, error)
	GetChannelInfo(ctx context.Context, broadcasterID string) (twitcheventsub.ChannelInfo, error)
	GetUserByLogin(ctx context.Context, login string) (twitcheventsub.UserInfo, bool, error)
	GetChannelFollow(ctx context.Context, userToken, broadcasterID, userID string) (twitcheventsub.FollowInfo, error)
}

// ---------------------------------------------------------------------------
// handlers

func (b *Bot) handleUptimeCommand(message *chatMessageEvent) {
	channelId := message.BroadcasterUserId
	channel := channelDisplayName(message)
	b.runInfoLookup("uptime", func(ctx context.Context) {
		s, err := b.cachedStream(ctx, channelId)
		if err != nil {
			log.Printf("[INFO] [%s] !uptime lookup failed: %v", message.BroadcasterUserLogin, err)
			b.sayInfo(channelId, fmt.Sprintf("couldn't check %s's uptime right now", channel))
			return
		}
		var lastEnd time.Time
		if !s.Live {
			// Best effort: any failure just drops the duration and falls
			// back to "<channel> is offline".
			end, found, err := b.cachedLastStreamEnd(ctx, channelId)
			if err != nil {
				log.Printf("[INFO] [%s] !uptime last-stream lookup failed: %v", message.BroadcasterUserLogin, err)
			} else if found {
				lastEnd = end
			}
		}
		b.sayInfo(channelId, uptimeReply(channel, s, lastEnd, b.infoClock()))
	})
}

func (b *Bot) handleGameCommand(message *chatMessageEvent) {
	channelId := message.BroadcasterUserId
	channel := channelDisplayName(message)
	b.runInfoLookup("game", func(ctx context.Context) {
		ci, err := b.cachedChannelInfo(ctx, channelId)
		if err != nil {
			log.Printf("[INFO] [%s] !game lookup failed: %v", message.BroadcasterUserLogin, err)
			b.sayInfo(channelId, fmt.Sprintf("couldn't look up %s's category right now", channel))
			return
		}
		b.sayInfo(channelId, gameReply(channel, ci))
	})
}

func (b *Bot) handleTitleCommand(message *chatMessageEvent) {
	channelId := message.BroadcasterUserId
	channel := channelDisplayName(message)
	b.runInfoLookup("title", func(ctx context.Context) {
		ci, err := b.cachedChannelInfo(ctx, channelId)
		if err != nil {
			log.Printf("[INFO] [%s] !title lookup failed: %v", message.BroadcasterUserLogin, err)
			b.sayInfo(channelId, fmt.Sprintf("couldn't look up %s's title right now", channel))
			return
		}
		b.sayInfo(channelId, titleReply(channel, ci))
	})
}

func (b *Bot) handleAccountAgeCommand(message *chatMessageEvent) {
	channelId := message.BroadcasterUserId
	asker := message.ChatterUserLogin
	target, ok := infoTargetLogin(message)
	if !ok {
		b.sayInfo(channelId, fmt.Sprintf("@%s that doesn't look like a username", asker))
		return
	}
	b.runInfoLookup("accountage", func(ctx context.Context) {
		u, found, err := b.cachedUser(ctx, target)
		if err != nil {
			log.Printf("[INFO] [%s] !accountage lookup of %s failed: %v", message.BroadcasterUserLogin, target, err)
			b.sayInfo(channelId, fmt.Sprintf("@%s couldn't look up %s right now", asker, target))
			return
		}
		if !found {
			b.sayInfo(channelId, fmt.Sprintf("@%s couldn't find a user named %s", asker, target))
			return
		}
		b.sayInfo(channelId, accountAgeReply(asker, u, b.infoClock()))
	})
}

func (b *Bot) handleFollowAgeCommand(message *chatMessageEvent) {
	channelId := message.BroadcasterUserId
	channel := channelDisplayName(message)
	asker := message.ChatterUserLogin
	target, ok := infoTargetLogin(message)
	if !ok {
		b.sayInfo(channelId, fmt.Sprintf("@%s that doesn't look like a username", asker))
		return
	}
	self := target == strings.ToLower(message.ChatterUserLogin)
	// Captured before the lookup moves to another goroutine.
	chatterId, chatterName := message.ChatterUserId, message.ChatterUserName

	b.runInfoLookup("followage", func(ctx context.Context) {
		userId, userName := chatterId, chatterName
		if !self || userId == "" {
			u, found, err := b.cachedUser(ctx, target)
			if err != nil {
				log.Printf("[INFO] [%s] !followage lookup of %s failed: %v", message.BroadcasterUserLogin, target, err)
				b.sayFollowageUnavailable(channelId)
				return
			}
			if !found {
				b.sayInfo(channelId, fmt.Sprintf("@%s couldn't find a user named %s", asker, target))
				return
			}
			userId, userName = u.ID, u.DisplayName
		}
		if userName == "" {
			userName = target
		}

		f, err := b.cachedFollow(ctx, channelId, userId)
		if err != nil {
			log.Printf("[INFO] [%s] !followage for %s unavailable: %v", message.BroadcasterUserLogin, target, err)
			b.sayFollowageUnavailable(channelId)
			return
		}
		b.sayInfo(channelId, followAgeReply(asker, userName, channel, f, b.infoClock()))
	})
}

// sayFollowageUnavailable posts the "can't check" notice at most once per
// followageUnavailableWindow per channel; later failures are silent.
func (b *Bot) sayFollowageUnavailable(channelId string) {
	if !b.cooldowns.allow(channelId, followageUnavailableKey, "", int(followageUnavailableWindow/time.Second), false) {
		return
	}
	b.sayInfo(channelId, followageUnavailableMsg)
}

// ---------------------------------------------------------------------------
// execution + caching

// runInfoLookup runs fn off the dispatch worker with a bounded context. If
// every slot is busy the lookup is dropped rather than queued. A panic in fn
// is recovered so it can never take the bot down. With no slot pool (bare
// test fixtures) fn runs inline.
func (b *Bot) runInfoLookup(name string, fn func(ctx context.Context)) {
	run := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[INFO] !%s panicked: %v", name, r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), infoLookupTimeout)
		defer cancel()
		fn(ctx)
	}
	if b.infoSlots == nil {
		run()
		return
	}
	select {
	case b.infoSlots <- struct{}{}:
	default:
		log.Printf("[INFO] !%s dropped: %d lookups already in flight", name, cap(b.infoSlots))
		return
	}
	go func() {
		defer func() { <-b.infoSlots }()
		run()
	}()
}

// infoClock is the info commands' clock: b.infoNow when set, else time.Now.
func (b *Bot) infoClock() time.Time {
	if b.infoNow != nil {
		return b.infoNow()
	}
	return time.Now()
}

var errNoHelix = errors.New("twitch client not configured")

func (b *Bot) cachedStream(ctx context.Context, broadcasterId string) (twitcheventsub.StreamInfo, error) {
	return cachedLookup(b.infoCache, "stream:"+broadcasterId, func() (twitcheventsub.StreamInfo, error) {
		if b.info == nil {
			return twitcheventsub.StreamInfo{}, errNoHelix
		}
		return b.info.GetStream(ctx, broadcasterId)
	})
}

type lastStreamEndLookup struct {
	end   time.Time
	found bool
}

// cachedLastStreamEnd returns when the channel's last stream ended (from its
// newest archive VOD); found is false when that's unknown.
func (b *Bot) cachedLastStreamEnd(ctx context.Context, broadcasterId string) (time.Time, bool, error) {
	r, err := cachedLookup(b.infoCache, "lastend:"+broadcasterId, func() (lastStreamEndLookup, error) {
		if b.info == nil {
			return lastStreamEndLookup{}, errNoHelix
		}
		end, found, err := b.info.GetLastStreamEnd(ctx, broadcasterId)
		return lastStreamEndLookup{end: end, found: found}, err
	})
	return r.end, r.found, err
}

func (b *Bot) cachedChannelInfo(ctx context.Context, broadcasterId string) (twitcheventsub.ChannelInfo, error) {
	return cachedLookup(b.infoCache, "channel:"+broadcasterId, func() (twitcheventsub.ChannelInfo, error) {
		if b.info == nil {
			return twitcheventsub.ChannelInfo{}, errNoHelix
		}
		return b.info.GetChannelInfo(ctx, broadcasterId)
	})
}

type userLookup struct {
	user  twitcheventsub.UserInfo
	found bool
}

func (b *Bot) cachedUser(ctx context.Context, login string) (twitcheventsub.UserInfo, bool, error) {
	r, err := cachedLookup(b.infoCache, "user:"+login, func() (userLookup, error) {
		if b.info == nil {
			return userLookup{}, errNoHelix
		}
		u, found, err := b.info.GetUserByLogin(ctx, login)
		return userLookup{user: u, found: found}, err
	})
	return r.user, r.found, err
}

func (b *Bot) cachedFollow(ctx context.Context, broadcasterId, userId string) (twitcheventsub.FollowInfo, error) {
	return cachedLookup(b.infoCache, "follow:"+broadcasterId+":"+userId, func() (twitcheventsub.FollowInfo, error) {
		if b.info == nil {
			return twitcheventsub.FollowInfo{}, errNoHelix
		}
		tok, err := b.helixUserToken(ctx)
		if err != nil {
			return twitcheventsub.FollowInfo{}, fmt.Errorf("%w: %w", twitcheventsub.ErrFollowsUnavailable, err)
		}
		return b.info.GetChannelFollow(ctx, tok, broadcasterId, userId)
	})
}

// infoCache is a small TTL cache of successful Helix responses. Errors are
// never cached. Safe for concurrent use; a nil *infoCache caches nothing.
type infoCache struct {
	mu      sync.Mutex
	now     func() time.Time
	ttl     time.Duration
	max     int
	entries map[string]infoCacheEntry
}

type infoCacheEntry struct {
	val     any
	expires time.Time
}

func newInfoCache(ttl time.Duration, max int) *infoCache {
	return &infoCache{now: time.Now, ttl: ttl, max: max, entries: make(map[string]infoCacheEntry)}
}

func (c *infoCache) get(key string) (any, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if !c.now().Before(e.expires) {
		delete(c.entries, key)
		return nil, false
	}
	return e.val, true
}

func (c *infoCache) put(key string, val any) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if len(c.entries) >= c.max {
		for k, e := range c.entries {
			if !now.Before(e.expires) {
				delete(c.entries, k)
			}
		}
		for k := range c.entries {
			if len(c.entries) < c.max {
				break
			}
			delete(c.entries, k)
		}
	}
	c.entries[key] = infoCacheEntry{val: val, expires: now.Add(c.ttl)}
}

func cachedLookup[T any](c *infoCache, key string, fetch func() (T, error)) (T, error) {
	if v, ok := c.get(key); ok {
		if t, ok := v.(T); ok {
			return t, nil
		}
	}
	t, err := fetch()
	if err != nil {
		return t, err
	}
	c.put(key, t)
	return t, nil
}

// ---------------------------------------------------------------------------
// reply text

// sayInfo sanitizes and queues one reply.
func (b *Bot) sayInfo(channelId, msg string) {
	if msg = sanitizeChatReply(msg); msg != "" {
		b.say(channelId, msg)
	}
}

// sanitizeChatReply makes text safe to post as a single chat line: control
// characters (newlines, tabs, NUL, ...) and invalid UTF-8 become spaces and
// all whitespace runs (including unicode spaces) collapse to one space;
// leading '/' or '.' are stripped so user/broadcaster-controlled text can never
// be read as a Twitch chat command such as "/ban" or ".timeout"; and the result
// is capped at maxChatReplyLen runes (ending in "…" when cut). An empty result
// means "send nothing".
func sanitizeChatReply(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == utf8.RuneError || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimLeftFunc(s, func(r rune) bool {
		return r == '/' || r == '.' || unicode.IsSpace(r)
	})
	if r := []rune(s); len(r) > maxChatReplyLen {
		s = strings.TrimRightFunc(string(r[:maxChatReplyLen-1]), unicode.IsSpace) + "…"
	}
	return s
}

func channelDisplayName(message *chatMessageEvent) string {
	if message.BroadcasterUserName != "" {
		return message.BroadcasterUserName
	}
	return message.BroadcasterUserLogin
}

// infoTargetLogin returns the login named after the command word, or the
// chatter's own login when none was given. ok is false for an argument that
// isn't a valid Twitch login.
func infoTargetLogin(message *chatMessageEvent) (string, bool) {
	arg := commandArg(message.Message.Text, message.matchedTrigger)
	if arg == "" {
		return strings.ToLower(message.ChatterUserLogin), message.ChatterUserLogin != ""
	}
	return normalizeStalkTarget(arg)
}

// commandArg returns the first word after trigger in text (case-insensitive),
// or "" when there is none. An empty trigger means the first word of text.
func commandArg(text, trigger string) string {
	fields := strings.Fields(text)
	for i, f := range fields {
		if trigger == "" || strings.EqualFold(f, trigger) {
			if i+1 < len(fields) {
				return fields[i+1]
			}
			return ""
		}
	}
	return ""
}

// uptimeReply answers !uptime. When offline, lastEnd (zero = unknown) is when
// the previous stream ended; an unknown or implausible (future) end time
// falls back to the plain "<channel> is offline".
func uptimeReply(channel string, s twitcheventsub.StreamInfo, lastEnd, now time.Time) string {
	if !s.Live || s.StartedAt.IsZero() {
		if lastEnd.IsZero() || lastEnd.After(now) {
			return fmt.Sprintf("%s is offline", channel)
		}
		return fmt.Sprintf("%s has been offline for %s", channel, formatDayClockDuration(now.Sub(lastEnd)))
	}
	return fmt.Sprintf("%s has been live for %s", channel, formatClockDuration(now.Sub(s.StartedAt)))
}

func gameReply(channel string, ci twitcheventsub.ChannelInfo) string {
	if strings.TrimSpace(ci.GameName) == "" {
		return fmt.Sprintf("%s has no category set", channel)
	}
	return fmt.Sprintf("%s is playing %s", channel, ci.GameName)
}

func titleReply(channel string, ci twitcheventsub.ChannelInfo) string {
	if strings.TrimSpace(ci.Title) == "" {
		return fmt.Sprintf("%s has no title set", channel)
	}
	return fmt.Sprintf("%s's title: %s", channel, ci.Title)
}

func accountAgeReply(asker string, u twitcheventsub.UserInfo, now time.Time) string {
	name := u.DisplayName
	if name == "" {
		name = u.Login
	}
	if u.CreatedAt.IsZero() {
		return fmt.Sprintf("@%s, couldn't tell when %s's account was created", asker, name)
	}
	return fmt.Sprintf("@%s, %s created their account %s ago", asker, name, formatCalendarAge(u.CreatedAt, now))
}

func followAgeReply(asker, user, channel string, f twitcheventsub.FollowInfo, now time.Time) string {
	if !f.Following || f.FollowedAt.IsZero() {
		return fmt.Sprintf("@%s, %s isn't following %s", asker, user, channel)
	}
	return fmt.Sprintf("@%s, %s has been following %s for %s", asker, user, channel, formatCalendarAge(f.FollowedAt, now))
}

// formatClockDuration renders an uptime: "1 hour 2 mins", "45 mins",
// "less than a minute". Days are folded into hours (streams rarely run that
// long and "26 hours" reads fine).
func formatClockDuration(d time.Duration) string {
	if d < time.Minute {
		return "less than a minute"
	}
	hours := int(d / time.Hour)
	mins := int((d % time.Hour) / time.Minute)
	var parts []string
	if hours > 0 {
		parts = append(parts, plural(hours, "hour", "hours"))
	}
	if mins > 0 {
		parts = append(parts, plural(mins, "min", "mins"))
	}
	return strings.Join(parts, " ")
}

// formatDayClockDuration is formatClockDuration with whole days split out:
// "2 days 3 hours 5 mins", "1 day", "15 hours 33 mins". Used for offline time,
// which (unlike a stream) routinely spans days.
func formatDayClockDuration(d time.Duration) string {
	const day = 24 * time.Hour
	days := int(d / day)
	if days == 0 {
		return formatClockDuration(d)
	}
	parts := []string{plural(days, "day", "days")}
	if rest := d % day; rest >= time.Minute {
		parts = append(parts, formatClockDuration(rest))
	}
	return strings.Join(parts, " ")
}

// formatCalendarAge renders the calendar distance from then to now as
// "1 year 4 months 13 days", dropping zero units. Under a day it falls back
// to hours/minutes.
func formatCalendarAge(then, now time.Time) string {
	then, now = then.UTC(), now.UTC()
	if !now.After(then) {
		return "less than a minute"
	}
	years, months, days := calendarDiff(then, now)
	if years == 0 && months == 0 && days == 0 {
		return formatClockDuration(now.Sub(then))
	}
	var parts []string
	if years > 0 {
		parts = append(parts, plural(years, "year", "years"))
	}
	if months > 0 {
		parts = append(parts, plural(months, "month", "months"))
	}
	if days > 0 {
		parts = append(parts, plural(days, "day", "days"))
	}
	return strings.Join(parts, " ")
}

// calendarDiff returns whole years, months and days from a to b (a <= b),
// stepping with AddDate so month lengths and leap years are handled by the
// time package (e.g. Jan 31 -> Mar 1 is 29 days, not a negative borrow).
func calendarDiff(a, b time.Time) (years, months, days int) {
	years = b.Year() - a.Year()
	if years > 0 && a.AddDate(years, 0, 0).After(b) {
		years--
	}
	t := a.AddDate(years, 0, 0)
	for months < 12 && !t.AddDate(0, months+1, 0).After(b) {
		months++
	}
	t = t.AddDate(0, months, 0)
	days = int(b.Sub(t) / (24 * time.Hour))
	return years, months, days
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
