package twitchbot

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// R5.1: the four optional env vars now arrive through Config. These tests pin
// that the zero value keeps each feature off (or on its default) exactly as an
// unset env var did, and that a non-empty value turns it on.

func TestConfigZeroDeepLKeyDisablesTranslate(t *testing.T) {
	stub := &deeplStub{status: http.StatusOK}
	b := replyBot(&replyFakeAPI{}) // zero DeepLAPIKey
	b.httpClient = &http.Client{Transport: stub}
	b.handleTranslateCommand(replyMsg("!translate hello", true))
	wantReplies(t, b, "@alice translation isn't configured right now")
	if len(stub.langs) != 0 {
		t.Fatalf("called DeepL %d times, want 0", len(stub.langs))
	}
}

func TestConfigDeepLKeyEnablesTranslate(t *testing.T) {
	stub := &deeplStub{status: http.StatusOK, body: `{"translations":[{"detected_source_language":"EN","text":"こんにちは"}]}`}
	b := replyBot(&replyFakeAPI{})
	b.config.DeepLAPIKey = "key:fx"
	b.httpClient = &http.Client{Transport: stub}
	b.handleTranslateCommand(replyMsg("!translate hello", true))
	wantReplies(t, b, "@alice [EN→JA] こんにちは")
	if len(stub.langs) != 1 {
		t.Fatalf("called DeepL %d times, want 1", len(stub.langs))
	}
}

// discordRecorder counts go-live POSTs and keeps the last body.
type discordRecorder struct {
	mu    sync.Mutex
	posts int
	body  string
}

func newDiscordServer(t *testing.T) (*httptest.Server, *discordRecorder) {
	t.Helper()
	rec := &discordRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec.mu.Lock()
		rec.posts++
		rec.body = string(raw)
		rec.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, rec
}

func TestConfigZeroDiscordWebhookDisablesGoLive(t *testing.T) {
	_, rec := newDiscordServer(t)
	var cfg Config
	notifyDiscordGoLive(cfg.DiscordWebhookURL, "Alice", "alice")
	if rec.posts != 0 {
		t.Fatalf("zero Config posted %d go-live messages, want 0", rec.posts)
	}
}

func TestConfigDiscordWebhookEnablesGoLive(t *testing.T) {
	srv, rec := newDiscordServer(t)
	cfg := Config{DiscordWebhookURL: srv.URL}
	notifyDiscordGoLive(cfg.DiscordWebhookURL, "Alice", "alice")
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if rec.posts != 1 {
		t.Fatalf("posted %d go-live messages, want 1", rec.posts)
	}
	if !strings.Contains(rec.body, "https://twitch.tv/alice") {
		t.Fatalf("go-live body %q missing stream URL", rec.body)
	}
}

func TestConfigZeroEventSubSecretRejectsEveryDelivery(t *testing.T) {
	h := newTwitchEventSubEndpoint(&Bot{}) // zero EventSubSecret
	rec := serveEventSub(t, h, signedEventSubRequest(MESSAGE_TYPE_NOTIFICATION, "zero-1", time.Now(), unknownNotification))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
}

func TestConfigEventSubSecretAcceptsSignedDelivery(t *testing.T) {
	h := newTwitchEventSubEndpoint(&Bot{config: Config{EventSubSecret: testEventSubSecret}})
	rec := serveEventSub(t, h, signedEventSubRequest(MESSAGE_TYPE_NOTIFICATION, "set-1", time.Now(), unknownNotification))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204", rec.Code)
	}
}

func TestConfigWebhookListenAddress(t *testing.T) {
	addr, explicit := webhookListenAddress(Config{})
	if addr != defaultWebhookListenAddress || explicit {
		t.Fatalf("zero Config: got (%q, %v), want (%q, false)", addr, explicit, defaultWebhookListenAddress)
	}
	addr, explicit = webhookListenAddress(Config{WebhookListenAddress: "127.0.0.1:9999"})
	if addr != "127.0.0.1:9999" || !explicit {
		t.Fatalf("set: got (%q, %v), want (127.0.0.1:9999, true)", addr, explicit)
	}
}

// The journal lines NewWebhookListener prints must stay byte-identical so a
// before/after boot can be compared.
func TestConfigWebhookListenerStartupLogLines(t *testing.T) {
	const warn = "warning: TWITCH_EVENTSUB_SECRET is not set; eventsub signature verification will reject every request"
	for _, tc := range []struct {
		name     string
		secret   string
		wantWarn bool
	}{
		{"zero secret", "", true},
		{"secret set", testEventSubSecret, false},
	} {
		prev := log.Writer()
		var buf bytes.Buffer
		log.SetOutput(&buf)
		NewWebhookListener(&Bot{config: Config{WebhookListenAddress: "127.0.0.1:0", EventSubSecret: tc.secret}})
		log.SetOutput(prev)
		out := buf.String()
		if got := strings.Contains(out, warn); got != tc.wantWarn {
			t.Fatalf("%s: warning present = %v, want %v; log:\n%s", tc.name, got, tc.wantWarn, out)
		}
		if !strings.Contains(out, "eventsub webhook listener started on 127.0.0.1:") {
			t.Fatalf("%s: missing listener started line; log:\n%s", tc.name, out)
		}
	}
}
