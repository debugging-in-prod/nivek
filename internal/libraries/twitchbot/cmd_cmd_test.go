package twitchbot

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
)

// fakeCmdCoreAPI implements only ManageChannelCommand / GetChannelCommands; any
// other CoreAPIClient call panics on the nil embedded interface.
type fakeCmdCoreAPI struct {
	api.CoreAPIClient

	mu       sync.Mutex
	reqs     []commands.ChatManageRequest
	res      commands.ChatManageResult
	err      error
	channel  []commands.Commands
	reloaded chan string
}

func (f *fakeCmdCoreAPI) ManageChannelCommand(req commands.ChatManageRequest) (commands.ChatManageResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, req)
	return f.res, f.err
}

func (f *fakeCmdCoreAPI) GetChannelCommands(id string) ([]commands.Commands, error) {
	if f.reloaded != nil {
		defer func() { f.reloaded <- id }()
	}
	return f.channel, nil
}

func (f *fakeCmdCoreAPI) requests() []commands.ChatManageRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]commands.ChatManageRequest(nil), f.reqs...)
}

func cmdBot(core *fakeCmdCoreAPI, seeded bool) (*Bot, *atomic.Int32, *fakeClock) {
	var fish atomic.Int32
	b, clk := dispatchBot(&fish)
	b.coreAPI = core
	if seeded {
		b.addTestCommand(cmdTrigger, (*Bot).handleCmdCommand, 3)
	}
	return b, &fish, clk
}

func drainSay(b *Bot) []string {
	var out []string
	for {
		select {
		case r := <-b.sayQueue:
			out = append(out, r.message)
		default:
			return out
		}
	}
}

func TestParseCmdArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in                      string
		action, trigger, answer string
		ok                      bool
	}{
		{"!cmd add !Hi Hello  There, friend!", "add", "!hi", "Hello  There, friend!", true},
		{"!CMD edit !hi   new text ", "edit", "!hi", "new text", true},
		{"!cmd del !hi", "delete", "!hi", "", true},
		{"!cmd delete !hi extra", "delete", "!hi", "", true},
		{"!cmd remove hi", "delete", "hi", "", true},
		{"!cmd add !hi", "", "", "", false},
		{"!cmd edit !hi   ", "", "", "", false},
		{"!cmd del", "", "", "", false},
		{"!cmd", "", "", "", false},
		{"!cmd frob !hi x", "", "", "", false},
		{"!cmdx add !hi x", "", "", "", false},
		{"!cmd add !so Go follow ${touser} (${count})", "add", "!so", "Go follow ${touser} (${count})", true},
		{"!cmd edit !r ${random.pick 'a b' \"c\"}", "edit", "!r", "${random.pick 'a b' \"c\"}", true},
	}
	for _, c := range cases {
		a, tr, r, ok := parseCmdArgs(c.in)
		if ok != c.ok || a != c.action || tr != c.trigger || r != c.answer {
			t.Errorf("parseCmdArgs(%q) = (%q,%q,%q,%v), want (%q,%q,%q,%v)",
				c.in, a, tr, r, ok, c.action, c.trigger, c.answer, c.ok)
		}
	}
}

