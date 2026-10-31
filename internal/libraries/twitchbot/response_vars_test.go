package twitchbot

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
)

// fixedIntN always returns k (clamped to n-1) so random picks are deterministic.
func fixedIntN(k int) func(int) int {
	return func(n int) int { return min(k, n-1) }
}

func TestExpandResponse(t *testing.T) {
	t.Parallel()
	base := responseVars{
		User:    "Alice",
		Channel: "SomeChannel",
		Args:    []string{"@Bob", "two", "three"},
		Count:   func() (int, bool) { return 7, true },
		IntN:    fixedIntN(1),
	}
	noArgs := base
	noArgs.Args = nil
	countFails := base
	countFails.Count = func() (int, bool) { return 0, false }
	nilCount := base
	nilCount.Count = nil
	first := base
	first.IntN = fixedIntN(0)

	cases := []struct {
		name string
		tmpl string
		vars responseVars
		want string
	}{
		{"plain", "hello there", base, "hello there"},
		{"user", "hi ${user}!", base, "hi Alice!"},
		{"channel", "welcome to ${channel}", base, "welcome to SomeChannel"},
		{"touser strips @", "${user} hugs ${touser}", base, "Alice hugs Bob"},
		{"touser strips many @", "${touser}", responseVars{User: "A", Args: []string{"@@x"}}, "x"},
		{"touser falls back", "${user} hugs ${touser}", noArgs, "Alice hugs Alice"},
		{"touser bare @ falls back", "${touser}", responseVars{User: "A", Args: []string{"@"}}, "A"},
		{"positional", "${1}|${2}|${3}|${4}|${9}", base, "@Bob|two|three||"},
		{"positional no args", "[${1}]", noArgs, "[]"},
		{"zero not positional", "${0}", base, "${0}"},
		{"two digits not positional", "${10}", base, "${10}"},
		{"count", "used ${count} times", base, "used 7 times"},
		{"count failure stays literal", "used ${count} times", countFails, "used ${count} times"},
		{"nil count stays literal", "${count}", nilCount, "${count}"},
		{"pick single quotes", "${random.pick 'a' 'b' 'c'}", base, "b"},
		{"pick double quotes", `${random.pick "x y" "z"}`, first, "x y"},
		{"pick mixed quotes", `${random.pick 'a' "b"}`, base, "b"},
		{"pick brace in quotes", "${random.pick '}' 'x'}", first, "}"},
		{"pick empty option", "[${random.pick ''}]", base, "[]"},
		{"pick no options", "${random.pick}", base, "${random.pick}"},
		{"pick unquoted", "${random.pick a b}", base, "${random.pick a b}"},
		{"pick unterminated", "${random.pick 'a}", base, "${random.pick 'a}"},
		{"pick no space", "${random.pick'a'}", base, "${random.pick'a'}"},
		{"pick adjacent", "${random.pick 'a''b'}", base, "${random.pick 'a''b'}"},
		{"range", "${random.1-100}", base, "2"},
		{"range single", "${random.5-5}", base, "5"},
		{"range reversed", "${random.10-1}", base, "${random.10-1}"},
		{"range negative", "${random.-5-5}", base, "${random.-5-5}"},
		{"range too big", "${random.1-1000000001}", base, "${random.1-1000000001}"},
		{"range junk", "${random.a-b}", base, "${random.a-b}"},
		{"range missing dash", "${random.100}", base, "${random.100}"},
		{"unknown literal", "hi ${nope} ${user}", base, "hi ${nope} Alice"},
		{"unclosed literal", "hi ${user", base, "hi ${user"},
		{"unclosed then valid", "${ ${user}", base, "${ Alice"},
		{"nested", "${x ${user}}", base, "${x Alice}"},
		{"dollar alone", "$5 and $ {user}", base, "$5 and $ {user}"},
		{"empty var", "${}", base, "${}"},
		{"adjacent", "${user}${user}", base, "AliceAlice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := expandResponse(tc.tmpl, tc.vars); got != tc.want {
				t.Fatalf("expandResponse(%q) = %q, want %q", tc.tmpl, got, tc.want)
			}
		})
	}
}

