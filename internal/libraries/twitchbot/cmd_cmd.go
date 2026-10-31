package twitchbot

import (
	"fmt"
	"log"
	"strings"
	"unicode"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
)

// cmdTrigger manages a channel's custom commands from chat:
//
//	!cmd add !trigger <response>
//	!cmd edit !trigger <response>
//	!cmd del !trigger            (also "delete" / "remove")
//
// Mod/broadcaster only; anyone else is ignored silently. Validation and storage
// live in core-api (commands.ManageFromChat, shared with the dashboard editor);
// the bot only parses, forwards, applies the result to its in-memory custom
// command map, and posts one short confirmation or error line.
const cmdTrigger = "!cmd"

const cmdUsage = "usage: !cmd add !trigger <response> · !cmd edit !trigger <response> · !cmd del !trigger"

func isCmdCommand(msg string) bool {
	word, _ := cutWord(msg)
	return strings.EqualFold(word, cmdTrigger)
}

// cmdActive reports whether !cmd is seeded and enabled. Until the seed row
// exists the handler stays inactive and the message is treated like any other.
func (b *Bot) cmdActive() bool {
	_, ok := b.lookupCommand(cmdTrigger)
	return ok
}

// cutWord splits s into its first whitespace-delimited word and the remainder
// (leading whitespace trimmed from both).
func cutWord(s string) (word, rest string) {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}
	return s[:i], strings.TrimLeftFunc(s[i:], unicode.IsSpace)
}

// parseCmdArgs parses the raw (case-preserved) chat text of a !cmd message.
// ok is false when the shape is wrong (unknown verb, missing trigger, or a
// missing response for add/edit). The response keeps its original case and
// inner spacing.
func parseCmdArgs(raw string) (action, trigger, response string, ok bool) {
	head, rest := cutWord(raw)
	if !strings.EqualFold(head, cmdTrigger) {
		return "", "", "", false
	}
	verb, rest := cutWord(rest)
	action, ok = commands.NormalizeChatAction(verb)
	if !ok {
		return "", "", "", false
	}
	trigger, rest = cutWord(rest)
	trigger = strings.ToLower(trigger)
	if trigger == "" {
		return "", "", "", false
	}
	response = strings.TrimSpace(rest)
	if action == commands.ChatActionDelete {
		return action, trigger, "", true
	}
	if response == "" {
		return "", "", "", false
	}
	return action, trigger, response, true
}

// cmdReply posts text through the shared sanitizeChatReply (cmd_info.go).
func (b *Bot) cmdReply(channelID, text string) {
	if text = sanitizeChatReply(text); text != "" {
		b.say(channelID, text)
	}
}

// handleCmdCommand runs a !cmd message. The dispatcher has already applied the
// mod check and the central cooldown gate; the mod check is repeated here so
// the handler is safe on any dispatch path.
func (b *Bot) handleCmdCommand(message *chatMessageEvent) {
	if !isModOrBroadcaster(message) {
		return
	}
	channelID := message.BroadcasterUserId
	channel := message.BroadcasterUserLogin
	user := message.ChatterUserLogin

	action, trigger, response, ok := parseCmdArgs(message.Message.Text)
	if !ok {
		b.cmdReply(channelID, fmt.Sprintf("@%s %s", user, cmdUsage))
		return
	}
	if b.coreAPI == nil {
		return
	}

	res, err := b.coreAPI.ManageChannelCommand(commands.ChatManageRequest{
		ChannelTwitchID: channelID,
		Action:          action,
		Trigger:         trigger,
		Response:        response,
	})
	if err != nil {
		log.Printf("[CMD] [%s] %s %s by %s failed: %v", channel, action, trigger, user, err)
		b.cmdReply(channelID, fmt.Sprintf("@%s couldn't save that command, try again later", user))
		return
	}
	if !res.OK {
		msg := res.Error
		if msg == "" {
			msg = "that didn't work"
		}
		b.cmdReply(channelID, fmt.Sprintf("@%s %s", user, msg))
		return
	}

	if b.applyCustomCommandChange(channel, res) {
		// No set was loaded for this channel (offline, or it had none): pull the
		// full set so the in-memory map matches the DB, not just this one row.
		go b.loadCustomCommands(channelID, channel)
	}

	verb := map[string]string{
		commands.ChatActionAdd:    "added",
		commands.ChatActionEdit:   "updated",
		commands.ChatActionDelete: "deleted",
	}[res.Action]
	if verb == "" {
		verb = "saved"
	}
	if res.Trigger == "" {
		res.Trigger = trigger
	}
	log.Printf("[CMD] [%s] %s %s %s", channel, user, verb, res.Trigger)
	b.cmdReply(channelID, fmt.Sprintf("@%s %s %s", user, verb, res.Trigger))
}

// applyCustomCommandChange updates the channel's in-memory custom command set
// after a successful !cmd so the change works immediately. It returns true when
// the channel had no set loaded; the caller should then reload the full set
// (the changed command is already installed so it works in the meantime).
func (b *Bot) applyCustomCommandChange(login string, res commands.ChatManageResult) (needsReload bool) {
	key := strings.ToLower(login)
	trigger := strings.ToLower(res.Trigger)
	if res.Command != nil && res.Command.Trigger != "" {
		trigger = strings.ToLower(res.Command.Trigger)
	}
	if key == "" || trigger == "" {
		return false
	}

	b.customMu.Lock()
	defer b.customMu.Unlock()
	if b.customCommands == nil {
		b.customCommands = make(map[string]map[string]commands.Commands)
	}
	set, loaded := b.customCommands[key]

	install := res.Action != commands.ChatActionDelete &&
		res.Command != nil && res.Command.Enabled && res.Command.ResponseTmpl != nil
	if !install {
		if loaded {
			delete(set, trigger)
			if len(set) == 0 {
				delete(b.customCommands, key)
			}
		}
		return false
	}

	if !loaded {
		set = make(map[string]commands.Commands)
		b.customCommands[key] = set
	}
	set[trigger] = *res.Command
	return !loaded
}
