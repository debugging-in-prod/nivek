package twitchbot

import (
	"strings"
	"sync"
	"time"

	"github.com/gempir/go-twitch-irc/v4"
)

// Central command cooldowns. Every chat command (builtin or custom) is run
// through Bot.commandAllowed in the dispatch path before its handler runs, so a
// new command is covered just by being dispatched -- handlers never need their
// own throttle to stay spam-safe.
//
// Two windows apply to each (channel, trigger):
//
//   - global:   one use per window by anyone in that channel
//   - per-user: one use per window by the same chatter in that channel
//
// With command.cooldown_secs unset (0) the defaults below apply. A positive
// cooldown_secs is the operator's explicit pace and is used for both windows
// (still clamped up to cooldownFloor). The broadcaster and mods skip both
// windows but can never fire the same command in a channel more often than
// cooldownFloor. A command on cooldown is silently ignored -- replying "on
// cooldown" would itself be chat spam.
const (
	defaultGlobalCooldown  = 5 * time.Second
	defaultPerUserCooldown = 15 * time.Second
	// cooldownFloor is the minimum gap between two runs of one command in one
	// channel, for everyone (including broadcaster/mods).
	cooldownFloor = 2 * time.Second

	// cooldownPruneInterval bounds how often a check sweeps expired entries.
	cooldownPruneInterval = time.Minute
	// cooldownMaxEntries hard-caps each map so memory stays bounded even under
	// a flood of distinct chatters inside one window.
	cooldownMaxEntries = 50_000
)

// Triggers of the commands dispatched ahead of the word scan in
// handleWebhookMessage (they take free-form arguments).
const (
	newPromoTrigger  = "!newpromo"
	stalkTrigger     = "!stalk"
	translateTrigger = "!translate"
	banishTrigger    = "!banish"
)

// The intentional "^" echo (any chat word containing "^" makes the bot reply
// "^") is not a command, but it is a reply path, so it shares the central
// cooldown state under a dedicated key that no "!" trigger can collide with.
const (
	caretEchoKey = "^echo"
	// caretEchoCooldownSecs is one global window per channel. There is no
	// per-user window and no privileged bypass: mods and the broadcaster are
	// held to the same 10s as everyone else (stricter than cooldownFloor).
	caretEchoCooldownSecs = 10
)

// A bare !hangman (start a game / show the board) uses the normal command
// cooldown. A guess (!hangman <letter|word>) is a move in a game already on the
// board, and the 5s/15s command windows dropped most of them, so guesses skip
// those windows and instead share the central state under dedicated keys with
// two short windows, both of which must pass:
//
//   - per user (hangmanGuessUserCooldown): one chatter can't guess faster than
//     this in a channel;
//   - per channel (hangmanGuessChannelCooldown): the whole channel can't
//     trigger guess replies faster than this, whoever is guessing.
//
// The channel window is what bounds reply volume: senderLoop paces every send
// across all channels (a 1.5s sleep after each send in bot_send_message.go,
// ~0.67 msg/s total), so an unbounded guess rate would fill the say() queue and
// drop replies everywhere. No privileged bypass.
const (
	hangmanTrigger         = "!hangman"
	hangmanGuessKey        = "^hangman-guess"
	hangmanGuessChannelKey = "^hangman-guess-channel"

	hangmanGuessUserCooldown    = 3 * time.Second
	hangmanGuessChannelCooldown = 2 * time.Second
)

// cooldownWindows resolves the effective (global, per-user) windows for a
// command's configured cooldown_secs.
func cooldownWindows(cooldownSecs int) (global, perUser time.Duration) {
	if cooldownSecs <= 0 {
		return defaultGlobalCooldown, defaultPerUserCooldown
	}
	d := time.Duration(cooldownSecs) * time.Second
	if d < cooldownFloor {
		d = cooldownFloor
	}
	return d, d
}

type cooldownStamp struct {
	last    time.Time // when the command last ran
	expires time.Time // when this entry can no longer block anything
}

