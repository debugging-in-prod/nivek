package twitchbot

import (
	"fmt"
	"log"
	"strings"
)

const shoutoutNeedsTarget = "a shoutout needs a target — usage: !so <username>"

// shoutout fires the Twitch announcement (Helix Send Chat Announcement) for a
// channel shout. It is the single source of the shoutout wording so the manual
// !so command and the auto-shout system always say exactly the same thing.
// target must be a bare, lowercase Twitch login. Prerequisites (same as
// auto-shout): the bot's user token holds moderator:manage:announcements and the
// bot is a moderator in the channel.
func (b *Bot) shoutout(channelId, target string) {
	b.announce(channelId, fmt.Sprintf("Follow @%s over at twitch.tv/%s!", target, target))
}

// handleShoutoutCommand implements !so <username>: fire the same announcement
// the auto-shout system sends, for a manually named channel. Dispatched through
// the generic builtin loop (registered as "so" in builtinRegistry), like every
// other builtin. Mod/broadcaster only, matching the other channel-management
// builtins (!banish, !newpromo). A bare "!so" with no target is rejected -- a
// shoutout requires a target.
func (b *Bot) handleShoutoutCommand(message *chatMessageEvent) {
	channelId := message.BroadcasterUserId
	username := message.ChatterUserLogin

	if !isModOrBroadcaster(message) {
		b.say(channelId, fmt.Sprintf("@%s only a mod or the broadcaster can give a shoutout", username))
		return
	}

	raw := strings.TrimSpace(message.Message.Text)
	args := ""
	if idx := strings.IndexAny(raw, " \t"); idx != -1 {
		args = strings.TrimSpace(raw[idx+1:])
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		b.say(channelId, fmt.Sprintf("@%s %s", username, shoutoutNeedsTarget))
		return
	}

	// Normalize to a bare lowercase login so the @mention and twitch.tv URL are
	// valid whether the mod typed "Stan", "@Stan", or "stan".
	target := strings.ToLower(strings.TrimPrefix(fields[0], "@"))

	b.shoutout(channelId, target)
	log.Printf("[Shoutout] !so given to %s in %s by %s", target, message.BroadcasterUserLogin, username)
}
