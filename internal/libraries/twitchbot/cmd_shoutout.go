package twitchbot

import (
	"fmt"
	"log"
	"strings"
)

const shoutoutUsage = "usage: !so <username>"

// shoutout fires the Twitch announcement (Helix Send Chat Announcement) for a
// channel shout. It is the single source of the shoutout wording so the manual
// !so command and the auto-shout system always say exactly the same thing.
// target must be a bare, lowercase Twitch login. Prerequisites (same as
// auto-shout): the bot's user token holds moderator:manage:announcements and the
// bot is a moderator in the channel.
func (b *Bot) shoutout(channelId, target string) {
	b.announce(channelId, fmt.Sprintf("Follow @%s over at twitch.tv/%s !", target, target))
}

// isShoutoutCommand reports whether msg is the !so command, with or without a
// target. Dispatched up front in handleWebhookMessage (like !stalk) so the
// target username isn't also scanned as another command trigger -- e.g.
// "!so bread" must not also fire the !bread builtin.
func isShoutoutCommand(msg string) bool {
	return msg == "!so" || strings.HasPrefix(msg, "!so ")
}

// handleShoutoutCommand implements !so <username>: fire the same announcement
// the auto-shout system sends, for a manually named channel. Mod/broadcaster
// only, matching the other channel-management builtins (!banish, !newpromo).
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
		b.say(channelId, fmt.Sprintf("@%s %s", username, shoutoutUsage))
		return
	}

	// Normalize to a bare lowercase login so the @mention and twitch.tv URL are
	// valid whether the mod typed "Stan", "@Stan", or "stan".
	target := strings.ToLower(strings.TrimPrefix(fields[0], "@"))

	b.shoutout(channelId, target)
	log.Printf("[Shoutout] !so given to %s in %s by %s", target, message.BroadcasterUserLogin, username)
}