// commandCooldowns is the in-memory cooldown state. Safe for concurrent use
// (EventSub chat messages are dispatched on a worker pool). The zero value is
// not usable; build one with newCommandCooldowns. A nil *commandCooldowns
// allows everything (bare &Bot{} test fixtures).
type commandCooldowns struct {
	mu        sync.Mutex
	now       func() time.Time
	global    map[string]cooldownStamp // key: channel \x00 trigger
	user      map[string]cooldownStamp // key: channel \x00 trigger \x00 user
	lastPrune time.Time
	maxSize   int
}

func newCommandCooldowns() *commandCooldowns {
	return &commandCooldowns{
		now:     time.Now,
		global:  make(map[string]cooldownStamp),
		user:    make(map[string]cooldownStamp),
		maxSize: cooldownMaxEntries,
	}
}

// allow reports whether trigger may run now in channel for user, and if so
// records the use. channel and user should be stable ids (Twitch user ids);
// trigger is matched case-insensitively. privileged marks the broadcaster or a
// mod, who bypass both windows but not cooldownFloor.
func (c *commandCooldowns) allow(channel, trigger, user string, cooldownSecs int, privileged bool) bool {
	if c == nil {
		return true
	}
	trigger = strings.ToLower(trigger)
	globalWin, userWin := cooldownWindows(cooldownSecs)
	gKey := channel + "\x00" + trigger
	uKey := gKey + "\x00" + user

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.pruneLocked(now)

	g, gok := c.global[gKey]
	if gok {
		since := now.Sub(g.last)
		if since < cooldownFloor || (!privileged && since < globalWin) {
			return false
		}
	}
	if !privileged && user != "" {
		if u, ok := c.user[uKey]; ok && now.Sub(u.last) < userWin {
			return false
		}
	}

	c.global[gKey] = cooldownStamp{last: now, expires: now.Add(max(globalWin, cooldownFloor))}
	if !privileged && user != "" {
		c.user[uKey] = cooldownStamp{last: now, expires: now.Add(userWin)}
	}
	c.capLocked(c.global)
	c.capLocked(c.user)
	return true
}

// allowUserAndChannel gates a use on two windows at once: a per-user window
// (userKey, userWin; stored in c.user) and a channel-wide window (chanKey,
// chanWin; stored in c.global). No privileged bypass.
//
// Order and atomicity: under one hold of c.mu it first checks both windows
// without recording anything, and only if both pass records both stamps. So a
// use rejected by either window consumes neither (a guess dropped by the
// channel window doesn't start that user's window, and vice versa), and two
// concurrent callers can't both slip through.
func (c *commandCooldowns) allowUserAndChannel(channel, user, userKey string, userWin time.Duration, chanKey string, chanWin time.Duration) bool {
	if c == nil {
		return true
	}
	cKey := channel + "\x00" + strings.ToLower(chanKey)
	uKey := channel + "\x00" + strings.ToLower(userKey) + "\x00" + user

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.pruneLocked(now)

	// Check both first; record nothing on rejection.
	if g, ok := c.global[cKey]; ok && now.Sub(g.last) < chanWin {
		return false
	}
	if u, ok := c.user[uKey]; ok && now.Sub(u.last) < userWin {
		return false
	}

	// Both passed: record both.
	c.global[cKey] = cooldownStamp{last: now, expires: now.Add(chanWin)}
	c.user[uKey] = cooldownStamp{last: now, expires: now.Add(userWin)}
	c.capLocked(c.global)
	c.capLocked(c.user)
	return true
}

// pruneLocked drops expired entries at most once per cooldownPruneInterval.
func (c *commandCooldowns) pruneLocked(now time.Time) {
	if now.Sub(c.lastPrune) < cooldownPruneInterval {
		return
	}
	c.lastPrune = now
	for _, m := range []map[string]cooldownStamp{c.global, c.user} {
		for k, s := range m {
			if !now.Before(s.expires) {
				delete(m, k)
			}
		}
	}
}

