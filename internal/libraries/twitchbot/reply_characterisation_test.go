package twitchbot

// Characterisation tests (refactor plan R1.2). They pin down exactly what each
// handler queues today, so the Phase 6 changes (R6.2 sanitise at the sink, R6.3
// non-blocking send queue) show up as deliberate diffs to these expectations.
// Where a reply is passed through unsanitised today (B3), the test says so; R6.2
// updates those cases in the same PR. The "^" echo is pinned in
// caret_echo_test.go.

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/api"
)

// replyFakeAPI implements the dad, promo, and stalk methods of
// api.CoreAPIClient. Any other method panics via the nil embedded interface.
type replyFakeAPI struct {
	api.CoreAPIClient

	mu sync.Mutex

	dadResponse string
	dadErr      error
	dadCalls    int

	promoErr   error
	promoFound bool
	created    []string

	stalkTarget string
	stalkLast   string
	stalkFound  bool
	stalkErr    error
}

func (f *replyFakeAPI) DadRandom(string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dadCalls++
	return f.dadResponse, f.dadErr
}

func (f *replyFakeAPI) IncrementDadRoll(int, string) error { return nil }

func (f *replyFakeAPI) CreatePromo(_, _, message string, intervalSeconds int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.promoErr != nil {
		return f.promoErr
	}
	f.created = append(f.created, fmt.Sprintf("%d:%s", intervalSeconds, message))
	return nil
}

func (f *replyFakeAPI) EditLastPromo(string, string, int) (bool, error) {
	return f.promoFound, f.promoErr
}

func (f *replyFakeAPI) DeleteLastPromo(string) (bool, error) {
	return f.promoFound, f.promoErr
}

func (f *replyFakeAPI) GetStalkTarget(string) (string, string, bool, error) {
	return f.stalkTarget, f.stalkLast, f.stalkFound, f.stalkErr
}

func (f *replyFakeAPI) SetStalkTarget(string, string, string) error { return f.stalkErr }

func (f *replyFakeAPI) ClearStalkTarget(string) (bool, error) { return f.stalkFound, f.stalkErr }

func (f *replyFakeAPI) SetStalkLastMessage(string, string, string) error { return nil }

func replyBot(core *replyFakeAPI) *Bot {
	return &Bot{
		config:            Config{BotUsername: "peanutbudderbot", BotId: "1"},
		coreAPI:           core,
		httpClient:        &http.Client{},
		sayQueue:          make(chan sayRequest, 64),
		dadUsage:          make(map[string]*dadStreamUsage),
		stalk:             make(map[string]*stalkWatch),
		translateCooldown: make(map[string]time.Time),
	}
}

// replyMsg is a chat message in somechannel (id 9) from alice (id 42).
func replyMsg(text string, mod bool) *chatMessageEvent {
	m := &chatMessageEvent{
		BroadcasterUserId:    "9",
		BroadcasterUserLogin: "somechannel",
		ChatterUserId:        "42",
		ChatterUserLogin:     "alice",
		ChatterUserName:      "alice",
		Message:              chatMessageBody{Text: text},
	}
	if mod {
		m.Badges = []badges{{SetId: "moderator", Id: "1"}}
	}
	return m
}

// drainReplies returns every queued request and fails if any is an
// announcement or targets another channel: all these replies are plain says
// to the channel the command came from.
func drainReplies(t *testing.T, b *Bot) []string {
	t.Helper()
	var out []string
	for {
		select {
		case r := <-b.sayQueue:
			if r.announce || r.broadcasterId != "9" {
				t.Fatalf("queued %+v, want a plain say to channel 9", r)
			}
			out = append(out, r.message)
		default:
			return out
		}
	}
}