func TestExpandResponseNoReexpansion(t *testing.T) {
	t.Parallel()
	var countCalls atomic.Int32
	vars := responseVars{
		User:  "${count}",
		Args:  []string{"${user}", "${random.pick 'a'}"},
		Count: func() (int, bool) { countCalls.Add(1); return 1, true },
	}
	got := expandResponse("${user} ${1} ${2} ${touser}", vars)
	if want := "${count} ${user} ${random.pick 'a'} ${user}"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if countCalls.Load() != 0 {
		t.Fatalf("substituted text triggered ${count} %d times", countCalls.Load())
	}
}

func TestExpandResponseCountCalledOnce(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	vars := responseVars{Count: func() (int, bool) { return int(calls.Add(1)), true }}
	if got := expandResponse("${count} ${count}", vars); got != "1 1" {
		t.Fatalf("got %q, want %q", got, "1 1")
	}
	if calls.Load() != 1 {
		t.Fatalf("Count called %d times, want 1", calls.Load())
	}
	calls.Store(0)
	expandResponse("no counter here ${user}", vars)
	if calls.Load() != 0 {
		t.Fatalf("Count called %d times for a template without ${count}", calls.Load())
	}
}

func TestExpandResponseRandomRangeBounds(t *testing.T) {
	t.Parallel()
	// Default (real) RNG must stay inside the inclusive range.
	for range 500 {
		got := expandResponse("${random.3-6}", responseVars{})
		if got != "3" && got != "4" && got != "5" && got != "6" {
			t.Fatalf("random.3-6 produced %q", got)
		}
	}
	if got := expandResponse("${random.0-1000000000}", responseVars{IntN: fixedIntN(1 << 40)}); got != "1000000000" {
		t.Fatalf("max bound produced %q", got)
	}
}

func TestExpandResponseOutputBounded(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("x", 500)
	tmpl := strings.Repeat("${1}", 1000)
	got := expandResponse(tmpl, responseVars{Args: []string{big}})
	if len(got) > maxExpandedBytes {
		t.Fatalf("expanded to %d bytes, cap is %d", len(got), maxExpandedBytes)
	}
}

func TestCommandArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		text, trigger string
		want          []string
	}{
		{"!hug @Bob", "!hug", []string{"@Bob"}},
		{"!HUG  Bob   Carol", "!hug", []string{"Bob", "Carol"}},
		{"hey chat !hug Bob", "!hug", []string{"Bob"}},
		{"!hug", "!hug", nil},
		{"nothing", "!hug", nil},
		{"!hug 1 2 3 4 5 6 7 8 9 10 11", "!hug", []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}},
	}
	for _, tc := range cases {
		got := commandArgs(tc.text, tc.trigger)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") || len(got) != len(tc.want) {
			t.Errorf("commandArgs(%q, %q) = %q, want %q", tc.text, tc.trigger, got, tc.want)
		}
	}
}

