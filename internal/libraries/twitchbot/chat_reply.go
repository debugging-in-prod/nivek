package twitchbot

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxChatReplyLen is the most runes the bot posts in one chat line.
const maxChatReplyLen = 450

// sanitizeChatReply makes text safe to post as a single chat line: control
// characters (newlines, tabs, NUL, ...) and invalid UTF-8 become spaces and
// all whitespace runs (including unicode spaces) collapse to one space;
// leading '/' or '.' are stripped so user/broadcaster-controlled text can never
// be read as a Twitch chat command such as "/ban" or ".timeout"; and the result
// is capped at maxChatReplyLen runes (ending in "…" when cut). An empty result
// means "send nothing".
//
// It is idempotent: sanitizing an already-sanitized string returns it
// unchanged. The send sinks (say, announce, ircsay) call it on every message,
// so handlers that sanitize first are unaffected.
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
