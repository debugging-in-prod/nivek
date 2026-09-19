package twitchbot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

type Hangman struct {
	Games map[string]*Game // map[channel]Game
}

type Game struct {
	Word     string
	Guesses  []string
	GameOver bool
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
		if game != nil && !game.GameOver {

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

	// check if game exists or if game has already ended
	if game == nil || game.GameOver {
		b.say(channelId, printState(game))
		return
	}

	guess := fields[0]

	// check if word was already guessed
	if !slices.Contains(game.Guesses, guess) {
		game.Guesses = append(game.Guesses, guess)
	}

	b.say(channelId, printState(game))
	fmt.Printf("[HANGMAN] handling guess %s for word %s\n", guess, game.Word)
}

// calcMisses figure out how many wrong guesses have been made and which letters are wrong
func calcMisses(game *Game) ([]string, int) {
	if game == nil {
		return []string{}, 0
	}

	var misses []string
	for _, g := range game.Guesses {
		lg := strings.ToLower(g)
		if len(lg) == 1 && !strings.ContainsAny(game.Word, lg) {
			misses = append(misses, lg)
		}
	}
	return misses, min(len(misses), hangmanMaxWrong)
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

	misses, wrong := calcMisses(game)

	line := fmt.Sprintf("%s  %s", hangmanFaces[wrong], strings.Join(slots, " "))
	if len(misses) > 0 {
		line += "   ❌ " + strings.Join(misses, " ")
	}
	line += fmt.Sprintf(" (%d/%d)", wrong, hangmanMaxWrong)

	if wrong >= hangmanMaxWrong {
		line += fmt.Sprintf(" Game over! The word was %s", game.Word)
		game.GameOver = true
	} else if !slices.Contains(slots, "_") {
		game.GameOver = true
		line += " You win! Use '!hangman' to start a new game"
	}

	return line
}

func startNewGame(httpClient *http.Client) *Game {
	newgame := &Game{
		Word:     "",
		Guesses:  []string{},
		GameOver: false,
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
