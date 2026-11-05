package twitchbot

import (
	"testing"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/hangman"
)

// fakeHangmanAPI records PushHangmanGameState calls. Any other method panics
// via the nil embedded interface.
type fakeHangmanAPI struct {
	api.CoreAPIClient
	pushed chan string
}

func (f *fakeHangmanAPI) PushHangmanGameState(_ *hangman.Game, channel string) error {
	f.pushed <- channel
	return nil
}

func newHangmanBot() (*Bot, *fakeHangmanAPI) {
	fake := &fakeHangmanAPI{pushed: make(chan string, 4)}
	return &Bot{
		sayQueue:     make(chan sayRequest, 8),
		hangmanGames: make(map[string]*hangman.Game),
		coreAPI:      fake,
	}, fake
}

func hangmanMsg(text string) *chatMessageEvent {
	return &chatMessageEvent{
		BroadcasterUserId:    "123",
		BroadcasterUserLogin: "chan",
		Message:              chatMessageBody{Text: text},
	}
}

// oneRune builds a one-rune string, so invisible test inputs stay visible in source.
func oneRune(code rune) string { return string(code) }

func TestStripInvisible(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{oneRune(0x034F), ""},
		{"a" + oneRune(0x200B) + oneRune(0x200C) + oneRune(0x200D), "a"},
		{oneRune(0x2060) + "e" + oneRune(0xFEFF), "e"},
		{oneRune(0x07) + "x", "x"},
		{"a b", "a b"},
		{oneRune(0xE9), oneRune(0xE9)},
	}
	for _, tc := range cases {
		if got := stripInvisible(tc.in); got != tc.want {
			t.Errorf("stripInvisible(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHangmanGuessWithNoGame(t *testing.T) {
	t.Parallel()
	for _, ended := range []bool{false, true} {
		b, _ := newHangmanBot()
		if ended {
			b.hangmanGames["chan"] = &hangman.Game{Word: "apple", GameOver: true}
		}

		b.handleHangmanCommand(hangmanMsg("!hangman e"))

		got := drainSay(b)
		if len(got) != 1 || got[0] != hangmanNoGameMsg {
			t.Errorf("ended=%v: replies = %q, want [%q]", ended, got, hangmanNoGameMsg)
		}
	}
}

func TestHangmanInvisibleArgStartsGame(t *testing.T) {
	t.Parallel()
	b, fake := newHangmanBot()

	b.handleHangmanCommand(hangmanMsg("!hangman " + oneRune(0x034F)))

	select {
	case ch := <-fake.pushed:
		if ch != "chan" {
			t.Errorf("pushed state for %q, want chan", ch)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("new game state was not pushed")
	}
	if g := b.hangmanGames["chan"]; g == nil || g.GameOver {
		t.Fatalf("game = %+v, want a running game", g)
	}
	got := drainSay(b)
	if len(got) != 2 || got[0] != "New game started!" {
		t.Errorf("replies = %q, want new-game message then board", got)
	}
}

func TestPrintStateNil(t *testing.T) {
	t.Parallel()
	if got := hangman.PrintState(nil); got != "" {
		t.Errorf("PrintState(nil) = %q, want empty", got)
	}
}
