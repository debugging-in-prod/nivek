package twitchbot

import "strings"

// DefaultNoReplyLogins are the chatters the bot ignores when
// TWITCH_BOT_NO_REPLY_LOGINS is unset: other chat bots whose automated
// messages could otherwise trigger reply loops.
var DefaultNoReplyLogins = []string{"streamelements", "moobot", "sery_bot"}

// ParseNoReplyLogins parses a comma-delimited list of Twitch logins
// (case-insensitive, surrounding spaces trimmed, empty entries dropped).
// A blank value yields DefaultNoReplyLogins.
func ParseNoReplyLogins(raw string) []string {
	var logins []string
	for part := range strings.SplitSeq(raw, ",") {
		if login := strings.ToLower(strings.TrimSpace(part)); login != "" {
			logins = append(logins, login)
		}
	}
	if len(logins) == 0 {
		return append([]string(nil), DefaultNoReplyLogins...)
	}
	return logins
}

// buildNoReplySet returns the set of lowercased logins the bot never replies
// to. A nil logins slice means DefaultNoReplyLogins. The bot's own login is
// always included.
func buildNoReplySet(logins []string, botUsername string) map[string]struct{} {
	if logins == nil {
		logins = DefaultNoReplyLogins
	}
	set := make(map[string]struct{}, len(logins)+1)
	for _, l := range logins {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			set[l] = struct{}{}
		}
	}
	if bot := strings.ToLower(strings.TrimSpace(botUsername)); bot != "" {
		set[bot] = struct{}{}
	}
	return set
}

// isNoReplyLogin reports whether the bot must ignore messages from login.
func (b *Bot) isNoReplyLogin(login string) bool {
	if login == "" {
		return false
	}
	if strings.EqualFold(login, b.config.BotUsername) {
		return true
	}
	_, ok := b.noReply[strings.ToLower(strings.TrimSpace(login))]
	return ok
}
