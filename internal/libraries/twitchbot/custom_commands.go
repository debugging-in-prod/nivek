package twitchbot

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
)

// Per-channel custom ("channel"-scoped) commands are loaded on demand rather
// than once at boot: the set is pulled when a channel goes live (stream.online
// webhook, or at boot for channels already live) and dropped when it goes
// offline. This keeps the dispatch map small and the in-memory data fresh for
// the current stream — a dashboard edit mid-stream is picked up on the next
// go-live, matching how the bot already treats a live session as the unit of
// state (auto-shout, !dad counters). Edits made from chat with !cmd are applied
// to this map immediately (see cmd_cmd.go).

// loadCustomCommands fetches the channel's enabled custom commands from core-api
// (keyed by Twitch id) and stores them for dispatch under the lowercased login,
// which is what handleMessage matches on. Safe to call repeatedly — it replaces
// the channel's set wholesale, so a re-fire (e.g. a duplicate stream.online)
// just refreshes it. Intended to run in its own goroutine off the webhook/boot
// path since it makes a network call. Fails soft: errors are logged and a
// panic is recovered, so a bad refresh never takes the bot down.
func (b *Bot) loadCustomCommands(twitchID, login string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[CUSTOM-CMD] panic loading custom commands for %s (%s): %v", login, twitchID, r)
		}
	}()
	rows, err := b.coreAPI.GetChannelCommands(twitchID)
	if err != nil {
		log.Printf("[CUSTOM-CMD] failed to load custom commands for %s (%s): %s", login, twitchID, err.Error())
		return
	}

	set := make(map[string]commands.Commands, len(rows))
	for _, row := range rows {
		// A channel row without a template can't be answered (only global
		// builtins carry a compiled handler), so skip it rather than register a
		// trigger that would silently do nothing.
		if row.ResponseTmpl == nil {
			continue
		}
		set[strings.ToLower(row.Trigger)] = row
	}

	key := strings.ToLower(login)
	b.customMu.Lock()
	if len(set) == 0 {
		delete(b.customCommands, key)
	} else {
		b.customCommands[key] = set
	}
	b.customMu.Unlock()

	log.Printf("[CUSTOM-CMD] loaded %d custom command(s) for %s", len(set), login)
}

// dropCustomCommands evicts a channel's custom command set. Called on
// stream.offline so an offline channel's triggers stop responding and the map
// stays bounded to currently-live channels.
func (b *Bot) dropCustomCommands(login string) {
	key := strings.ToLower(login)
	b.customMu.Lock()
	delete(b.customCommands, key)
	b.customMu.Unlock()
}

// customCommandFor looks up a channel's custom command by trigger (both keys are
// lowercased). ok is false when the channel has no custom set or no such
// trigger.
func (b *Bot) customCommandFor(login, trigger string) (commands.Commands, bool) {
	b.customMu.Lock()
	defer b.customMu.Unlock()
	set, ok := b.customCommands[strings.ToLower(login)]
	if !ok {
		return commands.Commands{}, false
	}
	cmd, ok := set[strings.ToLower(trigger)]
	return cmd, ok
}

// commandUseTimeout bounds the ${count} increment call so a slow core-api
// can't hold a chat dispatch worker for the client's full timeout.
const commandUseTimeout = 2 * time.Second

// replyCustomCommand expands a custom command's response template for this
// message and queues the reply. The caller has already passed the role check
// and the central cooldown gate (commandAllowed), so this runs at most once
// per allowed dispatch. A failed ${count} increment just leaves the variable
// literal; the reply is still sent. A reply that sanitizes to nothing is
// dropped.
func (b *Bot) replyCustomCommand(message *chatMessageEvent, cmd commands.Commands, trigger string) {
	if cmd.ResponseTmpl == nil {
		return
	}
	vars := responseVars{
		User:    displayOr(message.ChatterUserName, message.ChatterUserLogin),
		Channel: displayOr(message.BroadcasterUserName, message.BroadcasterUserLogin),
		Args:    commandArgs(message.Message.Text, trigger),
		Count: func() (int, bool) {
			return b.incrementCommandUse(cmd.Id, message.BroadcasterUserId)
		},
	}
	reply := sanitizeChatReply(expandResponse(*cmd.ResponseTmpl, vars))
	if reply == "" {
		log.Printf("[CUSTOM-CMD] %s in %s expanded to an empty reply; not sending", trigger, message.BroadcasterUserLogin)
		return
	}
	b.say(message.BroadcasterUserId, reply)
}

// incrementCommandUse bumps the command's persisted use counter via core-api.
// ok=false on any failure (logged), which the expander treats as "leave
// ${count} literal".
func (b *Bot) incrementCommandUse(commandID int, channelTwitchID string) (int, bool) {
	if b.coreAPI == nil || commandID <= 0 || channelTwitchID == "" {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandUseTimeout)
	defer cancel()
	n, err := b.coreAPI.IncrementCommandUse(ctx, commandID, channelTwitchID)
	if err != nil {
		log.Printf("[CUSTOM-CMD] failed to increment use count for command %d (%s): %s", commandID, channelTwitchID, err.Error())
		return 0, false
	}
	return n, true
}

func displayOr(display, login string) string {
	if display != "" {
		return display
	}
	return login
}

// meetsMinRole reports whether the message's author clears a command's min_role.
// Roles are cumulative (a broadcaster clears every tier). "everyone" and any
// unrecognized value are open to all, matching the column's default.
func meetsMinRole(message *chatMessageEvent, minRole string) bool {
	isBroadcaster := message.BroadcasterUserId == message.ChatterUserId
	isModerator := false
	isSubscriber := false
	isVip := false

	for _, badge := range message.Badges {
		if badge.SetId == "moderator" {
			isModerator = true
		}

		if badge.SetId == "subscriber" {
			isSubscriber = true
		}

		if badge.SetId == "vip" {
			isVip = true
		}
	}

	switch minRole {
	case "broadcaster":
		return isBroadcaster
	case "mod":
		if isBroadcaster || isModerator {
			return true
		}

		return false
	case "vip":
		return isBroadcaster || isModerator || isVip
	case "sub":
		if isBroadcaster || isModerator || isSubscriber {
			return true
		}

		return false
	default: // "everyone" or anything unexpected
		return true
	}
}
