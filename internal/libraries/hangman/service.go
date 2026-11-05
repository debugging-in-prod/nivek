package hangman

import (
	_ "embed"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"unicode/utf8"
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

// Guess records guess against game, ignoring repeats, and sets GameOver
// when the guess wins or loses the game. A guess of more than
// one letter is a guess at the whole word and is stored lower-cased so case
// variants of the same word count once. It reports whether guess was new.
func Guess(game *Game, guess string) bool {
	if game == nil || guess == "" {
		return false
	}
	if !isLetterGuess(guess) {
		guess = strings.ToLower(guess)
	}
	if slices.Contains(game.Guesses, guess) {
		return false
	}
	game.Guesses = append(game.Guesses, guess)
	won, lost := outcome(game)
	game.GameOver = won || lost
	return true
}

// outcome reports whether game has been won or lost. A game at the miss
// limit is lost even if the word is also fully revealed.
func outcome(game *Game) (won, lost bool) {
	_, wrong := calcMisses(game)
	if wrong >= HangmanMaxWrong {
		return false, true
	}
	return !slices.Contains(revealSlots(game), "_"), false
}

// PrintState renders the board for game. It returns "" for a nil game. It
// does not modify game; Guess sets GameOver.
func PrintState(game *Game) string {
	if game == nil {
		return ""
	}
	slots := revealSlots(game)
	misses, wrong := calcMisses(game)

	line := fmt.Sprintf("%s  %s", HangmanFaces[wrong], strings.Join(slots, " "))
	if len(misses) > 0 {
		line += "   ❌ " + strings.Join(misses, " ")
	}
	line += fmt.Sprintf(" (%d/%d)", wrong, HangmanMaxWrong)

	if won, lost := outcome(game); lost {
		line += fmt.Sprintf(" Game over! The word was %s", game.Word)
	} else if won {
		line += " You win! Use '!hangman' to start a new game"
	}

	return line
}

// isLetterGuess reports whether g is a single-letter guess. Anything longer
// is a guess at the whole word.
func isLetterGuess(g string) bool {
	return utf8.RuneCountInString(g) == 1
}

// isWordMatch reports whether word guess g is the secret word.
func isWordMatch(game *Game, g string) bool {
	return strings.EqualFold(g, game.Word)
}

// revealSlots returns one slot per letter of the word: the letter if it has
// been guessed, "_" otherwise. A correct word guess reveals every letter; a
// wrong one reveals nothing.
func revealSlots(game *Game) []string {
	guessed := make(map[rune]bool, len(game.Guesses))
	solved := false
	for _, g := range game.Guesses {
		lg := strings.ToLower(g)
		if isLetterGuess(lg) {
			for _, r := range lg {
				guessed[r] = true
			}
		} else if isWordMatch(game, lg) {
			solved = true
		}
	}

	slots := make([]string, 0, len(game.Word))
	for _, r := range game.Word {
		if solved || guessed[r] {
			slots = append(slots, string(r))
		} else {
			slots = append(slots, "_")
		}
	}
	return slots
}

// calcMisses figures out which guesses were wrong and how many misses that
// is. A wrong letter and a wrong word guess each count as one miss.
func calcMisses(game *Game) ([]string, int) {
	if game == nil {
		return []string{}, 0
	}

	var misses []string
	for _, g := range game.Guesses {
		lg := strings.ToLower(g)
		if isLetterGuess(lg) {
			if !strings.Contains(game.Word, lg) {
				misses = append(misses, lg)
			}
		} else if !isWordMatch(game, lg) {
			misses = append(misses, lg)
		}
	}
	return misses, min(len(misses), HangmanMaxWrong)
}
