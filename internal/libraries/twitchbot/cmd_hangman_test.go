package twitchbot

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/hangman"
)

// hangmanPush is one recorded PushHangmanGameState call.
type hangmanPush struct {
	channel string
	game    hangman.Game
}

// fakeHangmanAPI records PushHangmanGameState calls. Any other method panics
// via the nil embedded interface.
type fakeHangmanAPI struct {
	api.CoreAPIClient
	pushed chan hangmanPush
}

func (f *fakeHangmanAPI) PushHangmanGameState(game *hangman.Game, channel string) error {
	f.pushed <- hangmanPush{channel: channel, game: *game}
	return nil
}

// nextPush waits for the next recorded push.
func (f *fakeHangmanAPI) nextPush(t *testing.T) hangmanPush {
	t.Helper()
	select {
	case p := <-f.pushed:
		return p
	case <-time.After(2 * time.Second):
		t.Fatal("game state was not pushed")
		return hangmanPush{}
	}
}

// noPush asserts nothing more is pushed.
func (f *fakeHangmanAPI) noPush(t *testing.T) {
	t.Helper()
	select {
	case p := <-f.pushed:
		t.Fatalf("unexpected push %+v", p)
	case <-time.After(100 * time.Millisecond):
	}
}

func newHangmanBot() (*Bot, *fakeHangmanAPI) {
	fake := &fakeHangmanAPI{pushed: make(chan hangmanPush, 16)}
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

	if p := fake.nextPush(t); p.channel != "chan" {
		t.Errorf("pushed state for %q, want chan", p.channel)
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

func TestHangmanGuessPushesState(t *testing.T) {
	t.Parallel()
	b, fake := newHangmanBot()
	b.hangmanGames["chan"] = &hangman.Game{Word: "apple", Guesses: []string{}}

	b.handleHangmanCommand(hangmanMsg("!hangman p"))

	p := fake.nextPush(t)
	if p.channel != "chan" || len(p.game.Guesses) != 1 || p.game.Guesses[0] != "p" || p.game.GameOver {
		t.Errorf("push = %+v, want running game with guess p", p)
	}

	// A repeated guess changes nothing, so nothing is pushed.
	b.handleHangmanCommand(hangmanMsg("!hangman p"))
	fake.noPush(t)
	if got := drainSay(b); len(got) != 2 {
		t.Errorf("replies = %q, want one board per guess", got)
	}
}

func TestHangmanWinPushesGameOver(t *testing.T) {
	t.Parallel()
	b, fake := newHangmanBot()
	b.hangmanGames["chan"] = &hangman.Game{Word: "apple", Guesses: []string{}}

	b.handleHangmanCommand(hangmanMsg("!hangman Apple"))

	if p := fake.nextPush(t); !p.game.GameOver {
		t.Errorf("push = %+v, want GameOver after a correct word guess", p)
	}
	if !b.hangmanGames["chan"].GameOver {
		t.Error("in-memory game not marked over after a win")
	}
	got := drainSay(b)
	if len(got) != 1 || !strings.Contains(got[0], "You win!") {
		t.Errorf("replies = %q, want one winning board", got)
	}
}

func TestHangmanLossPushesGameOver(t *testing.T) {
	t.Parallel()
	b, fake := newHangmanBot()
	b.hangmanGames["chan"] = &hangman.Game{Word: "apple", Guesses: []string{"z", "x", "q", "w", "k"}}

	b.handleHangmanCommand(hangmanMsg("!hangman grape"))

	p := fake.nextPush(t)
	if !p.game.GameOver || len(p.game.Guesses) != 6 {
		t.Errorf("push = %+v, want final lost state with 6 guesses", p)
	}
	got := drainSay(b)
	if len(got) != 1 || !strings.Contains(got[0], "Game over! The word was apple") {
		t.Errorf("replies = %q, want one losing board", got)
	}

	// The game is over, so a further guess gets the no-game reply.
	b.handleHangmanCommand(hangmanMsg("!hangman a"))
	fake.noPush(t)
	if got := drainSay(b); len(got) != 1 || got[0] != hangmanNoGameMsg {
		t.Errorf("replies = %q, want [%q]", got, hangmanNoGameMsg)
	}
}

func TestPrintStateDoesNotEndGame(t *testing.T) {
	t.Parallel()
	game := &hangman.Game{Word: "apple", Guesses: []string{"apple"}}
	if got := hangman.PrintState(game); !strings.Contains(got, "You win!") {
		t.Fatalf("PrintState = %q, want a winning board", got)
	}
	if game.GameOver {
		t.Error("PrintState set GameOver; only Guess should")
	}
}

func TestHangmanWrongLengthGuessRejected(t *testing.T) {
	t.Parallel()
	for _, guess := range []string{strings.Repeat("y", 400), "plea"} {
		b, fake := newHangmanBot()
		b.hangmanGames["chan"] = &hangman.Game{Word: "apple", Guesses: []string{"p"}}

		b.handleHangmanCommand(hangmanMsg("!hangman " + guess))

		fake.noPush(t)
		if g := b.hangmanGames["chan"]; len(g.Guesses) != 1 || g.GameOver {
			t.Errorf("%.10q: game = %+v, want unchanged", guess, g)
		}
		want := fmt.Sprintf(hangmanWrongLengthMsg, 5)
		got := drainSay(b)
		if len(got) != 1 || got[0] != want {
			t.Errorf("%.10q: replies = %q, want [%q]", guess, got, want)
		}
		if strings.Contains(hangman.PrintState(b.hangmanGames["chan"]), "❌") {
			t.Errorf("%.10q: board shows a miss, want none", guess)
		}
	}
}

func TestHangmanSameLengthWrongWordOneMiss(t *testing.T) {
	t.Parallel()
	b, fake := newHangmanBot()
	b.hangmanGames["chan"] = &hangman.Game{Word: "apple", Guesses: []string{}}

	b.handleHangmanCommand(hangmanMsg("!hangman grape"))

	if p := fake.nextPush(t); len(p.game.Guesses) != 1 || p.game.Guesses[0] != "grape" {
		t.Errorf("push = %+v, want one guess grape", p)
	}
	got := drainSay(b)
	if len(got) != 1 || !strings.Contains(got[0], "❌ grape (1/6)") {
		t.Errorf("replies = %q, want one board with exactly one miss", got)
	}
}

func TestHangmanStalePushDropped(t *testing.T) {
	t.Parallel()
	b, fake := newHangmanBot()
	newer := &hangman.Game{Word: "apple", Guesses: []string{"p", "a"}}
	older := &hangman.Game{Word: "apple", Guesses: []string{"p"}}

	// The newer snapshot lands first; the older one must not overwrite it.
	b.pushGameStateToDB(newer, "chan", 2)
	b.pushGameStateToDB(older, "chan", 1)

	if p := fake.nextPush(t); len(p.game.Guesses) != 2 {
		t.Errorf("push = %+v, want the newer snapshot", p)
	}
	fake.noPush(t)

	// Ordering is per channel.
	b.pushGameStateToDB(older, "other", 1)
	if p := fake.nextPush(t); p.channel != "other" {
		t.Errorf("push = %+v, want a push for other", p)
	}
}
