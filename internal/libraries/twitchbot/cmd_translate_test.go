package twitchbot

import "testing"

func TestContainsJapanese(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"hello world", false},
		{"Kappa 123 !?", false},
		{"こんにちは", true},    // hiragana
		{"カタカナ", true},     // katakana
		{"日本語", true},      // kanji
		{"hello 世界", true}, // mixed
		{"", false},
	}
	for _, c := range cases {
		if got := containsJapanese(c.in); got != c.want {
			t.Errorf("containsJapanese(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsTranslateCommand(t *testing.T) {
	yes := []string{"!translate", "!translate hello", "!translate こんにちは"}
	no := []string{"!translated", "!tr hello", "translate hello", "x !translate y"}
	for _, m := range yes {
		if !isTranslateCommand(m) {
			t.Errorf("isTranslateCommand(%q) = false, want true", m)
		}
	}
	for _, m := range no {
		if isTranslateCommand(m) {
			t.Errorf("isTranslateCommand(%q) = true, want false", m)
		}
	}
}
