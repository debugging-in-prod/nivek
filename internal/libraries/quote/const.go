package quote

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const TableQuote = "quote"

// MaxTextLen is the longest quote (in characters, after Clean) that can be
// stored. Enforced by the bot and again by core-api.
const MaxTextLen = 300

// Quote is one row of nivek.quote. QuoteNumber is per channel (#1, #2, ...)
// and is what chatters see and type; Id is the internal serial.
type Quote struct {
	Id              int       `db:"id,omitempty" json:"id"`
	ChannelTwitchId string    `db:"channel_twitch_id" json:"channel_twitch_id"`
	QuoteNumber     int       `db:"quote_number" json:"quote_number"`
	Text            string    `db:"text" json:"text"`
	AddedBy         string    `db:"added_by" json:"added_by"`
	Game            *string   `db:"game" json:"game,omitempty"`
	CreatedAt       time.Time `db:"created_at,omitempty" json:"created_at"`
}

// Clean normalizes quote text: control characters (including newlines) become
// spaces and runs of whitespace collapse to one, so a stored quote is always a
// single printable chat line.
func Clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == utf8.RuneError || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// ValidText cleans s and reports whether it is storable (non-empty, at most
// MaxTextLen characters).
func ValidText(s string) (string, bool) {
	s = Clean(s)
	n := utf8.RuneCountInString(s)
	return s, n > 0 && n <= MaxTextLen
}
