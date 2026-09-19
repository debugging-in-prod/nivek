package hangman

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

func StartNewGame(httpClient *http.Client) *Game {
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

func PrintState(game *Game) string {
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

	line := fmt.Sprintf("%s  %s", HangmanFaces[wrong], strings.Join(slots, " "))
	if len(misses) > 0 {
		line += "   ❌ " + strings.Join(misses, " ")
	}
	line += fmt.Sprintf(" (%d/%d)", wrong, HangmanMaxWrong)

	if wrong >= HangmanMaxWrong {
		line += fmt.Sprintf(" Game over! The word was %s", game.Word)
		game.GameOver = true
	} else if !slices.Contains(slots, "_") {
		game.GameOver = true
		line += " You win! Use '!hangman' to start a new game"
	}

	return line
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
	return misses, min(len(misses), HangmanMaxWrong)
}