func TestIsCmdCommand(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]bool{
		"!cmd":          true,
		"!cmd add !x y": true,
		"!cmd\tdel !x":  true,
		"!cmds":         false,
		"hi !cmd":       false,
		"":              false,
	} {
		if got := isCmdCommand(in); got != want {
			t.Errorf("isCmdCommand(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestApplyCustomCommandChange(t *testing.T) {
	t.Parallel()
	b := &Bot{customCommands: map[string]map[string]commands.Commands{
		"chan": {"!old": {Trigger: "!old", ResponseTmpl: ptr("x"), Enabled: true}},
	}}

	cmd := commands.Commands{Trigger: "!new", ResponseTmpl: ptr("hello"), Enabled: true, CooldownSecs: 10}
	if b.applyCustomCommandChange("Chan", commands.ChatManageResult{OK: true, Action: "add", Trigger: "!new", Command: &cmd}) {
		t.Fatal("loaded set should not need a reload")
	}
	if got, ok := b.customCommandFor("chan", "!new"); !ok || *got.ResponseTmpl != "hello" {
		t.Fatalf("added command not installed: %+v %v", got, ok)
	}

	edited := commands.Commands{Trigger: "!new", ResponseTmpl: ptr("bye"), Enabled: true}
	b.applyCustomCommandChange("chan", commands.ChatManageResult{OK: true, Action: "edit", Trigger: "!new", Command: &edited})
	if got, _ := b.customCommandFor("chan", "!new"); *got.ResponseTmpl != "bye" {
		t.Fatalf("edit not applied: %q", *got.ResponseTmpl)
	}

	// Editing a disabled command removes it from dispatch.
	disabled := commands.Commands{Trigger: "!new", ResponseTmpl: ptr("bye"), Enabled: false}
	b.applyCustomCommandChange("chan", commands.ChatManageResult{OK: true, Action: "edit", Trigger: "!new", Command: &disabled})
	if _, ok := b.customCommandFor("chan", "!new"); ok {
		t.Fatal("disabled command must not stay dispatchable")
	}

	b.applyCustomCommandChange("chan", commands.ChatManageResult{OK: true, Action: "delete", Trigger: "!old"})
	if _, ok := b.customCommands["chan"]; ok {
		t.Fatal("empty set should be evicted")
	}

	// No set loaded: install the command and ask for a full reload.
	if !b.applyCustomCommandChange("other", commands.ChatManageResult{OK: true, Action: "add", Trigger: "!new", Command: &cmd}) {
		t.Fatal("unloaded channel should request a reload")
	}
	if _, ok := b.customCommandFor("other", "!new"); !ok {
		t.Fatal("command should work before the reload lands")
	}
	// Deleting from an unloaded channel is a no-op and needs no reload.
	if b.applyCustomCommandChange("nobody", commands.ChatManageResult{OK: true, Action: "delete", Trigger: "!x"}) {
		t.Fatal("delete should never request a reload")
	}
}

func TestCmdAddFromChatUpdatesMapAndConfirms(t *testing.T) {
	t.Parallel()
	saved := commands.Commands{Trigger: "!yo", ResponseTmpl: ptr("Yo !fish chat"), Enabled: true, CooldownSecs: 10}
	core := &fakeCmdCoreAPI{res: commands.ChatManageResult{OK: true, Action: "add", Trigger: "!yo", Command: &saved}}
	b, fish, _ := cmdBot(core, true)

	b.handleWebhookMessage(chatEvent("m1", "50", "modguy", "!cmd add !YO Yo !fish chat", true))

	reqs := core.requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 core-api call, got %d", len(reqs))
	}
	if r := reqs[0]; r.ChannelTwitchID != "9" || r.Action != "add" || r.Trigger != "!yo" || r.Response != "Yo !fish chat" {
		t.Fatalf("bad request: %+v", r)
	}
	if fish.Load() != 0 {
		t.Fatal("words inside the !cmd response must not dispatch other commands")
	}
	if got := drainSay(b); len(got) != 1 || got[0] != "@modguy added !yo" {
		t.Fatalf("replies = %q", got)
	}
	if _, ok := b.customCommandFor("somechannel", "!yo"); !ok {
		t.Fatal("new command should be live without a go-live")
	}

	// Works right away for viewers.
	b.handleWebhookMessage(chatEvent("m2", "42", "viewer", "!yo", false))
	if got := drainSay(b); len(got) != 1 || got[0] != "Yo !fish chat" {
		t.Fatalf("custom reply = %q", got)
	}
}

func TestCmdNonModIsSilent(t *testing.T) {
	t.Parallel()
	core := &fakeCmdCoreAPI{}
	b, fish, _ := cmdBot(core, true)

	b.handleWebhookMessage(chatEvent("m1", "42", "viewer", "!cmd add !x hello !fish", false))
	b.handleWebhookMessage(chatEvent("m2", "42", "viewer", "!cmd", false))
	if n := len(core.requests()); n != 0 {
		t.Fatalf("non-mod reached core-api %d times", n)
	}
	if got := drainSay(b); len(got) != 0 {
		t.Fatalf("non-mod got replies: %q", got)
	}
	if fish.Load() != 0 {
		t.Fatal("non-mod !cmd body must not dispatch other commands")
	}

	// A non-mod attempt must not burn the cooldown for a mod.
	core.res = commands.ChatManageResult{OK: true, Action: "delete", Trigger: "!x"}
	b.handleWebhookMessage(chatEvent("m3", "50", "modguy", "!cmd del !x", true))
	if n := len(core.requests()); n != 1 {
		t.Fatalf("mod request blocked after a non-mod attempt: %d calls", n)
	}
}

func TestCmdBroadcasterAllowedWithoutModBadge(t *testing.T) {
	t.Parallel()
	core := &fakeCmdCoreAPI{res: commands.ChatManageResult{OK: true, Action: "delete", Trigger: "!x"}}
	b, _, _ := cmdBot(core, true)
	b.handleWebhookMessage(chatEvent("m1", "9", "somechannel", "!cmd del !x", false))
	if n := len(core.requests()); n != 1 {
		t.Fatalf("broadcaster should be allowed, got %d calls", n)
	}
}

func TestCmdRefusalAndErrorReplies(t *testing.T) {
	t.Parallel()
	core := &fakeCmdCoreAPI{res: commands.ChatManageResult{OK: false, Action: "add", Error: "!fish is a built-in command and can't be used as a custom trigger"}}
	b, _, clk := cmdBot(core, true)
	b.handleWebhookMessage(chatEvent("m1", "50", "modguy", "!cmd add !fish nope", true))
	if got := drainSay(b); len(got) != 1 || got[0] != "@modguy !fish is a built-in command and can't be used as a custom trigger" {
		t.Fatalf("refusal reply = %q", got)
	}

	core.err = errors.New("core-api down")
	clk.advance(cooldownFloor)
	b.handleWebhookMessage(chatEvent("m2", "50", "modguy", "!cmd add !yo hi", true))
	got := drainSay(b)
	if len(got) != 1 || !strings.Contains(got[0], "couldn't save") {
		t.Fatalf("internal error reply = %q", got)
	}
	if _, ok := b.customCommandFor("somechannel", "!yo"); ok {
		t.Fatal("a failed save must not touch the in-memory map")
	}
}

func TestCmdUsageReply(t *testing.T) {
	t.Parallel()
	core := &fakeCmdCoreAPI{}
	b, _, _ := cmdBot(core, true)
	b.handleWebhookMessage(chatEvent("m1", "50", "modguy", "!cmd add !x", true))
	got := drainSay(b)
	if len(got) != 1 || !strings.Contains(got[0], "usage:") {
		t.Fatalf("usage reply = %q", got)
	}
	if n := len(core.requests()); n != 0 {
		t.Fatalf("malformed !cmd reached core-api %d times", n)
	}
}

func TestCmdCooldownLimitsReplies(t *testing.T) {
	t.Parallel()
	core := &fakeCmdCoreAPI{}
	b, _, _ := cmdBot(core, true)
	for _, id := range []string{"m1", "m2", "m3"} {
		b.handleWebhookMessage(chatEvent(id, "50", "modguy", "!cmd", true))
	}
	if got := drainSay(b); len(got) != 1 {
		t.Fatalf("!cmd replied %d times inside the cooldown floor, want 1", len(got))
	}
}

func TestCmdInactiveWithoutSeedRow(t *testing.T) {
	t.Parallel()
	core := &fakeCmdCoreAPI{}
	b, _, _ := cmdBot(core, false)
	b.handleWebhookMessage(chatEvent("m1", "50", "modguy", "!cmd add !x hello", true))
	if n := len(core.requests()); n != 0 {
		t.Fatalf("unseeded !cmd reached core-api %d times", n)
	}
	if got := drainSay(b); len(got) != 0 {
		t.Fatalf("unseeded !cmd replied: %q", got)
	}
}

func TestCmdReloadsFullSetWhenNoneLoaded(t *testing.T) {
	t.Parallel()
	saved := commands.Commands{Trigger: "!yo", ResponseTmpl: ptr("yo"), Enabled: true}
	core := &fakeCmdCoreAPI{
		res:      commands.ChatManageResult{OK: true, Action: "add", Trigger: "!yo", Command: &saved},
		channel:  []commands.Commands{saved, {Trigger: "!other", ResponseTmpl: ptr("o"), Enabled: true}},
		reloaded: make(chan string, 1),
	}
	b, _, _ := cmdBot(core, true)
	delete(b.customCommands, "somechannel")

	b.handleWebhookMessage(chatEvent("m1", "50", "modguy", "!cmd add !yo yo", true))
	if id := <-core.reloaded; id != "9" {
		t.Fatalf("reloaded wrong channel %q", id)
	}
	// loadCustomCommands stores the set after GetChannelCommands returns; poll
	// briefly under the lock.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := b.customCommandFor("somechannel", "!other"); ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("full custom set was not reloaded")
}

// TestLoadCustomCommandsRecoversPanic: loadCustomCommands runs in its own
// goroutine, so a panic there would crash the whole bot. It must fail soft.
func TestLoadCustomCommandsRecoversPanic(t *testing.T) {
	t.Parallel()
	// A nil embedded CoreAPIClient panics on GetChannelCommands.
	b := &Bot{coreAPI: struct{ api.CoreAPIClient }{}, customCommands: map[string]map[string]commands.Commands{}}
	b.loadCustomCommands("9", "somechannel")
	if _, ok := b.customCommandFor("somechannel", "!x"); ok {
		t.Fatal("panicked load must not register commands")
	}
}

// TestSeededHandlerKeysAreRegistered guarantees a seed row can never ship
// without its Go handler: every builtin handler_key in database/init.sql must
// exist in builtinRegistry (an unknown key fails bot boot).
func TestSeededHandlerKeysAreRegistered(t *testing.T) {
	t.Parallel()
	sql, err := os.ReadFile("../../../database/init.sql")
	if err != nil {
		t.Fatalf("read init.sql: %v", err)
	}
	re := regexp.MustCompile(`\('(![a-z0-9_]+)',\s*'builtin',\s*'([a-z0-9_]+)'`)
	matches := re.FindAllStringSubmatch(string(sql), -1)
	if len(matches) == 0 {
		t.Fatal("no builtin seed rows found in init.sql")
	}
	seen := map[string]bool{}
	for _, m := range matches {
		if _, ok := builtinRegistry[m[2]]; !ok {
			t.Errorf("seed %s uses handler_key %q missing from builtinRegistry", m[1], m[2])
		}
		seen[m[1]] = true
	}
	if !seen[cmdTrigger] {
		t.Error("init.sql should seed !cmd")
	}
}
