package hangman

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

var wordPattern = regexp.MustCompile(`^[a-z]{5,8}$`)

func TestWordListFormat(t *testing.T) {
	for i, w := range wordList {
		if !wordPattern.MatchString(w) {
			t.Errorf("word %d %q does not match %s", i, w, wordPattern)
		}
	}
}

func TestWordListNoDuplicates(t *testing.T) {
	seen := make(map[string]bool, len(wordList))
	for _, w := range wordList {
		if seen[w] {
			t.Errorf("duplicate word %q", w)
		}
		seen[w] = true
	}
}

func TestWordListSize(t *testing.T) {
	const minWords = 1500
	if len(wordList) < minWords {
		t.Fatalf("wordbank has %d words, want at least %d", len(wordList), minWords)
	}
}

func TestWordListBlocklist(t *testing.T) {
	// A small sample of words that must never be a hangman answer. This is a
	// regression guard, not the full filter the list was built with.
	blocklist := []string{
		"bastard", "bitch", "bitches", "boobs", "cracker", "fucker",
		"hooker", "penis", "pussy", "rapist", "retard",
		"shitty", "slutty", "vagina", "whore", "whores",
	}
	for _, w := range blocklist {
		if slices.Contains(wordList, w) {
			t.Errorf("blocked word %q is in the wordbank", w)
		}
	}
}

func TestStartNewGameUsesWordbank(t *testing.T) {
	for range 50 {
		game := StartNewGame()
		if game == nil {
			t.Fatal("StartNewGame returned nil with a non-empty wordbank")
		}
		if !slices.Contains(wordList, game.Word) {
			t.Fatalf("game word %q is not in the wordbank", game.Word)
		}
		if game.GameOver || len(game.Guesses) != 0 {
			t.Fatalf("new game is not fresh: %+v", game)
		}
	}
}

func TestStartNewGameEmptyWordbank(t *testing.T) {
	saved := wordList
	t.Cleanup(func() { wordList = saved })

	wordList = nil
	if game := StartNewGame(); game != nil {
		t.Fatalf("StartNewGame with empty wordbank = %+v, want nil", game)
	}
}

func playGuesses(game *Game, guesses ...string) string {
	var state string
	for _, g := range guesses {
		Guess(game, g)
		state = PrintState(game)
	}
	return state
}

func TestWordGuessCorrectWins(t *testing.T) {
	game := &Game{Word: "apple", Guesses: []string{}}

	state := playGuesses(game, "APPLE")

	if !strings.Contains(state, "a p p l e") || !strings.Contains(state, "You win!") {
		t.Fatalf("state = %q, want full word revealed and a win", state)
	}
	if !strings.Contains(state, "(0/6)") {
		t.Errorf("state = %q, want no misses", state)
	}
	if !game.GameOver {
		t.Error("GameOver = false after a correct word guess")
	}
}

func TestWordGuessWrongCostsOneMissRevealsNothing(t *testing.T) {
	game := &Game{Word: "apple", Guesses: []string{}}

	// "lapel" shares every letter with "apple" but must reveal none of them.
	state := playGuesses(game, "lapel")

	if !strings.Contains(state, "_ _ _ _ _") {
		t.Errorf("state = %q, want nothing revealed", state)
	}
	if !strings.Contains(state, "❌ lapel") || !strings.Contains(state, "(1/6)") {
		t.Errorf("state = %q, want exactly one miss for lapel", state)
	}
	if game.GameOver {
		t.Error("GameOver = true after one wrong word guess")
	}

	// Repeating the same wrong word, in any case, doesn't cost another miss.
	state = playGuesses(game, "LAPEL")
	if !strings.Contains(state, "(1/6)") {
		t.Errorf("state = %q, want a repeated word guess to be ignored", state)
	}
}

func TestWordGuessWrongAtMissLimitLoses(t *testing.T) {
	game := &Game{Word: "apple", Guesses: []string{}}

	state := playGuesses(game, "z", "x", "q", "w", "k", "grape")

	if !strings.Contains(state, "(6/6)") || !strings.Contains(state, "Game over! The word was apple") {
		t.Fatalf("state = %q, want a loss at 6/6", state)
	}
	if !game.GameOver {
		t.Error("GameOver = false after the sixth miss")
	}
}

func TestLetterGuessesUnchanged(t *testing.T) {
	game := &Game{Word: "apple", Guesses: []string{}}

	state := playGuesses(game, "p", "z", "p")
	if !strings.Contains(state, "_ p p _ _") || !strings.Contains(state, "❌ z") || !strings.Contains(state, "(1/6)") {
		t.Fatalf("state = %q, want p revealed and one miss for z", state)
	}

	state = playGuesses(game, "a", "l", "e")
	if !strings.Contains(state, "You win!") || !game.GameOver {
		t.Fatalf("state = %q, want a win once every letter is guessed", state)
	}
}

func TestWordGuessWrongLengthRejected(t *testing.T) {
	for _, guess := range []string{"plea", "apples", strings.Repeat("x", 400)} {
		game := &Game{Word: "apple", Guesses: []string{"p"}}

		if !WrongLength(game, guess) {
			t.Errorf("WrongLength(%.10q) = false, want true", guess)
		}
		if Guess(game, guess) {
			t.Errorf("Guess(%.10q) = true, want rejected", guess)
		}
		if len(game.Guesses) != 1 || game.GameOver {
			t.Errorf("game = %+v after %.10q, want unchanged", game, guess)
		}
		if misses, wrong := calcMisses(game); len(misses) != 0 || wrong != 0 {
			t.Errorf("misses = %q (%d) after %.10q, want none", misses, wrong, guess)
		}
	}

	// Single letters and same-length words are not length-checked.
	game := &Game{Word: "apple"}
	for _, guess := range []string{"z", "lapel", "APPLE"} {
		if WrongLength(game, guess) {
			t.Errorf("WrongLength(%q) = true, want false", guess)
		}
	}
}
