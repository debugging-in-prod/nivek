package twitchbot

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gempir/go-twitch-irc/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/twitcheventsub"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/user"
)

// fastChannelRetry shrinks every channel-loader wait (and the command loader's,
// since NewBot runs both). Tests using it must not call t.Parallel.
func fastChannelRetry(t *testing.T, bootSteps int) {
	t.Helper()
	fastCommandRetry(t, bootSteps)
	oldBoot, oldBase, oldMax, oldSleep := bootChannelBackoff, backgroundChannelRetryBase, backgroundChannelRetryMax, channelRetrySleep
	bootChannelBackoff = make([]time.Duration, bootSteps)
	for i := range bootChannelBackoff {
		bootChannelBackoff[i] = time.Millisecond
	}
	backgroundChannelRetryBase = time.Millisecond
	backgroundChannelRetryMax = 2 * time.Millisecond
	channelRetrySleep = time.Sleep
	t.Cleanup(func() {
		bootChannelBackoff, backgroundChannelRetryBase, backgroundChannelRetryMax, channelRetrySleep = oldBoot, oldBase, oldMax, oldSleep
	})
}

func testUser(id, login string, live bool) user.User {
	return user.User{TwitchID: ptr(id), TwitchLogin: ptr(login), IsLive: live, BotOptIn: true}
}

// fakeCoreAPI is a core-api HTTP stand-in. GET /api/bot/channels/active
// returns 500 for the first failN calls (or while down is set), then users.
// GET /api/commands follows the same down switch. Everything else is a 404 so
// the per-channel loaders log and carry on.
type fakeCoreAPI struct {
	client       api.CoreAPIClient
	channelCalls atomic.Int32
	down         atomic.Bool
}

func newFakeCoreAPI(t *testing.T, failN int32, users []user.User) *fakeCoreAPI {
	t.Helper()
	f := &fakeCoreAPI{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api" + api.GetActiveChannels:
			n := f.channelCalls.Add(1)
			if f.down.Load() || n <= failN {
				http.Error(w, `{"error":"down"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"channels": users})
		case "/api/commands":
			if f.down.Load() {
				http.Error(w, `{"error":"down"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"commands": loaderRows})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := api.NewCoreAPIClient(srv.URL, "00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("NewCoreAPIClient: %s", err)
	}
	f.client = client
	return f
}

// fakeIRC is a minimal Twitch IRC server: it welcomes the client after NICK,
// answers PINGs, and records every line the bot sends so tests can assert on
// JOINs and that no PRIVMSG (chat) was ever sent.
type fakeIRC struct {
	ln    net.Listener
	mu    sync.Mutex
	lines []string
}

func newFakeIRC(t *testing.T) *fakeIRC {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %s", err)
	}
	f := &fakeIRC{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	return f
}

func (f *fakeIRC) serve(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	for sc.Scan() {
		line := sc.Text()
		f.mu.Lock()
		f.lines = append(f.lines, line)
		f.mu.Unlock()
		switch {
		case strings.HasPrefix(line, "NICK "):
			_, _ = conn.Write([]byte(":tmi.twitch.tv 001 peanutbudderbot :Welcome, GLHF!\r\n"))
		case strings.HasPrefix(line, "PING"):
			_, _ = conn.Write([]byte(":tmi.twitch.tv PONG tmi.twitch.tv :tmi.twitch.tv\r\n"))
		}
	}
}

func (f *fakeIRC) joined() map[string]bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]bool{}
	for _, l := range f.lines {
		if !strings.HasPrefix(l, "JOIN ") {
			continue
		}
		for _, ch := range strings.Split(strings.TrimPrefix(l, "JOIN "), ",") {
			out[strings.TrimPrefix(ch, "#")] = true
		}
	}
	return out
}

func (f *fakeIRC) privmsgs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, l := range f.lines {
		if strings.HasPrefix(l, "PRIVMSG") {
			out = append(out, l)
		}
	}
	return out
}

// countingTransport fails (and counts) every outbound HTTP request, so a test
// can prove the Helix send path (say/announce) was never used.
type countingTransport struct{ n atomic.Int32 }