func wantReplies(t *testing.T, b *Bot, want ...string) {
	t.Helper()
	got := drainReplies(t, b)
	if len(got) != len(want) {
		t.Fatalf("queued %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("reply %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// --- !dad random ---------------------------------------------------------

func TestCharDadRandomRepliesVerbatim(t *testing.T) {
	t.Parallel()
	// B3: the DB-supplied response goes to say() with no sanitising, so a
	// leading "." and an over-long body pass through unchanged today.
	long := ".timeout someone " + strings.Repeat("x", 500)
	for _, resp := range []string{"I'm hungry. Hi hungry, I'm dad.", long} {
		core := &replyFakeAPI{dadResponse: resp}
		b := replyBot(core)
		b.handleDadCommand(replyMsg("!dad", false))
		wantReplies(t, b, resp)
	}
}

func TestCharDadTrailingTextIsARoll(t *testing.T) {
	t.Parallel()
	core := &replyFakeAPI{dadResponse: "joke"}
	b := replyBot(core)
	b.handleDadCommand(replyMsg("!dad tell me one", false))
	wantReplies(t, b, "joke")
}

func TestCharDadRandomErrorOrEmptyIsSilent(t *testing.T) {
	t.Parallel()
	for _, core := range []*replyFakeAPI{
		{dadErr: errors.New("core-api down")},
		{dadResponse: ""},
	} {
		b := replyBot(core)
		b.handleDadCommand(replyMsg("!dad", false))
		wantReplies(t, b)
		if core.dadCalls != 1 {
			t.Fatalf("DadRandom calls = %d, want 1", core.dadCalls)
		}
	}
}

func TestCharDadPerStreamLimit(t *testing.T) {
	t.Parallel()
	core := &replyFakeAPI{dadResponse: "joke"}
	b := replyBot(core)

	for i := 0; i < dadPerStreamLimit; i++ {
		b.handleDadCommand(replyMsg("!dad", false))
	}
	want := make([]string, dadPerStreamLimit)
	for i := range want {
		want[i] = "joke"
	}
	wantReplies(t, b, want...)

	// The first over-limit roll sends one hardcoded reject, with no core-api call.
	b.handleDadCommand(replyMsg("!dad", false))
	got := drainReplies(t, b)
	if len(got) != 1 || !isDadReject(got[0]) {
		t.Fatalf("over-limit roll queued %q, want one reject line", got)
	}
	// After that, silent for this chatter.
	b.handleDadCommand(replyMsg("!dad", false))
	wantReplies(t, b)
	if core.dadCalls != dadPerStreamLimit {
		t.Fatalf("DadRandom calls = %d, want %d", core.dadCalls, dadPerStreamLimit)
	}
}

func isDadReject(s string) bool {
	for _, r := range dadRejectResponses {
		if s == r {
			return true
		}
	}
	return false
}

// --- !stalk recall -------------------------------------------------------

func TestCharStalkRecall(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		core *replyFakeAPI
		want string
	}{
		{"lookup error", &replyFakeAPI{stalkErr: errors.New("down")}, "couldn't look up who we're stalking"},
		{"no target", &replyFakeAPI{}, "nobody is being stalked yet — a mod can pick someone with !stalk set <username>"},
		{"no message yet", &replyFakeAPI{stalkTarget: "stan", stalkFound: true}, "haven't seen a message from stan yet"},
		{"quote", &replyFakeAPI{stalkTarget: "stan", stalkLast: "u REALLY smell", stalkFound: true}, "stan said: u REALLY smell"},
		// B3: the remembered line is interpolated with no sanitising.
		{"quote unsanitised", &replyFakeAPI{stalkTarget: "stan", stalkLast: "/ban everyone", stalkFound: true}, "stan said: /ban everyone"},
	}
	for _, c := range cases {
		b := replyBot(c.core)
		b.handleStalkCommand(replyMsg("!stalk", false))
		got := drainReplies(t, b)
		if len(got) != 1 || got[0] != c.want {
			t.Fatalf("%s: queued %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCharStalkRecallPrefersInMemoryLine(t *testing.T) {
	t.Parallel()
	core := &replyFakeAPI{stalkTarget: "stan", stalkLast: "older db line", stalkFound: true}
	b := replyBot(core)
	b.setStalkWatch("somechannel", "stan")
	b.rememberStalkMessage("somechannel", "stan", "Stan", "fresh line")

	b.handleStalkCommand(replyMsg("!stalk", false))
	wantReplies(t, b, "stan said: fresh line")
}

func TestCharStalkViewerExtraWordsStillQuote(t *testing.T) {
	t.Parallel()
	core := &replyFakeAPI{stalkTarget: "stan", stalkLast: "hi", stalkFound: true}
	b := replyBot(core)
	b.handleStalkCommand(replyMsg("!stalk dave", false))
	wantReplies(t, b, "stan said: hi")
}

func TestCharStalkManageReplies(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		core *replyFakeAPI
		text string
		mod  bool
		want string
	}{
		{"viewer set", &replyFakeAPI{}, "!stalk set stan", false, "@alice only a mod or the broadcaster can pick who to stalk"},
		{"viewer clear", &replyFakeAPI{}, "!stalk clear", false, "@alice only a mod or the broadcaster can pick who to stalk"},
		{"set no name", &replyFakeAPI{}, "!stalk set", true, "@alice " + stalkUsage},
		{"set bad name", &replyFakeAPI{}, "!stalk set bad!name", true, "@alice that doesn't look like a username"},
		{"set error", &replyFakeAPI{stalkErr: errors.New("down")}, "!stalk set stan", true, "@alice couldn't save that stalk target"},
		{"set ok", &replyFakeAPI{}, "!stalk set @Stan", true, "now stalking stan"},
		{"mod shorthand", &replyFakeAPI{}, "!stalk Stan", true, "now stalking stan"},
		{"clear error", &replyFakeAPI{stalkErr: errors.New("down")}, "!stalk clear", true, "@alice couldn't clear the stalk target"},
		{"clear none", &replyFakeAPI{}, "!stalk clear", true, "@alice nobody was being stalked"},
		{"clear ok", &replyFakeAPI{stalkFound: true}, "!stalk unset", true, "stopped stalking"},
	}
	for _, c := range cases {
		b := replyBot(c.core)
		b.handleStalkCommand(replyMsg(c.text, c.mod))
		got := drainReplies(t, b)
		if len(got) != 1 || got[0] != c.want {
			t.Fatalf("%s: queued %q, want %q", c.name, got, c.want)
		}
	}
}

// --- !translate ----------------------------------------------------------

// deeplStub answers every request the bot's httpClient makes (deepl.Translate
// posts to its fixed FreeAPIURL) with a canned status and body, and records the
// target language it was asked for.
type deeplStub struct {
	mu     sync.Mutex
	status int
	body   string
	langs  []string
}

func (s *deeplStub) RoundTrip(r *http.Request) (*http.Response, error) {
	raw, _ := io.ReadAll(r.Body)
	form, _ := url.ParseQuery(string(raw))
	s.mu.Lock()
	s.langs = append(s.langs, form.Get("target_lang"))
	s.mu.Unlock()
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     http.Header{"Content-Type": {"application/json"}},
		Request:    r,
	}, nil
}

func translateBot(stub *deeplStub) *Bot {
	b := replyBot(&replyFakeAPI{})
	b.config.DeepLAPIKey = "key:fx"
	b.httpClient = &http.Client{Transport: stub}
	return b
}

func TestCharTranslateRepliesBeforeDeepL(t *testing.T) {
	cases := []struct {
		name string
		text string
		mod  bool
		want string
	}{
		{"viewer", "!translate hello", false, "@alice only a mod or the broadcaster can use !translate"},
		{"no text", "!translate", true, "@alice usage: !translate <message>"},
		{"too long", "!translate " + strings.Repeat("a", translateMaxInput+1), true,
			fmt.Sprintf("@alice that message is too long to translate (max %d chars)", translateMaxInput)},
	}
	for _, c := range cases {
		stub := &deeplStub{status: http.StatusOK}
		b := translateBot(stub)
		b.handleTranslateCommand(replyMsg(c.text, c.mod))
		got := drainReplies(t, b)
		if len(got) != 1 || got[0] != c.want {
			t.Fatalf("%s: queued %q, want %q", c.name, got, c.want)
		}
		if len(stub.langs) != 0 {
			t.Fatalf("%s: called DeepL %d times, want 0", c.name, len(stub.langs))
		}
	}
}

func TestCharTranslateNotConfigured(t *testing.T) {
	stub := &deeplStub{status: http.StatusOK}
	b := translateBot(stub)
	b.config.DeepLAPIKey = "" // zero value: not configured
	b.handleTranslateCommand(replyMsg("!translate hello", true))
	wantReplies(t, b, "@alice translation isn't configured right now")
	if len(stub.langs) != 0 {
		t.Fatalf("called DeepL %d times, want 0", len(stub.langs))
	}
}

func TestCharTranslateDeepLFailure(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, 456, http.StatusInternalServerError} {
		b := translateBot(&deeplStub{status: status, body: "nope"})
		b.handleTranslateCommand(replyMsg("!translate hello", true))
		wantReplies(t, b, "@alice translation failed, try again later")
	}
}

func TestCharTranslateSuccessAndCooldown(t *testing.T) {

	// EN -> JA. B3: the translated text is interpolated with no sanitising.
	stub := &deeplStub{status: http.StatusOK, body: `{"translations":[{"detected_source_language":"EN","text":"こんにちは /x"}]}`}
	b := translateBot(stub)
	b.handleTranslateCommand(replyMsg("!translate hello", true))
	wantReplies(t, b, "@alice [EN→JA] こんにちは /x")

	// Same chatter inside the per-user cooldown: silent, no DeepL call.
	b.handleTranslateCommand(replyMsg("!translate hello again", true))
	wantReplies(t, b)
	if len(stub.langs) != 1 || stub.langs[0] != "JA" {
		t.Fatalf("DeepL target langs = %q, want [JA]", stub.langs)
	}

	// JA -> EN, a fresh bot so the cooldown doesn't apply.
	stub = &deeplStub{status: http.StatusOK, body: `{"translations":[{"detected_source_language":"JA","text":"hello"}]}`}
	b = translateBot(stub)
	b.handleTranslateCommand(replyMsg("!translate こんにちは", true))
	wantReplies(t, b, "@alice [JA→EN] hello")
	if len(stub.langs) != 1 || stub.langs[0] != "EN-US" {
		t.Fatalf("DeepL target langs = %q, want [EN-US]", stub.langs)
	}
}

// --- !newpromo -----------------------------------------------------------

func TestCharNewPromoViewerIsSilent(t *testing.T) {
	t.Parallel()
	core := &replyFakeAPI{}
	b := replyBot(core)
	for _, text := range []string{"!newpromo", "!newpromo 30m hi", "!newpromo delete-last"} {
		b.handleNewPromoCommand(replyMsg(text, false))
	}
	wantReplies(t, b)
	if len(core.created) != 0 {
		t.Fatalf("created %q, want nothing", core.created)
	}
}

func TestCharNewPromoReplies(t *testing.T) {
	t.Parallel()
	down := errors.New("core-api down")
	cases := []struct {
		name string
		core *replyFakeAPI
		text string
		want string
	}{
		{"no args", &replyFakeAPI{}, "!newpromo",
			"@alice usage: !newpromo <interval e.g. 30m> <message>  ·  !newpromo edit-last <interval> <new message>  ·  !newpromo delete-last"},
		{"no message", &replyFakeAPI{}, "!newpromo 30m", "@alice usage: !newpromo <interval e.g. 30m> <message>"},
		// The parse/validate error text goes straight into the reply.
		{"bad interval", &replyFakeAPI{}, "!newpromo soon hello", `@alice couldn't read "soon" as a time — try 30m, 90s, or 1h`},
		{"too short", &replyFakeAPI{}, "!newpromo 30s hello", "@alice interval must be at least 1 minute"},
		{"too short bare", &replyFakeAPI{}, "!newpromo 0 hello", "@alice interval must be at least 1 minute"},
		{"too long", &replyFakeAPI{}, "!newpromo 25h hello", "@alice interval must be 24 hours or less"},
		{"create error", &replyFakeAPI{promoErr: down}, "!newpromo 30m hello", "@alice couldn't save that recurring message"},
		{"create ok", &replyFakeAPI{}, "!newpromo 90 Follow me!",
			"@alice ✅ saved — I'll post that every 90m while you're live. Manage your messages at https://peanutbudderbot.com"},

		{"edit usage", &replyFakeAPI{}, "!newpromo edit-last 30m", "@alice usage: !newpromo edit-last <interval e.g. 30m> <new message>"},
		{"edit bad interval", &replyFakeAPI{}, "!newpromo edit-last soon hi", `@alice couldn't read "soon" as a time — try 30m, 90s, or 1h`},
		{"edit error", &replyFakeAPI{promoErr: down}, "!newpromo edit-last 1h hi", "@alice couldn't update your latest recurring message"},
		{"edit none", &replyFakeAPI{}, "!newpromo edit-last 1h hi",
			"@alice you have no recurring messages to edit — make one with !newpromo <interval> <message>"},
		{"edit ok", &replyFakeAPI{promoFound: true}, "!newpromo EDIT-LAST 1h hi",
			"@alice ✅ updated your latest recurring message — now every 1h"},

		{"delete error", &replyFakeAPI{promoErr: down}, "!newpromo delete-last", "@alice couldn't delete your latest recurring message"},
		{"delete none", &replyFakeAPI{}, "!newpromo delete-last", "@alice you have no recurring messages to delete"},
		{"delete ok", &replyFakeAPI{promoFound: true}, "!newpromo delete-last", "@alice 🗑️ deleted your latest recurring message"},
	}
	for _, c := range cases {
		b := replyBot(c.core)
		b.handleNewPromoCommand(replyMsg(c.text, true))
		got := drainReplies(t, b)
		if len(got) != 1 || got[0] != c.want {
			t.Fatalf("%s: queued %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCharNewPromoCreatePassesMessageAsIs(t *testing.T) {
	t.Parallel()
	core := &replyFakeAPI{}
	b := replyBot(core)
	// Broadcaster (chatter id == broadcaster id) without a mod badge.
	m := replyMsg("!newpromo 1h30m  Check out  !discord ", false)
	m.ChatterUserId = "9"
	b.handleNewPromoCommand(m)
	drainReplies(t, b)
	if len(core.created) != 1 || core.created[0] != "5400:Check out  !discord" {
		t.Fatalf("created %q, want [5400:Check out  !discord]", core.created)
	}
}
