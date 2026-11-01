package twitchbot

import (
	"encoding/json"
	"log"
	"slices"
	"strings"
)

type chatMessageEvent struct {
	BroadcasterUserId    string          `json:"broadcaster_user_id"`
	BroadcasterUserLogin string          `json:"broadcaster_user_login"`
	BroadcasterUserName  string          `json:"broadcaster_user_name"`
	ChatterUserId        string          `json:"chatter_user_id"`
	ChatterUserLogin     string          `json:"chatter_user_login"`
	ChatterUserName      string          `json:"chatter_user_name"`
	MessageId            string          `json:"message_id"`
	Message              chatMessageBody `json:"message"`
	Color                string          `json:"color"`
	Badges               []badges        `json:"badges"`
	// Badges example content:
	// [
	//   {
	//     "set_id": "moderator",
	//     "id": "1",
	//     "info": ""
	//   },
	//   {
	//     "set_id": "subscriber",
	//     "id": "12",
	//     "info": "16"
	//   },
	//   {
	//     "set_id": "sub-gifter",
	//     "id": "1",
	//     "info": ""
	//   }
	// ],
	MessageType                 string     `json:"message_type"`
	Cheer                       *chatCheer `json:"cheer,omitempty"`
	Reply                       *chatReply `json:"reply,omitempty"`
	ChannelPointsCustomRewardId *string    `json:"channel_points_custom_reward_id,omitempty"`
	// Shared Chat: set when the message was sent in another channel of a
	// shared session and relayed to this one; null for ordinary messages.
	SourceBroadcasterUserId    *string  `json:"source_broadcaster_user_id,omitempty"`
	SourceBroadcasterUserLogin *string  `json:"source_broadcaster_user_login,omitempty"`
	SourceBroadcasterUserName  *string  `json:"source_broadcaster_user_name,omitempty"`
	SourceMessageId            *string  `json:"source_message_id,omitempty"`
	SourceBadges               []badges `json:"source_badges,omitempty"`

	// matchedTrigger is the command word the dispatcher matched, set just before
	// a handler runs. Not part of the Twitch payload -- it exists so a handler
	// can find its own arguments in a message that may contain several commands.
	matchedTrigger string
}

type badges struct {
	SetId string `json:"set_id"`
	Id    string `json:"id"`
	Info  string `json:"info"`
}

// chatCheer is the "cheer" object of a channel.chat.message event (null unless
// the message carried bits).
type chatCheer struct {
	Bits int `json:"bits"`
}

// chatReply is the "reply" object of a channel.chat.message event (null unless
// the message is a reply).
type chatReply struct {
	ParentMessageId   string `json:"parent_message_id"`
	ParentMessageBody string `json:"parent_message_body"`
	ParentUserId      string `json:"parent_user_id"`
	ParentUserName    string `json:"parent_user_name"`
	ParentUserLogin   string `json:"parent_user_login"`
	ThreadMessageId   string `json:"thread_message_id"`
	ThreadUserId      string `json:"thread_user_id"`
	ThreadUserName    string `json:"thread_user_name"`
	ThreadUserLogin   string `json:"thread_user_login"`
}

// isForeignSharedChat reports whether the message was sent in another channel
// of a Shared Chat session and only relayed to this one. The bot ignores those:
// the origin channel's own copy is the one to act on, and acting on the relay
// would answer commands for (and double-reply into) a channel it wasn't sent in.
func (m *chatMessageEvent) isForeignSharedChat() bool {
	return m.SourceBroadcasterUserId != nil &&
		*m.SourceBroadcasterUserId != "" &&
		*m.SourceBroadcasterUserId != m.BroadcasterUserId
}

type chatMessageBody struct {
	Text      string     `json:"text"`
	Fragments []fragment `json:"fragments"`
	// example Fragments content:
	// [
	//        {
	//          "type": "text",
	//          "text": "Hi chat",
	//          "cheermote": null,
	//          "emote": null,
	//          "mention": null
	// 	}
	// ]
}

