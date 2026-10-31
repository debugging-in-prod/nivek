package twitchbot

import "strings"

// ParseNoReplyLogins parses a comma-delimited list of Twitch logins
// (case-insensitive, surrounding spaces trimmed, empty entries dropped). A
// blank value yields nil. The result is added to defaultNoReplyLogins, never
// a replacement for it.
func ParseNoReplyLogins(raw string) []string {
	var logins []string
	for part := range strings.SplitSeq(raw, ",") {
		if login := strings.ToLower(strings.TrimSpace(part)); login != "" {
			logins = append(logins, login)
		}
	}
	return logins
}

// NoReplyLoginCount returns how many distinct logins the bot ignores for the
// given configured logins and bot username: defaultNoReplyLogins, logins and
// the bot's own login, deduped case-insensitively. Used for the startup log.
func NoReplyLoginCount(logins []string, botUsername string) int {
	return len(buildNoReplySet(logins, botUsername))
}

// buildNoReplySet returns the set of lowercased logins the bot never replies
// to: the union of defaultNoReplyLogins, logins (e.g. from
// TWITCH_BOT_NO_REPLY_LOGINS) and the bot's own login.
func buildNoReplySet(logins []string, botUsername string) map[string]struct{} {
	set := make(map[string]struct{}, len(defaultNoReplyLogins)+len(logins)+1)
	add := func(l string) {
		if l = strings.ToLower(strings.TrimSpace(l)); l != "" {
			set[l] = struct{}{}
		}
	}
	for _, l := range defaultNoReplyLogins {
		add(l)
	}
	for _, l := range logins {
		add(l)
	}
	add(botUsername)
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
