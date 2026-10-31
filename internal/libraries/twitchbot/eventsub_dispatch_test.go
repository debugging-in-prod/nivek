package twitchbot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

func chatNotification(broadcasterID, text string) *EventSubSubscriptionResponse {
	ev, _ := json.Marshal(map[string]string{"broadcaster_user_id": broadcasterID, "text": text})
	return &EventSubSubscriptionResponse{
		Subscription: SubscriptionResponse{
			Type:      "channel.chat.message",
			Condition: map[string]string{"broadcaster_user_id": broadcasterID},
		},
		Event: ev,
	}
}

func eventText(t *testing.T, n *EventSubSubscriptionResponse) string {
	t.Helper()
	var ev struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(n.Event, &ev); err != nil {
		t.Errorf("unmarshal event: %v", err)
	}
	return ev.Text
}

func waitOrFail(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// Messages for the same broadcaster are handled in arrival order.
func TestChatDispatcherPreservesPerChannelOrder(t *testing.T) {
	const total = 200
	var mu sync.Mutex
	got := map[string][]string{}
	done := make(chan struct{})
	count := 0

	d := newChatDispatcher(4, total, func(n *EventSubSubscriptionResponse) {
		mu.Lock()
		defer mu.Unlock()
		b := n.Subscription.Condition["broadcaster_user_id"]
		got[b] = append(got[b], eventText(t, n))
		count++
		if count == total {
			close(done)
		}
	})

	want := map[string][]string{}
	for i := 0; i < total; i++ {
		b := []string{"111", "222", "333"}[i%3]
		text := fmt.Sprintf("msg-%d", i)
		want[b] = append(want[b], text)
		if !d.enqueue(chatNotification(b, text)) {
			t.Fatalf("enqueue %d unexpectedly dropped", i)
		}
	}
	waitOrFail(t, done, "all messages")

	mu.Lock()
	defer mu.Unlock()
	for b, w := range want {
		if len(got[b]) != len(w) {
			t.Fatalf("broadcaster %s: got %d messages, want %d", b, len(got[b]), len(w))
		}
		for i := range w {
			if got[b][i] != w[i] {
				t.Fatalf("broadcaster %s: message %d = %q, want %q (out of order)", b, i, got[b][i], w[i])
			}
		}
	}
}

// A full queue drops (non-blocking) instead of spawning unbounded work.
func TestChatDispatcherDropsWhenFull(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var mu sync.Mutex
	handled := 0

	d := newChatDispatcher(1, 2, func(n *EventSubSubscriptionResponse) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		mu.Lock()
		handled++
		mu.Unlock()
	})

	// First message occupies the worker; wait until it's being handled.
	if !d.enqueue(chatNotification("1", "busy")) {
		t.Fatal("first enqueue dropped")
	}
	waitOrFail(t, started, "worker to start")

	// Two more fill the queue; the next must be dropped without blocking.
	for i := 0; i < 2; i++ {
		if !d.enqueue(chatNotification("1", "queued")) {
			t.Fatalf("queued enqueue %d dropped", i)
		}
	}
	if d.enqueue(chatNotification("1", "overflow")) {
		t.Fatal("enqueue on full queue should drop")
	}

	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := handled
		mu.Unlock()
		if n == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("handled %d messages, want 3", n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A panicking handler doesn't kill the worker; later messages still run.
func TestChatDispatcherRecoversFromPanic(t *testing.T) {
	done := make(chan struct{})
	d := newChatDispatcher(1, 4, func(n *EventSubSubscriptionResponse) {
		if eventText(t, n) == "boom" {
			panic("handler exploded")
		}
		close(done)
	})
	d.enqueue(chatNotification("1", "boom"))
	d.enqueue(chatNotification("1", "ok"))
	waitOrFail(t, done, "message after panic")
}

// The endpoint ACKs a chat notification without waiting on the handler.
func TestEventSubChatMessageAckedBeforeHandling(t *testing.T) {
	t.Setenv("TWITCH_EVENTSUB_SECRET", testEventSubSecret)
	release := make(chan struct{})
	handled := make(chan struct{})
	var once sync.Once
	// A blocking stand-in handler proves the 204 comes back while the handler
	// is still running.
	chat := newChatDispatcher(1, 4, func(*EventSubSubscriptionResponse) {
		<-release
		once.Do(func() { close(handled) })
	})
	h := newTwitchEventSubEndpointWithDispatcher(&Bot{}, chat)

	body := `{"subscription":{"id":"s1","type":"channel.chat.message","condition":{"broadcaster_user_id":"42"}},"event":{}}`
	rec := serveEventSub(t, h, signedEventSubRequest(MESSAGE_TYPE_NOTIFICATION, "chat-1", time.Now(), body))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204", rec.Code)
	}
	select {
	case <-handled:
		t.Fatal("handler finished before ACK test released it")
	default:
	}
	close(release)
	waitOrFail(t, handled, "queued chat handler")
}
