package twitchbot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Hangman struct {
	Games map[string]*Game // map[channel]Game
}

type Game struct {
	Word    string
	Guesses []string
}

const hangmanMaxWrong = 6

var hangmanFaces = [hangmanMaxWrong + 1]string{
	"😀", "😧", "😨", "😰", "😱", "😵", "💀", // index 6 = dead / game over
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
	fields := strings.Fields(args)

	// if no args, either new game or checking game state
	if len(fields) == 0 {
		if game != nil && len(game.Guesses) < hangmanMaxWrong {

			// resume existing game -- print current game state
			fmt.Printf("[HANGMAN] game found! %+v\n", game)
			b.say(channelId, printState(game))
		} else {

			// new game
			newgame := startNewGame(b.httpClient)
			if newgame == nil {
				b.say(channelId, "whoops! Failed to start new game")
			} else {
				b.say(channelId, "New game started!")
				b.say(channelId, printState(newgame))
				b.hangmanGames[channel] = newgame
			}
		}
		return
	}

	// handle guess

	// check how many guesses have already been made
	// if 6 guesses - game has ended and this guess is invalid
	if len(game.Guesses) >= hangmanMaxWrong {
		b.say(channelId, printState(game))
		b.say(channelId, "use command '!hangman' to start a new game")
		return
	}

	guess := fields[0]
	game.Guesses = append(game.Guesses, guess)
	b.say(channelId, printState(game))
	fmt.Printf("[HANGMAN] handling guess %s for word %s", guess, game.Word)
}

func printState(game *Game) string {
	guessed := make(map[rune]bool, len(game.Guesses))
	for _, g := range game.Guesses {
		for _, r := range strings.ToLower(g) {
			guessed[r] = true
		}
	}

	slots := make([]string, 0, len(game.Word))
	for _, r := range game.Word {
		if guessed[r] {
			slots = append(slots, string(r))
		} else {
			slots = append(slots, "_")
		}
	}

	var misses []string
	for _, g := range game.Guesses {
		lg := strings.ToLower(g)
		if len(lg) == 1 && !strings.ContainsAny(game.Word, lg) {
			misses = append(misses, lg)
		}
	}
	wrong := min(len(misses), hangmanMaxWrong)

	line := fmt.Sprintf("%s  %s", hangmanFaces[wrong], strings.Join(slots, " "))
	if len(misses) > 0 {
		line += "   ❌ " + strings.Join(misses, " ")
	}
	line += fmt.Sprintf(" (%d/%d)", wrong, hangmanMaxWrong)

	if wrong >= hangmanMaxWrong {
		line += " Game over!"
	}

	return line
}

func startNewGame(httpClient *http.Client) *Game {
	newgame := &Game{
		Word:    "",
		Guesses: []string{},
	}

	// get word
	resp, err := httpClient.Get("https://random-word-api.herokuapp.com/word")
	if err != nil {
		fmt.Printf("[HANGMAN] failed to fetch word! %s\n", err.Error())
		return nil
	}
	defer resp.Body.Close()

	var words []string
	if err := json.NewDecoder(resp.Body).Decode(&words); err != nil {
		fmt.Printf("[HANGMAN] failed to decode new-word response: %s\n", err.Error())
		return nil
	}

	if len(words) == 0 {
		fmt.Println("[HANGMAN] word API returned an empty array")
		return nil
	}

	fmt.Printf("[HANGMAN] word generated for new game: %s\n", words[0])

	newgame.Word = words[0]
	return newgame
}