func (c *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	c.n.Add(1)
	return nil, http.ErrHandlerTimeout
}

// fakeEventSub stubs the two Twitch calls Start makes on the boot path. Any
// other method panics via the nil embedded interface.
type fakeEventSub struct {
	twitcheventsub.TwitchEventSubClient
	grantedID string
}

func (f *fakeEventSub) ListEventSubSubscriptions(context.Context) ([]twitcheventsub.EventSubSubscription, error) {
	var s twitcheventsub.EventSubSubscription
	s.Type = chatReadSubType
	s.Status = twitcheventsub.StatusEnabled
	s.Condition.BroadcasterUserID = f.grantedID
	return []twitcheventsub.EventSubSubscription{s}, nil
}

func (f *fakeEventSub) IsStreamLive(context.Context, string) (bool, error) { return false, nil }

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func trackedLogins(b *Bot) map[string]int {
	b.channelsMu.Lock()
	defer b.channelsMu.Unlock()
	out := map[string]int{}
	for _, c := range b.config.Channels {
		if c.TwitchLogin != nil {
			out[*c.TwitchLogin]++
		}
	}
	return out
}

var loaderUsers = []user.User{
	testUser("101", "livechannel", true),
	testUser("102", "offlinechannel", false),
	{TwitchID: ptr("103"), TwitchLogin: ptr("Bad Login!"), IsLive: true},
	{TwitchID: ptr("104")}, // no login
}

func TestLoadChannelsAtBootRetriesThenSucceeds(t *testing.T) {
	fastChannelRetry(t, 4)
	core := newFakeCoreAPI(t, 2, loaderUsers)

	channels, ok := LoadChannelsAtBoot(core.client)
	if !ok {
		t.Fatal("expected boot load to succeed on the 3rd attempt")
	}
	if got := core.channelCalls.Load(); got != 3 {
		t.Fatalf("core-api calls = %d, want 3", got)
	}
	if len(channels) != 2 || *channels[0].TwitchLogin != "livechannel" || *channels[1].TwitchLogin != "offlinechannel" {
		t.Fatalf("channels = %+v, want only the two valid logins", channels)
	}
}

func TestLoadChannelsAtBootGivesUpBounded(t *testing.T) {
	fastChannelRetry(t, 3)
	core := newFakeCoreAPI(t, 1_000, loaderUsers)

	channels, ok := LoadChannelsAtBoot(core.client)
	if ok || channels != nil {
		t.Fatalf("boot load should fail while core-api is down, got ok=%v channels=%v", ok, channels)
	}
	if got := core.channelCalls.Load(); got != 4 {
		t.Fatalf("core-api calls = %d, want 4 (1 + len(backoff))", got)
	}
}

func TestLoadChannelsAtBootEmptyListIsRetriedThenPending(t *testing.T) {
	for name, users := range map[string][]user.User{
		"empty":          {},
		"no valid login": {{TwitchID: ptr("1"), TwitchLogin: ptr("NOPE")}},
	} {
		t.Run(name, func(t *testing.T) {
			fastChannelRetry(t, 2)
			core := newFakeCoreAPI(t, 0, users)
			if _, ok := LoadChannelsAtBoot(core.client); ok {
				t.Fatal("an empty channel list must not count as loaded")
			}
			if got := core.channelCalls.Load(); got != 3 {
				t.Fatalf("core-api calls = %d, want 3 (empty list is retried)", got)
			}
		})
	}
}

type panicChannelsAPI struct{ api.CoreAPIClient }

func (panicChannelsAPI) GetActiveChannels() ([]user.User, error) { panic("boom") }

func TestFetchActiveChannelsRecoversPanicAndNilClient(t *testing.T) {
	if _, err := fetchActiveChannels(panicChannelsAPI{}); err == nil {
		t.Fatal("a panicking client must surface as an error")
	}
	if _, err := fetchActiveChannels(nil); err == nil {
		t.Fatal("a nil client must surface as an error")
	}
}

