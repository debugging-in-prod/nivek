package twitchbot

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
)

// addTestCommand installs one more global builtin on top of the current set
// (copy-on-write, like the real loader).
func (b *Bot) addTestCommand(trigger string, h commandHandler, cooldownSecs int) {
	handlers := map[string]commandHandler{}
	cooldowns := map[string]int{}
	if set := b.commands.Load(); set != nil {
		for k, v := range set.handlers {
			handlers[k] = v
		}
		for k, v := range set.cooldownSecs {
			cooldowns[k] = v
		}
	}
	handlers[trigger] = h
	if cooldownSecs > 0 {
		cooldowns[trigger] = cooldownSecs
	}
	b.setCommands(handlers, cooldowns)
}

// fastCommandRetry shrinks every loader wait for the test. Tests using it must
// not call t.Parallel (they swap package vars).
func fastCommandRetry(t *testing.T, bootSteps int) {
	t.Helper()
	oldBoot, oldBase, oldMax, oldSleep := bootCommandBackoff, backgroundCommandRetryBase, backgroundCommandRetryMax, commandRetrySleep
	bootCommandBackoff = make([]time.Duration, bootSteps)
	for i := range bootCommandBackoff {
		bootCommandBackoff[i] = time.Millisecond
	}
	backgroundCommandRetryBase = time.Millisecond
	backgroundCommandRetryMax = 2 * time.Millisecond
	commandRetrySleep = time.Sleep
	t.Cleanup(func() {
		bootCommandBackoff, backgroundCommandRetryBase, backgroundCommandRetryMax, commandRetrySleep = oldBoot, oldBase, oldMax, oldSleep
	})
}

// flakyCoreAPI is a real core-api HTTP server stand-in: GET /api/commands
// returns 503 for the first failN calls, then the given rows.
func flakyCoreAPI(t *testing.T, failN int32, rows []commands.Commands) (api.CoreAPIClient, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/commands" {
			http.NotFound(w, r)
			return
		}
		if calls.Add(1) <= failN {
			http.Error(w, `{"error":"down"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"commands": rows})
	}))
	t.Cleanup(srv.Close)
	client, err := api.NewCoreAPIClient(srv.URL, "00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("NewCoreAPIClient: %s", err)
	}
	return client, &calls
}

var loaderRows = []commands.Commands{
	{Trigger: "!pbcommands", Kind: "builtin", HandlerKey: ptr("pb_commands"), Enabled: true, CooldownSecs: 7},
	{Trigger: "!ghost", Kind: "builtin", HandlerKey: ptr("not_shipped_yet"), Enabled: true},
}

func TestLoadCommandsAtBootRetriesThenSucceeds(t *testing.T) {
	fastCommandRetry(t, 4)
	core, calls := flakyCoreAPI(t, 2, loaderRows)
	b := &Bot{coreAPI: core}

	if !b.loadCommandsAtBoot() {
		t.Fatal("expected boot load to succeed on the 3rd attempt")
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("core-api calls = %d, want 3", got)
	}
	if _, ok := b.lookupCommand("!pbcommands"); !ok {
		t.Fatal("!pbcommands should be loaded")
	}
	if _, ok := b.lookupCommand("!ghost"); ok {
		t.Fatal("a row with an unknown handler_key must be skipped, not installed")
	}
	if got := b.builtinCooldownSecs("!pbcommands"); got != 7 {
		t.Fatalf("cooldown = %d, want 7", got)
	}
}

func TestLoadCommandsAtBootGivesUpBounded(t *testing.T) {
	fastCommandRetry(t, 3)
	core, calls := flakyCoreAPI(t, 1_000, loaderRows)
	b := &Bot{coreAPI: core}

	if b.loadCommandsAtBoot() {
		t.Fatal("boot load should report failure while core-api is down")
	}
	if got := calls.Load(); got != 4 {
		t.Fatalf("core-api calls = %d, want 4 (1 + len(backoff))", got)
	}
	if b.commandsLoaded() {
		t.Fatal("no command set should be installed after a failed boot")
	}
	if _, ok := b.lookupCommand("!pbcommands"); ok || b.cmdActive() || b.builtinCooldownSecs("!pbcommands") != 0 {
		t.Fatal("an unloaded bot must behave as if no builtins exist")
	}
}

func TestBootBackoffIsBounded(t *testing.T) {
	var total time.Duration
	for _, d := range bootCommandBackoff {
		if d > 30*time.Second {
			t.Fatalf("boot backoff step %s exceeds 30s", d)
		}
		total += d
	}
	if total < time.Minute || total > 3*time.Minute {
		t.Fatalf("total boot backoff %s, want ~2 min", total)
	}
	if backgroundCommandRetryMax != 30*time.Second {
		t.Fatalf("background retry cap = %s, want 30s", backgroundCommandRetryMax)
	}
}

type panicCoreAPI struct {
	api.CoreAPIClient
	calls atomic.Int32
}

func (p *panicCoreAPI) GetGlobalEnabledCommands() ([]commands.Commands, error) {
	switch p.calls.Add(1) {
	case 1:
		panic("boom")
	case 2:
		return nil, errors.New("still down")
	}
	return loaderRows, nil
}

func TestBackgroundRetrySurvivesPanicAndInstalls(t *testing.T) {
	fastCommandRetry(t, 0)
	core := &panicCoreAPI{}
	b := &Bot{coreAPI: core}

	done := make(chan struct{})
	go func() {
		b.retryCommandsInBackground()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("background loader never finished")
	}
	if _, ok := b.lookupCommand("!pbcommands"); !ok {
		t.Fatal("background loader should install the commands once core-api recovers")
	}
	if got := core.calls.Load(); got != 3 {
		t.Fatalf("calls = %d, want 3", got)
	}
}

// Dispatch workers read the command set while the background loader swaps it
// in. Run with -race. Before the swap nothing replies; after it, the central
// cooldown still caps the builtin to one reply per window.
func TestCommandSetSwapIsRaceFree(t *testing.T) {
	fastCommandRetry(t, 0)
	core, _ := flakyCoreAPI(t, 5, loaderRows)
	var unused atomic.Int32
	b, _ := dispatchBot(&unused)
	b.commands.Store(nil) // start like a bot whose boot load failed
	b.coreAPI = core
	b.sayQueue = make(chan sayRequest, 1024)

	loaded := make(chan struct{})
	go func() {
		b.retryCommandsInBackground()
		close(loaded)
	}()

	var wg sync.WaitGroup
	var n atomic.Int32
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				id := n.Add(1)
				b.handleWebhookMessage(chatEvent(
					"m"+strconv.Itoa(int(id)), "u"+strconv.Itoa(w), "chatter", "!pbcommands", false))
				_ = b.builtinCooldownSecs("!pbcommands")
				_ = b.cmdActive()
			}
		}(w)
	}
	<-loaded
	wg.Wait()

	// One more after the swap is guaranteed to see the loaded set (it may be
	// on cooldown already if a worker fired post-swap; either way <= 1 reply).
	b.handleWebhookMessage(chatEvent("final", "u-final", "chatter", "!pbcommands", false))
	if got := len(b.sayQueue); got != 1 {
		t.Fatalf("replies = %d, want exactly 1 (cooldown-capped)", got)
	}
}