func TestSanitizeChatReply(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"hello", "hello"},
		{"/ban someone", "ban someone"},
		{".timeout someone 600", "timeout someone 600"},
		{"  /./ /me waves", "me waves"},
		{"line1\nline2\r\n/ban x", "line1 line2  /ban x"},
		{"\n/ban x", "ban x"},
		{"//", ""},
		{"   ", ""},
		{"ok trailing   ", "ok trailing"},
		{"a/b.c", "a/b.c"},
		{"bad\xffbyte", "bad byte"},
	}
	for _, tc := range cases {
		if got := sanitizeChatReply(tc.in); got != tc.want {
			t.Errorf("sanitizeChatReply(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	long := strings.Repeat("é", 1000)
	got := sanitizeChatReply(long)
	if n := utf8.RuneCountInString(got); n != maxChatReplyRunes {
		t.Fatalf("capped reply has %d runes, want %d", n, maxChatReplyRunes)
	}
	if !utf8.ValidString(got) {
		t.Fatal("capped reply is not valid UTF-8")
	}
}

// fakeUseCounter implements just IncrementCommandUse; any other CoreAPIClient
// call panics via the nil embedded interface.
type fakeUseCounter struct {
	api.CoreAPIClient
	calls atomic.Int32
	err   error
	gotID int
	gotCh string
}

func (f *fakeUseCounter) IncrementCommandUse(_ context.Context, id int, ch string) (int, error) {
	n := f.calls.Add(1)
	f.gotID, f.gotCh = id, ch
	if f.err != nil {
		return 0, f.err
	}
	return 41 + int(n), nil
}

func customVarsBot(tmpl string, fake *fakeUseCounter) *Bot {
	var calls atomic.Int32
	b, _ := dispatchBot(&calls)
	b.customCommands["somechannel"]["!hug"] = commands.Commands{
		Id: 12, Trigger: "!hug", ResponseTmpl: ptr(tmpl), MinRole: "everyone",
	}
	if fake != nil {
		b.coreAPI = fake
	}
	return b
}

func TestCustomCommandExpandsVariables(t *testing.T) {
	t.Parallel()
	fake := &fakeUseCounter{}
	b := customVarsBot("${user} hugs ${touser} (#${count})", fake)

	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!hug @Bob", false))
	if n := len(b.sayQueue); n != 1 {
		t.Fatalf("queued %d replies, want 1", n)
	}
	req := <-b.sayQueue
	if req.message != "alice hugs Bob (#42)" || req.broadcasterId != "9" {
		t.Fatalf("reply = %+v", req)
	}
	if fake.gotID != 12 || fake.gotCh != "9" {
		t.Fatalf("incremented command %d in %q, want 12 in \"9\"", fake.gotID, fake.gotCh)
	}

	// A second use inside the cooldown is dropped and doesn't bump the counter.
	b.handleWebhookMessage(chatEvent("m2", "43", "bob", "!hug", false))
	if len(b.sayQueue) != 0 || fake.calls.Load() != 1 {
		t.Fatalf("cooldown not enforced: queued=%d increments=%d", len(b.sayQueue), fake.calls.Load())
	}
}

func TestCustomCommandCountFailureStillReplies(t *testing.T) {
	t.Parallel()
	fake := &fakeUseCounter{err: errors.New("core-api down")}
	b := customVarsBot("count is ${count}", fake)
	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!hug", false))
	if len(b.sayQueue) != 1 {
		t.Fatalf("queued %d replies, want 1", len(b.sayQueue))
	}
	if got := (<-b.sayQueue).message; got != "count is ${count}" {
		t.Fatalf("reply = %q", got)
	}
}

func TestCustomCommandNilCoreAPIStillReplies(t *testing.T) {
	t.Parallel()
	b := customVarsBot("count is ${count}", nil)
	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!hug", false))
	if got := (<-b.sayQueue).message; got != "count is ${count}" {
		t.Fatalf("reply = %q", got)
	}
}

func TestCustomCommandReplyCannotBecomeChatCommand(t *testing.T) {
	t.Parallel()
	b := customVarsBot("${1} ${2}", nil)
	b.handleWebhookMessage(chatEvent("m1", "42", "alice", "!hug /ban victim", false))
	if got := (<-b.sayQueue).message; got != "ban victim" {
		t.Fatalf("reply = %q, want leading '/' stripped", got)
	}

	// A reply that sanitizes to nothing isn't sent at all.
	b2 := customVarsBot("${1}", nil)
	b2.handleWebhookMessage(chatEvent("m2", "42", "alice", "!hug", false))
	if len(b2.sayQueue) != 0 {
		t.Fatalf("empty reply was queued: %q", (<-b2.sayQueue).message)
	}
}

func TestCustomCommandIgnoresBotAndNoReplyChatters(t *testing.T) {
	t.Parallel()
	fake := &fakeUseCounter{}
	b := customVarsBot("${count}", fake)
	b.handleWebhookMessage(chatEvent("m1", "1", "peanutbudderbot", "!hug", false)) // the bot itself
	if len(b.sayQueue) != 0 || fake.calls.Load() != 0 {
		t.Fatalf("replied to the bot itself: queued=%d increments=%d", len(b.sayQueue), fake.calls.Load())
	}
}
