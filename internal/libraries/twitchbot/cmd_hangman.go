package twitchbot

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/hangman"
)

// hangmanNoGameMsg is the reply to a guess when no game is running.
const hangmanNoGameMsg = "No game running, start one with !hangman"

// hangmanWrongLengthMsg is the reply to a word guess of the wrong length. It
// never echoes the guess; %d is the secret word's length, already visible on
// the board.
const hangmanWrongLengthMsg = "Word guesses must be %d letters long"

// stripInvisible removes invisible/format runes (e.g. the U+034F combining
// grapheme joiner Chatterino/7TV append to dodge Twitch's duplicate-message
// filter) so they aren't mistaken for a guess. Whitespace is kept.
func stripInvisible(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 0x034F, 0x200B, 0x200C, 0x200D, 0x2060, 0xFEFF:
			return -1
		}
		if !unicode.IsGraphic(r) && !unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

func (b *Bot) handleHangmanCommand(message *chatMessageEvent) {
	channel := message.BroadcasterUserLogin
	channelId := message.BroadcasterUserId

	// get game record
	game := b.hangmanGames[channel]

	// get command args
	raw := strings.TrimSpace(message.Message.Text)
	args := ""
	if idx := strings.IndexAny(raw, " \t"); idx != -1 {
		args = strings.TrimSpace(raw[idx+1:])
	}
	fields := strings.Fields(stripInvisible(args))

	// if no args, either new game or checking game state
	if len(fields) == 0 {
		if game != nil && !game.GameOver {

			// resume existing game -- print current game state
			fmt.Printf("[HANGMAN] game found! %+v\n", game)
			b.say(channelId, hangman.PrintState(game))
		} else {

			// new game
			newgame := hangman.StartNewGame()
			if newgame == nil {
				b.say(channelId, "whoops! Failed to start new game")
			} else {
				b.say(channelId, "New game started!")
				b.say(channelId, hangman.PrintState(newgame))
				b.hangmanGames[channel] = newgame
				go b.pushGameStateToDB(snapshotGame(newgame), channel, b.hangmanSeq.Add(1))
			}
		}
		return
	}

	// handle guess

	// check if game exists or if game has already ended
	if game == nil || game.GameOver {
		b.say(channelId, hangmanNoGameMsg)
		return
	}

	guess := fields[0]

	// a word guess of the wrong length can't be right: reject it without
	// storing, counting, or echoing it
	if hangman.WrongLength(game, guess) {
		b.say(channelId, fmt.Sprintf(hangmanWrongLengthMsg, utf8.RuneCountInString(game.Word)))
		return
	}

	// record the guess (repeats are ignored); this also ends the game on a
	// win or loss
	isNew := hangman.Guess(game, guess)

	b.say(channelId, hangman.PrintState(game))
	fmt.Printf("[HANGMAN] handling guess %s for word %s\n", guess, game.Word)

	// persist the new state, including the final state when the game ends
	if isNew {
		go b.pushGameStateToDB(snapshotGame(game), channel, b.hangmanSeq.Add(1))
	}
}

// snapshotGame copies game so a background push never reads it while the
// next guess is being applied.
func snapshotGame(game *hangman.Game) *hangman.Game {
	cp := *game
	cp.Guesses = slices.Clone(game.Guesses)
	return &cp
}

// pushGameStateToDB saves game to core-api. It runs in its own goroutine and
// only logs on failure; the in-memory game is unaffected. seq is the
// snapshot's issue order (from hangmanSeq); a snapshot older than one already
// saved for channel is dropped so the latest state wins.
func (b *Bot) pushGameStateToDB(game *hangman.Game, channel string, seq uint64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[HANGMAN] [%s] panic pushing game state: %v", channel, r)
		}
	}()
	b.hangmanPushMu.Lock()
	defer b.hangmanPushMu.Unlock()
	if seq <= b.hangmanSaved[channel] {
		return
	}
	if err := b.coreAPI.PushHangmanGameState(game, channel); err != nil {
		log.Printf("[HANGMAN] [%s] push game state failed: %v", channel, err)
		return
	}
	if b.hangmanSaved == nil {
		b.hangmanSaved = make(map[string]uint64)
	}
	b.hangmanSaved[channel] = seq
}
