package hangman

import (
	"regexp"
	"slices"
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
