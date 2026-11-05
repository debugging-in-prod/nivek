package hangman

import (
	_ "embed"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
)

// wordsFile is the built-in hangman wordbank: one lowercase word per line.
//
//go:embed words.txt
var wordsFile string

// wordList is wordsFile split into words once at package init.
var wordList = strings.Fields(wordsFile)

// StartNewGame starts a game with a random word from the built-in wordbank.
// It returns nil if the wordbank is empty.
func StartNewGame() *Game {
	if len(wordList) == 0 {
		fmt.Println("[HANGMAN] wordbank is empty")
		return nil
	}

	word := wordList[rand.IntN(len(wordList))]
	fmt.Printf("[HANGMAN] word generated for new game: %s\n", word)

	return &Game{
		Word:     word,
		Guesses:  []string{},
		GameOver: false,
	}
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
