package promo

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// ValidateMessage trims a promo message and checks it is non-empty and fits in
// a single Twitch chat message.
func ValidateMessage(raw string) (string, error) {
	m := strings.TrimSpace(raw)
	if m == "" {
		return "", fmt.Errorf("message required")
	}
	if utf8.RuneCountInString(m) > MaxMessageLen {
		return "", fmt.Errorf("message must be at most %d characters", MaxMessageLen)
	}
	return m, nil
}
