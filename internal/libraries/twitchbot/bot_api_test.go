package twitchbot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/twitchsig"
)

const testEventSubSecret = "test-eventsub-secret"

// eventSubTestBot is a zero Bot except for the EventSub secret.
func eventSubTestBot() *Bot {
	return &Bot{config: Config{EventSubSecret: testEventSubSecret}}
}

func signedEventSubRequest(msgType, id string, ts time.Time, body string) *http.Request {
	tsStr := ts.UTC().Format(time.RFC3339Nano)
	mac := hmac.New(sha256.New, []byte(testEventSubSecret))
	mac.Write([]byte(id + tsStr + body))
	req := httptest.NewRequest(http.MethodPost, "/eventsub", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(MESSAGE_TYPE, msgType)
	req.Header.Set(twitchsig.HeaderMessageID, id)
	req.Header.Set(twitchsig.HeaderMessageTimestamp, tsStr)
	req.Header.Set(twitchsig.HeaderMessageSignature, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return req
}

func serveEventSub(t *testing.T, h echo.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := h(echo.New().NewContext(req, rec)); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return rec
}

// An unrecognized subscription type only logs, so a zero Bot is safe here.
const unknownNotification = `{"subscription":{"id":"s1","type":"test.unknown"},"event":{}}`

func TestEventSubVerificationChallengeAccepted(t *testing.T) {
	h := newTwitchEventSubEndpoint(eventSubTestBot())
	body := `{"challenge":"pogchamp-kappa-360","subscription":{"id":"s1","type":"stream.online"}}`

	for i := 0; i < 2; i++ { // a retried handshake must still be answered
		rec := serveEventSub(t, h, signedEventSubRequest(MESSAGE_TYPE_VERIFICATION, "v-1", time.Now(), body))
		if rec.Code != http.StatusOK || rec.Body.String() != "pogchamp-kappa-360" {
			t.Fatalf("attempt %d: got %d %q, want 200 challenge", i, rec.Code, rec.Body.String())
		}
	}
}

func TestEventSubFreshNotificationAccepted(t *testing.T) {
	h := newTwitchEventSubEndpoint(eventSubTestBot())
	rec := serveEventSub(t, h, signedEventSubRequest(MESSAGE_TYPE_NOTIFICATION, "n-1", time.Now(), unknownNotification))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204", rec.Code)
	}
}

func TestEventSubStaleTimestampRejected(t *testing.T) {
	h := newTwitchEventSubEndpoint(eventSubTestBot())
	for name, ts := range map[string]time.Time{
		"old":    time.Now().Add(-11 * time.Minute),
		"future": time.Now().Add(11 * time.Minute),
	} {
		rec := serveEventSub(t, h, signedEventSubRequest(MESSAGE_TYPE_NOTIFICATION, "stale-"+name, ts, unknownNotification))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d, want 403", name, rec.Code)
		}
	}
}

func TestEventSubDuplicateMessageIDAcknowledged(t *testing.T) {
	h := newTwitchEventSubEndpoint(eventSubTestBot())

	// Revocation of a non-chat-read type touches nothing on the bot.
	revocation := `{"subscription":{"id":"s1","type":"stream.online","status":"authorization_revoked"}}`
	for _, tc := range []struct{ msgType, id, body string }{
		{MESSAGE_TYPE_NOTIFICATION, "dup-n", unknownNotification},
		{MESSAGE_TYPE_REVOCATION, "dup-r", revocation},
	} {
		for i := 0; i < 2; i++ { // Twitch retry reuses the message id
			rec := serveEventSub(t, h, signedEventSubRequest(tc.msgType, tc.id, time.Now(), tc.body))
			if rec.Code != http.StatusNoContent {
				t.Fatalf("%s attempt %d: got %d, want 204", tc.msgType, i, rec.Code)
			}
		}
		if !handlerSeen(t, h, tc.id) {
			t.Fatalf("%s: id %s should be remembered as a duplicate", tc.msgType, tc.id)
		}
	}
}

// handlerSeen probes the handler's dedupe by replaying id with a body that would
// otherwise be rejected (unknown message type -> 400). A 204 means the id was
// already claimed.
func handlerSeen(t *testing.T, h echo.HandlerFunc, id string) bool {
	t.Helper()
	rec := serveEventSub(t, h, signedEventSubRequest("bogus", id, time.Now(), `{}`))
	return rec.Code == http.StatusNoContent
}

func TestEventSubUnknownTypeStillRejected(t *testing.T) {
	h := newTwitchEventSubEndpoint(eventSubTestBot())
	if handlerSeen(t, h, "fresh-id") {
		t.Fatal("fresh id with unknown message type should be 400, not deduped")
	}
}

func TestListenWebhookFallsBackForDefault(t *testing.T) {
	// 192.0.2.1 (TEST-NET-1) is never assigned locally, so binding fails.
	ln, err := listenWebhook("192.0.2.1:0", "127.0.0.1:0", true)
	if err != nil {
		t.Fatalf("expected fallback bind, got %v", err)
	}
	defer ln.Close()
	if host, _, _ := net.SplitHostPort(ln.Addr().String()); host != "127.0.0.1" {
		t.Fatalf("bound %s, want fallback 127.0.0.1", ln.Addr())
	}
}

func TestListenWebhookExplicitDoesNotFallBack(t *testing.T) {
	ln, err := listenWebhook("192.0.2.1:0", "127.0.0.1:0", false)
	if err == nil {
		ln.Close()
		t.Fatal("explicit address must not fall back")
	}
}

func TestListenWebhookBindsPrimary(t *testing.T) {
	ln, err := listenWebhook("127.0.0.1:0", "192.0.2.1:0", true)
	if err != nil {
		t.Fatalf("primary bind failed: %v", err)
	}
	ln.Close()
}