func TestRetryChannelsInBackgroundStopsOnCancel(t *testing.T) {
	fastChannelRetry(t, 1)
	core := newFakeCoreAPI(t, 1_000, loaderUsers)
	b := &Bot{coreAPI: core.client}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		b.retryChannelsInBackground(ctx)
		close(done)
	}()
	waitFor(t, "a few background attempts", func() bool { return core.channelCalls.Load() >= 3 })
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("background loader did not stop on cancel")
	}
}

// Recovery merges into whatever is tracked by then: a channel already added by
// !joinme (or with a !joinme in flight) is not tracked twice, offline channels
// are tracked but not joined, and nothing is said in chat.
func TestRetryChannelsInBackgroundJoinsWithoutChat(t *testing.T) {
	fastChannelRetry(t, 1)
	users := []user.User{
		testUser("101", "livechannel", true),
		testUser("102", "offlinechannel", false),
		testUser("103", "joinmechannel", true),
		testUser("104", "inflightchannel", true),
	}
	core := newFakeCoreAPI(t, 3, users)
	irc := newFakeIRC(t)

	client := twitch.NewClient("peanutbudderbot", "oauth:test")
	client.IrcAddress = irc.ln.Addr().String()
	client.TLS = false
	go func() { _ = client.Connect() }()
	t.Cleanup(func() { _ = client.Disconnect() })
	// Joins issued before or after the 001 welcome both reach the server
	// (go-twitch-irc replays pending joins on connect), so no wait is needed.

	b := &Bot{
		client:         client,
		coreAPI:        core.client,
		config:         Config{BotUsername: "peanutbudderbot", ChannelsPending: true},
		joinInFlight:   map[string]bool{"104": true},
		autoShout:      map[string][]string{},
		live:           map[string]bool{},
		dadUsage:       map[string]*dadStreamUsage{},
		customCommands: map[string]map[string]commands.Commands{},
		stalk:          map[string]*stalkWatch{},
		sayQueue:       make(chan sayRequest, 8),
		ircSayQueue:    make(chan ircSayRequest, 8),
	}
	b.config.Channels = []BotUser{{User: testUser("103", "joinmechannel", true)}}

	b.retryChannelsInBackground(context.Background())

	if got := core.channelCalls.Load(); got != 4 {
		t.Fatalf("core-api calls = %d, want 4 (3 failures then success)", got)
	}
	tracked := trackedLogins(b)
	want := map[string]int{"livechannel": 1, "offlinechannel": 1, "joinmechannel": 1}
	if len(tracked) != len(want) {
		t.Fatalf("tracked = %v, want %v", tracked, want)
	}
	for k, v := range want {
		if tracked[k] != v {
			t.Fatalf("tracked = %v, want %v", tracked, want)
		}
	}
	waitFor(t, "JOIN #livechannel", func() bool { return irc.joined()["livechannel"] })
	time.Sleep(100 * time.Millisecond)
	j := irc.joined()
	if j["offlinechannel"] || j["inflightchannel"] || j["joinmechannel"] {
		t.Fatalf("unexpected joins: %v", j)
	}
	b.liveMu.Lock()
	live, off := b.live["livechannel"], b.live["offlinechannel"]
	b.liveMu.Unlock()
	if !live || off {
		t.Fatalf("live state not seeded: livechannel=%v offlinechannel=%v", live, off)
	}
	if p := irc.privmsgs(); len(p) != 0 {
		t.Fatalf("no chat may be sent during recovery, got %v", p)
	}
	if len(b.sayQueue) != 0 || len(b.ircSayQueue) != 0 {
		t.Fatalf("no chat may be queued during recovery (say=%d irc=%d)", len(b.sayQueue), len(b.ircSayQueue))
	}
}