type fragment struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	Cheermote *any   `json:"cheermote,omitempty"`
	Emote     *any   `json:"emote,omitempty"`
	Mention   *any   `json:"mention,omitempty"`
}

func (b *Bot) handleWebhookMessage(notification *EventSubSubscriptionResponse) {
	var messageEvent chatMessageEvent
	if err := json.Unmarshal(notification.Event, &messageEvent); err != nil {
		log.Printf("failed to read chat message Event: %s", err.Error())
		return
	}

	// Shared Chat relay from another channel: drop silently before dedupe,
	// counters, and any dispatch or reply (no per-message log -- these are
	// frequent during shared sessions).
	if messageEvent.isForeignSharedChat() {
		return
	}

	if b.seenChatIDs != nil && b.seenChatIDs.seen(messageEvent.MessageId) {
		log.Printf("[EVENTSUB] duplicate chat message_id=%s; dropping", messageEvent.MessageId)
		return
	}

	if messageEvent.ChatterUserId == b.config.botID() || b.isNoReplyLogin(messageEvent.ChatterUserLogin) {
		return
	}

	// Count toward the promo activity gate (bot + no-reply already excluded).
	b.recordChatLine(messageEvent.BroadcasterUserLogin, messageEvent.ChatterUserLogin)

	b.rememberStalkMessage(
		messageEvent.BroadcasterUserLogin,
		messageEvent.ChatterUserLogin,
		messageEvent.ChatterUserName,
		messageEvent.Message.Text,
	)

	msg := strings.ToLower(messageEvent.Message.Text)
	channel := messageEvent.BroadcasterUserLogin
	channelId := messageEvent.BroadcasterUserId
	chatter := messageEvent.ChatterUserLogin

	// !newpromo takes free-form arguments (an interval + a message that may itself
	// contain other command words or URLs). Dispatch it up front and return so the
	// word-by-word scan below never treats the promo body as more commands.
	if strings.HasPrefix(msg, "!newpromo ") {
		if b.commandAllowed(&messageEvent, newPromoTrigger, b.builtinCooldownSecs(newPromoTrigger)) {
			log.Printf("[CMD-RECV] [%s] %s: %q", channel, chatter, msg)
			b.handleNewPromoCommand(&messageEvent)
		}
		return
	}

	// !stalk also takes arguments (`set` / `clear` / a username). Dispatch it up
	// front so a target named after another command isn't also fired, and so the
	// no-arg form still works if the DB seed hasn't loaded into b.commands yet.
	if isStalkCommand(msg) {
		if b.commandAllowed(&messageEvent, stalkTrigger, b.builtinCooldownSecs(stalkTrigger)) {
			log.Printf("[CMD-RECV] [%s] %s: %q", channel, chatter, msg)
			b.handleStalkCommand(&messageEvent)
		}
		return
	}

	// !translate takes a free-form message argument that may itself contain other
	// command triggers. Dispatch it up front and return so that body is never
	// scanned for more commands below.
	if isTranslateCommand(msg) {
		if b.commandAllowed(&messageEvent, translateTrigger, b.builtinCooldownSecs(translateTrigger)) {
			log.Printf("[CMD-RECV] [%s] %s: %q", channel, chatter, msg)
			b.handleTranslateCommand(&messageEvent)
		}
		return
	}

	// !quote add takes free-form text that may contain other command triggers.
	// Dispatch it up front, but only once the global !quote seed row has loaded
	// into b.commands: without the row the builtin stays inactive and the
	// message falls through to the normal scan (e.g. a channel's custom !quote).
	if _, enabled := b.lookupCommand(quoteTrigger); enabled && isQuoteCommand(msg) {
		if b.commandAllowed(&messageEvent, quoteTrigger, b.builtinCooldownSecs(quoteTrigger)) {
			log.Printf("[CMD-RECV] [%s] %s: %q", channel, chatter, msg)
			messageEvent.matchedTrigger = quoteTrigger
			b.handleQuoteCommand(&messageEvent)
		}
		return
	}

	// !cmd add|edit|del takes a free-form response that may itself contain
	// other command words. Dispatch it up front and return so the response is
	// never scanned for more commands. Only active once the !cmd seed row is
	// loaded; non-mods are ignored silently (checked before the cooldown gate
	// so they can't burn it).
	if b.cmdActive() && isCmdCommand(msg) {
		if isModOrBroadcaster(&messageEvent) &&
			b.commandAllowed(&messageEvent, cmdTrigger, b.builtinCooldownSecs(cmdTrigger)) {
			log.Printf("[CMD-RECV] [%s] %s: %q", channel, chatter, msg)
			messageEvent.matchedTrigger = cmdTrigger
			b.handleCmdCommand(&messageEvent)
		}
		return
	}

	// Check for commands. Every builtin and custom dispatch below goes through
	// commandAllowed (cooldown.go) -- keep it that way for new dispatch paths.
	caretChecked := false
	for msgword := range strings.SplitSeq(msg, " ") {
		// Intentional "^" echo: at most one reply per message (only the first
		// word containing "^" is considered), gated by caretEchoAllowed.
		if !caretChecked && strings.Contains(msgword, "^") {
			caretChecked = true
			if b.caretEchoAllowed(&messageEvent) {
				b.say(channelId, "^")
			}
		}
		// A global builtin wins the word; a channel's own custom command with the
		// same trigger falls through to the handling below (a channel can't shadow
		// a builtin in v1).
		if handler, ok := b.lookupCommand(msgword); ok {
			// Central cooldown gate: silently skip a command still cooling down.
			if !b.commandAllowed(&messageEvent, msgword, b.builtinCooldownSecs(msgword)) {
				continue
			}
			log.Printf("[CMD-RECV] [%s] %s: %q", channel, chatter, msg)
			messageEvent.matchedTrigger = msgword
			handler(b, &messageEvent)
			continue
		}
		// Per-channel custom commands (loaded while the channel is live). An
		// ungated global with the same trigger wins — handled above via continue
		// — so a channel can't shadow a builtin in v1.
		if cmd, ok := b.customCommandFor(channel, msgword); ok {
			// Role check first so an unauthorized attempt doesn't burn the
			// cooldown for everyone else.
			if cmd.ResponseTmpl != nil && meetsMinRole(&messageEvent, cmd.MinRole) &&
				b.commandAllowed(&messageEvent, msgword, cmd.CooldownSecs) {
				log.Printf("[CMD-RECV] [%s] %s: %q (custom)", channel, chatter, msg)
				b.replyCustomCommand(&messageEvent, cmd, msgword)
			}
		}
	}

	if _, ok := b.autoShout[channel]; ok {
		if slices.Contains(
			b.autoShout[channel],
			messageEvent.ChatterUserName,
		) {
			// Perform the shoutout ourselves instead of emitting "!so @user" for
			// another bot to act on: a Twitch announcement (Helix Send Chat
			// Announcement) prompting a follow, which renders with the highlighted
			// "Announcement" header. Shared with the manual !so command via
			// b.shoutout so both say exactly the same thing.
			b.shoutout(channelId, chatter)
			log.Printf("[Auto Shout] given to %s in %s", chatter, channel)
			// Persist the shout: bump shout_count and stamp this stream's key so
			// a restart mid-stream won't re-shout them. Off the message path.
			go b.incrementAutoShout(channel, messageEvent.ChatterUserName)
			i := slices.Index(b.autoShout[channel], messageEvent.ChatterUserName)
			b.autoShout[channel] = slices.Delete(
				b.autoShout[channel],
				i,
				i+1,
			)
		}
	}

}
