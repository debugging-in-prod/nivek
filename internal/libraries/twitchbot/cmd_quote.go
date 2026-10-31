package twitchbot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/quote"
)

const (
	quoteTrigger = "!quote"
	quoteUsage   = "usage: !quote  ·  !quote <n>  ·  !quote add <text>  ·  !quote del <n>"
)

// isQuoteCommand reports whether the (lowercased) message is a !quote
// invocation that the up-front dispatcher should own.
func isQuoteCommand(msg string) bool {
	return msg == quoteTrigger || strings.HasPrefix(msg, quoteTrigger+" ")
}

// quoteArgs returns the text after the leading "!quote", with case preserved.
// A !quote matched mid-message by the word scan (not at the start) has no
// arguments, so it just rolls a random quote.
func quoteArgs(text string) string {
	raw := strings.TrimSpace(text)
	if !isQuoteCommand(strings.ToLower(raw)) && !strings.HasPrefix(strings.ToLower(raw), quoteTrigger+"\t") {
		return ""
	}
	return strings.TrimSpace(raw[len(quoteTrigger):])
}

// handleQuoteCommand implements !quote:
//
//	!quote             → a random quote from this channel
//	!quote <n>         → quote #n
//	!quote add <text>  → mod/broadcaster: store a quote (max 300 chars)
//	!quote del <n>     → mod/broadcaster: delete quote #n
//
// Dispatch (and therefore the cooldown gate) happens in handleWebhookMessage;
// every reply below runs at most once per allowed invocation. Core-api errors
// are logged and never stop the bot.
func (b *Bot) handleQuoteCommand(message *chatMessageEvent) {
	channelId := message.BroadcasterUserId
	username := message.ChatterUserLogin
	if b.coreAPI == nil || channelId == "" {
		return
	}

	args := quoteArgs(message.Message.Text)
	sub, rest := args, ""
	if idx := strings.IndexAny(args, " \t"); idx != -1 {
		sub, rest = args[:idx], strings.TrimSpace(args[idx+1:])
	}

	switch strings.ToLower(sub) {
	case "":
		b.replyQuote(channelId, username, 0)
	case "add":
		if !isModOrBroadcaster(message) {
			b.sayQuote(channelId, fmt.Sprintf("@%s only a mod or the broadcaster can add quotes", username))
			return
		}
		b.addQuote(channelId, username, rest)
	case "del", "delete", "remove", "rm":
		if !isModOrBroadcaster(message) {
			b.sayQuote(channelId, fmt.Sprintf("@%s only a mod or the broadcaster can delete quotes", username))
			return
		}
		n, ok := parseQuoteNumber(rest)
		if !ok {
			b.sayQuote(channelId, fmt.Sprintf("@%s %s", username, quoteUsage))
			return
		}
		b.deleteQuote(channelId, username, n)
	default:
		n, ok := parseQuoteNumber(sub)
		if !ok {
			b.sayQuote(channelId, fmt.Sprintf("@%s %s", username, quoteUsage))
			return
		}
		b.replyQuote(channelId, username, n)
	}
}

func (b *Bot) replyQuote(channelId, username string, number int) {
	q, found, err := b.coreAPI.GetQuote(channelId, number)
	if err != nil {
		// Read failures stay silent in chat; the log is enough.
		log.Printf("[QUOTE] [%s] get #%d failed: %v", channelId, number, err)
		return
	}
	if !found {
		b.sayQuote(channelId, fmt.Sprintf("@%s, no quote found", username))
		return
	}
	b.sayQuote(channelId, formatQuote(q, b.location))
}

func (b *Bot) addQuote(channelId, username, text string) {
	text = quote.Clean(text)
	if text == "" {
		b.sayQuote(channelId, fmt.Sprintf("@%s usage: !quote add <text>", username))
		return
	}
	if utf8.RuneCountInString(text) > quote.MaxTextLen {
		b.sayQuote(channelId, fmt.Sprintf("@%s that quote is too long (max %d chars)", username, quote.MaxTextLen))
		return
	}
	q, err := b.coreAPI.AddQuote(channelId, text, username)
	if err != nil {
		log.Printf("[QUOTE] [%s] add by %s failed: %v", channelId, username, err)
		b.sayQuote(channelId, fmt.Sprintf("@%s couldn't save that quote", username))
		return
	}
	log.Printf("[QUOTE] [%s] %s added quote #%d", channelId, username, q.QuoteNumber)
	b.sayQuote(channelId, fmt.Sprintf("Quote #%d added", q.QuoteNumber))
}

func (b *Bot) deleteQuote(channelId, username string, number int) {
	found, err := b.coreAPI.DeleteQuote(channelId, number)
	if err != nil {
		log.Printf("[QUOTE] [%s] delete #%d by %s failed: %v", channelId, number, username, err)
		b.sayQuote(channelId, fmt.Sprintf("@%s couldn't delete that quote", username))
		return
	}
	if !found {
		b.sayQuote(channelId, fmt.Sprintf("@%s, no quote found", username))
		return
	}
	log.Printf("[QUOTE] [%s] %s deleted quote #%d", channelId, username, number)
	b.sayQuote(channelId, fmt.Sprintf("Quote #%d deleted", number))
}

// sayQuote is the only way the !quote handler talks to chat: every reply goes
// through sanitizeChatReply first.
func (b *Bot) sayQuote(channelId, msg string) {
	if msg = sanitizeChatReply(msg); msg != "" {
		b.say(channelId, msg)
	}
}

// parseQuoteNumber accepts "12" or "#12".
func parseQuoteNumber(s string) (int, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// formatQuote renders `#12: "text" [2026-10-31]` (with the game, when known,
// as `[Game, 2026-10-31]`). Stored text is re-cleaned on the way out so a row
// written by any path is still a single chat line.
func formatQuote(q quote.Quote, loc *time.Location) string {
	created := q.CreatedAt
	if loc != nil {
		created = created.In(loc)
	}
	meta := created.Format("2006-01-02")
	if q.Game != nil {
		if g := quote.Clean(*q.Game); g != "" {
			meta = g + ", " + meta
		}
	}
	return fmt.Sprintf("#%d: \"%s\" [%s]", q.QuoteNumber, quote.Clean(q.Text), meta)
}