// bootBot runs the real boot sequence (LoadChannelsAtBoot -> NewBot -> Start)
// against the fake core-api and a fake IRC server, the same way
// cmd/twitch-bot/main.go does.
func bootBot(t *testing.T, core *fakeCoreAPI, irc *fakeIRC, grantedID string) (*Bot, *countingTransport) {
	t.Helper()
	channels, ok := LoadChannelsAtBoot(core.client)
	b, err := NewBot(core.client, &fakeEventSub{grantedID: grantedID}, Config{
		BotUsername:     "peanutbudderbot",
		BotOAuth:        "oauth:test",
		Channels:        channels,
		ChannelsPending: !ok,
		StoragePath:     filepath.Join(t.TempDir(), "counters.json"),
		Timezone:        "UTC",

		WebhookListenAddress: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewBot: %s", err)
	}
	b.client.IrcAddress = irc.ln.Addr().String()
	b.client.TLS = false
	helix := &countingTransport{}
	b.httpClient = &http.Client{Transport: helix}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = b.Start(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		b.Stop()
		<-done
	})
	return b, helix
}

// Local simulation of the incident: core-api is down for the whole boot
// window. The bot must still start, join only its home channels, then join the
// live channel once core-api comes back, without sending any chat.
func TestBootSimulationCoreAPIDownThenUp(t *testing.T) {
	fastChannelRetry(t, 3)
	core := newFakeCoreAPI(t, 0, []user.User{
		testUser("101", "livechannel", true),
		testUser("102", "offlinechannel", false),
	})
	core.down.Store(true)
	irc := newFakeIRC(t)

	b, helix := bootBot(t, core, irc, "101")

	waitFor(t, "home channel JOINs", func() bool {
		j := irc.joined()
		return j[botCreatorChannel] && j["peanutbudderbot"]
	})
	if j := irc.joined(); j["livechannel"] {
		t.Fatal("must not join tracked channels before core-api recovers")
	}
	if n := len(trackedLogins(b)); n != 0 {
		t.Fatalf("no channels should be tracked while core-api is down, got %d", n)
	}
	waitFor(t, "background retries", func() bool { return core.channelCalls.Load() >= 8 })

	core.down.Store(false)

	waitFor(t, "JOIN #livechannel after recovery", func() bool { return irc.joined()["livechannel"] })
	waitFor(t, "commands loaded", b.commandsLoaded)
	waitFor(t, "chat-read privs re-hydrated", func() bool { return b.channelHasPrivs("livechannel") })
	tracked := trackedLogins(b)
	if tracked["livechannel"] != 1 || tracked["offlinechannel"] != 1 || len(tracked) != 2 {
		t.Fatalf("tracked = %v", tracked)
	}
	calls := core.channelCalls.Load()
	time.Sleep(100 * time.Millisecond)
	if got := core.channelCalls.Load(); got != calls {
		t.Fatalf("background loader kept polling after success (%d -> %d)", calls, got)
	}
	if irc.joined()["offlinechannel"] {
		t.Fatal("offline channel must not be joined")
	}
	if p := irc.privmsgs(); len(p) != 0 {
		t.Fatalf("no chat may be sent during recovery, got %v", p)
	}
	if n := helix.n.Load(); n != 0 {
		t.Fatalf("no Helix chat send may happen during recovery, got %d requests", n)
	}
}

// Healthy core-api: one channel fetch, no background loader, same joins as
// before this change.
func TestBootSimulationHealthyCoreAPIUnchanged(t *testing.T) {
	fastChannelRetry(t, 3)
	core := newFakeCoreAPI(t, 0, []user.User{
		testUser("101", "livechannel", true),
		testUser("102", "offlinechannel", false),
	})
	irc := newFakeIRC(t)

	b, helix := bootBot(t, core, irc, "")
	if b.config.ChannelsPending {
		t.Fatal("ChannelsPending must be false when core-api is healthy")
	}

	waitFor(t, "boot JOINs", func() bool {
		j := irc.joined()
		return j[botCreatorChannel] && j["peanutbudderbot"] && j["livechannel"]
	})
	time.Sleep(100 * time.Millisecond)
	if got := core.channelCalls.Load(); got != 1 {
		t.Fatalf("core-api channel calls = %d, want exactly 1", got)
	}
	if irc.joined()["offlinechannel"] {
		t.Fatal("offline channel must not be joined")
	}
	if p := irc.privmsgs(); len(p) != 0 {
		t.Fatalf("no chat may be sent at boot, got %v", p)
	}
	if n := helix.n.Load(); n != 0 {
		t.Fatalf("no Helix chat send may happen at boot, got %d requests", n)
	}
}