// capLocked enforces the hard size cap. Only reachable under a pathological
// flood inside one window; evicting arbitrary entries trades a little cooldown
// precision for bounded memory.
func (c *commandCooldowns) capLocked(m map[string]cooldownStamp) {
	if c.maxSize <= 0 || len(m) <= c.maxSize {
		return
	}
	now := c.now()
	for k, s := range m {
		if !now.Before(s.expires) {
			delete(m, k)
		}
	}
	// Evict down to 90% of the cap so a sustained flood doesn't rescan the
	// whole map on every insert.
	target := c.maxSize - c.maxSize/10
	for k := range m {
		if len(m) <= target {
			break
		}
		delete(m, k)
	}
}

// size returns the number of tracked entries (tests/diagnostics).
func (c *commandCooldowns) size() (global, user int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.global), len(c.user)
}

// builtinCooldownSecs returns the configured cooldown_secs for a global
// builtin trigger (0 = use defaults, also for unknown triggers).
func (b *Bot) builtinCooldownSecs(trigger string) int {
	set := b.commands.Load()
	if set == nil {
		return 0
	}
	return set.cooldownSecs[strings.ToLower(trigger)]
}

// commandAllowed is the single cooldown gate for EventSub chat commands. Call it
// immediately before running any command handler or custom reply. A command
// that passes is also counted for per-channel usage analytics (in-memory only;
// see usage_tracker.go), so usage is recorded exactly for dispatched commands.
func (b *Bot) commandAllowed(message *chatMessageEvent, trigger string, cooldownSecs int) bool {
	if !b.cooldowns.allow(
		message.BroadcasterUserId,
		trigger,
		message.ChatterUserId,
		cooldownSecs,
		isModOrBroadcaster(message),
	) {
		return false
	}
	b.recordCommandUse(message.BroadcasterUserId, message.BroadcasterUserLogin, trigger)
	return true
}

// caretEchoAllowed is the cooldown gate for the "^" echo. It goes through the
// same commandCooldowns as commands but deliberately skips recordCommandUse:
// the echo is not a command, so it is not counted in usage stats.
func (b *Bot) caretEchoAllowed(message *chatMessageEvent) bool {
	return b.cooldowns.allow(message.BroadcasterUserId, caretEchoKey, "", caretEchoCooldownSecs, false)
}

// hangmanGateAllowed is the cooldown gate for !hangman. A bare !hangman
// (including one whose only argument is invisible runes) goes through
// commandAllowed like any other command. A guess must instead pass both the
// hangmanGuessUserCooldown per-user window and the hangmanGuessChannelCooldown
// per-channel window (checked together, recorded only if both pass; see
// allowUserAndChannel). Allowed guesses are still counted as !hangman usage.
func (b *Bot) hangmanGateAllowed(message *chatMessageEvent) bool {
	if !isHangmanGuess(message.Message.Text) {
		return b.commandAllowed(message, hangmanTrigger, b.builtinCooldownSecs(hangmanTrigger))
	}
	if !b.cooldowns.allowUserAndChannel(
		message.BroadcasterUserId,
		message.ChatterUserId,
		hangmanGuessKey, hangmanGuessUserCooldown,
		hangmanGuessChannelKey, hangmanGuessChannelCooldown,
	) {
		return false
	}
	b.recordCommandUse(message.BroadcasterUserId, message.BroadcasterUserLogin, hangmanTrigger)
	return true
}

// ircCommandAllowed is the IRC-path counterpart of commandAllowed, for the few
// commands still executed from IRC PRIVMSGs (the !banish fallback).
func (b *Bot) ircCommandAllowed(message twitch.PrivateMessage, trigger string) bool {
	if !b.cooldowns.allow(
		message.RoomID,
		trigger,
		message.User.ID,
		b.builtinCooldownSecs(trigger),
		message.User.IsBroadcaster || message.User.IsMod || message.User.ID == message.RoomID,
	) {
		return false
	}
	b.recordCommandUse(message.RoomID, message.Channel, trigger)
	return true
}
