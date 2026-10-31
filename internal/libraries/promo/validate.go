package promo

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ValidateMessage trims a promo message and checks it is non-empty and fits in
// a single Twitch chat message. Messages starting with "/" or "." are refused so
// a promo can never be interpreted as a Twitch chat command (e.g. /ban).
func ValidateMessage(raw string) (string, error) {
	m := strings.TrimSpace(raw)
	if m == "" {
		return "", fmt.Errorf("message required")
	}
	if utf8.RuneCountInString(m) > MaxMessageLen {
		return "", fmt.Errorf("message must be at most %d characters", MaxMessageLen)
	}
	switch m[0] {
	case '/', '.':
		return "", fmt.Errorf("message can't start with \"/\" or \".\"")
	}
	return m, nil
}
