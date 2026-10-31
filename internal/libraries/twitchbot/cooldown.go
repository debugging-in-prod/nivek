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
