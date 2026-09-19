package twitchbot

import (
	"fmt"
	"slices"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/hangman"
)

// push state to db on state update
// state update == on new guess, on game over, on new game
// some state updates can involve others ex: if a new guess ends the game

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
	fields := strings.Fields(args)

	// if no args, either new game or checking game state
	if len(fields) == 0 {
		if game != nil && !game.GameOver {

			// resume existing game -- print current game state
			fmt.Printf("[HANGMAN] game found! %+v\n", game)
			b.say(channelId, hangman.PrintState(game))
		} else {

			// new game
			newgame := hangman.StartNewGame(b.httpClient)
			if newgame == nil {
				b.say(channelId, "whoops! Failed to start new game")
			} else {
				b.say(channelId, "New game started!")
				b.say(channelId, hangman.PrintState(newgame))
				b.hangmanGames[channel] = newgame
				go b.pushGameStateToDB(newgame, channel)
			}
		}
		return
	}

	// handle guess

	// check if game exists or if game has already ended
	if game == nil || game.GameOver {
		b.say(channelId, hangman.PrintState(game))
		return
	}

	guess := fields[0]

	// check if word was already guessed
	if !slices.Contains(game.Guesses, guess) {
		game.Guesses = append(game.Guesses, guess)
	}

	b.say(channelId, hangman.PrintState(game))
	fmt.Printf("[HANGMAN] handling guess %s for word %s\n", guess, game.Word)
}

func (b *Bot) pushGameStateToDB(game *hangman.Game, channel string) {
	b.coreAPI.PushHangmanGameState(game, channel)
}
