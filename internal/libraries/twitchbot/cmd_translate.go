package twitchbot

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/deepl"
)

const (
	// translateMaxInput caps the characters we send to DeepL per call -- bounds
	// cost/quota and keeps the reply under Twitch's 500-char message limit even
	// after the translation expands.
	translateMaxInput = 300
	// translatePerUserCooldown throttles repeat calls from one chatter so a mod
	// can't accidentally spray the paid API. Silent when it trips.
	translatePerUserCooldown = 5 * time.Second
)

// isTranslateCommand reports whether msg is the !translate command. Dispatched
// up front in handleWebhookMessage (like !newpromo/!stalk) because its argument
// is a free-form message that may itself contain other command triggers.
func isTranslateCommand(msg string) bool {
	return msg == "!translate" || strings.HasPrefix(msg, "!translate ")
}

// containsJapanese reports whether s has any Hiragana, Katakana, or CJK
// (Kanji) runes. For the EN<->JA pair that's all the language detection we
// need: Japanese and English use entirely different scripts, so a script check
// is deterministic and free -- no detection library or extra API call.
func containsJapanese(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) ||
			unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// handleTranslateCommand implements !translate <message>: detect whether the
// text is Japanese or English and translate it to the other language via DeepL.
// Mod/broadcaster only (the row's min_role is display-only for builtins, so the
// gate lives here, like !so/!banish).
func (b *Bot) handleTranslateCommand(message *chatMessageEvent) {
	channelId := message.BroadcasterUserId
	username := message.ChatterUserLogin

	if !isModOrBroadcaster(message) {
		b.say(channelId, fmt.Sprintf("@%s only a mod or the broadcaster can use !translate", username))
		return
	}

	raw := strings.TrimSpace(message.Message.Text)
	text := ""
	if idx := strings.IndexAny(raw, " \t"); idx != -1 {
		text = strings.TrimSpace(raw[idx+1:])
	}
	if text == "" {
		b.say(channelId, fmt.Sprintf("@%s usage: !translate <message>", username))
		return
	}
	if len(text) > translateMaxInput {
		b.say(channelId, fmt.Sprintf("@%s that message is too long to translate (max %d chars)", username, translateMaxInput))
		return
	}

	// Per-user cooldown. Silent on trip so a double-tap doesn't spam chat.
	if b.translateOnCooldown(message.ChatterUserId) {
		return
	}

	apiKey := os.Getenv("DEEPL_API_KEY")
	if apiKey == "" {
		b.say(channelId, fmt.Sprintf("@%s translation isn't configured right now", username))
		return
	}

	// EN<->JA: Japanese script present -> translate to English; else -> Japanese.
	targetLang, tag := "JA", "EN→JA"
	if containsJapanese(text) {
		targetLang, tag = "EN-US", "JA→EN"
	}

	translated, err := deepl.Translate(b.httpClient, apiKey, text, targetLang, "")
	if err != nil {
		log.Printf("[TRANSLATE] %s in %s: %v", username, message.BroadcasterUserLogin, err)
		b.say(channelId, fmt.Sprintf("@%s translation failed, try again later", username))
		return
	}

	b.say(channelId, fmt.Sprintf("@%s [%s] %s", username, tag, translated))
	log.Printf("[TRANSLATE] %s in %s: %s", username, message.BroadcasterUserLogin, tag)
}

// translateOnCooldown reports whether chatterID used !translate within the
// cooldown window, and stamps "now" when it does not.
func (b *Bot) translateOnCooldown(chatterID string) bool {
	b.translateMu.Lock()
	defer b.translateMu.Unlock()
	if last, ok := b.translateCooldown[chatterID]; ok && time.Since(last) < translatePerUserCooldown {
		return true
	}
	b.translateCooldown[chatterID] = time.Now()
	return false
}
