package twitchbot

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"testing"
	"time"
)

const queueFullLog = "[SAY] queue full, dropping reply to "

// sendSinks drives each send sink with an identifier the drop log should name.
var sendSinks = []struct {
	name string
	id   string
	send func(b *Bot, id, msg string)
}{
	{"say", "4242", func(b *Bot, id, m string) { b.say(id, m) }},
	{"announce", "4242", func(b *Bot, id, m string) { b.announce(id, m) }},
	{"ircsay", "somechannel", func(b *Bot, id, m string) { b.ircsay(id, m) }},
}

// queueLen reports how many requests the sink under test has queued.
func queueLen(b *Bot, sink string) int {
	if sink == "ircsay" {
		return len(b.ircSayQueue)
	}
	return len(b.sayQueue)
}

// captureLog redirects the standard logger for the duration of the test.
// Tests using it must not call t.Parallel (the logger is process-global).
func captureLog(t *testing.T) *syncBuffer {
	t.Helper()
	buf := &syncBuffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return buf
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// sendWithin runs fn and fails the test if it doesn't return within d.
func sendWithin(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		fn()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("%s blocked for more than %v", what, d)
	}
}

// TestSendSinksDropWhenQueueFull: with the queue full every sink returns
// immediately, queues nothing extra, and logs exactly one drop line per
// dropped reply, naming the target id but never the message text.
func TestSendSinksDropWhenQueueFull(t *testing.T) {
	for _, sink := range sendSinks {
		buf := captureLog(t)
		b := &Bot{
			sayQueue:    make(chan sayRequest, 2),
			ircSayQueue: make(chan ircSayRequest, 2),
		}
		for i := 0; i < 2; i++ {
			sink.send(b, sink.id, "filler")
		}
		if got := queueLen(b, sink.name); got != 2 {
			t.Fatalf("%s: setup queued %d, want 2", sink.name, got)
		}
		if strings.Contains(buf.String(), queueFullLog) {
			t.Fatalf("%s: logged a drop while the queue had space:\n%s", sink.name, buf.String())
		}

		const drops = 3
		sendWithin(t, 500*time.Millisecond, sink.name+" with full queue", func() {
			for i := 0; i < drops; i++ {
				sink.send(b, sink.id, "secret reply text")
			}
		})

		if got := queueLen(b, sink.name); got != 2 {
			t.Fatalf("%s: queue len %d after drops, want 2", sink.name, got)
		}
		out := buf.String()
		if n := strings.Count(out, queueFullLog+sink.id+"\n"); n != drops {
			t.Fatalf("%s: %d drop log lines for %s, want %d; log:\n%s", sink.name, n, sink.id, drops, out)
		}
		if strings.Contains(out, "secret reply text") {
			t.Fatalf("%s: drop log contains the message text:\n%s", sink.name, out)
		}
	}
}

// TestSendSinksEnqueueOneWithSpace: with space available a sink enqueues
// exactly one request and logs no drop.
func TestSendSinksEnqueueOneWithSpace(t *testing.T) {
	for _, sink := range sendSinks {
		buf := captureLog(t)
		b := &Bot{
			sayQueue:    make(chan sayRequest, 4),
			ircSayQueue: make(chan ircSayRequest, 4),
		}
		sendWithin(t, 500*time.Millisecond, sink.name, func() { sink.send(b, sink.id, "hello") })
		if got := queueLen(b, sink.name); got != 1 {
			t.Fatalf("%s: queued %d, want 1", sink.name, got)
		}
		if other := len(b.sayQueue) + len(b.ircSayQueue) - 1; other != 0 {
			t.Fatalf("%s: %d requests leaked into the other queue", sink.name, other)
		}
		if strings.Contains(buf.String(), queueFullLog) {
			t.Fatalf("%s: logged a drop with space available:\n%s", sink.name, buf.String())
		}
	}
}

// TestSendSinksConcurrentNonBlocking: many goroutines hammer every sink on
// small queues with nothing draining. Nobody blocks, the queues never exceed
// their capacity, and every send is either queued or logged as a drop.
func TestSendSinksConcurrentNonBlocking(t *testing.T) {
	buf := captureLog(t)
	const capacity, workers, perWorker = 8, 16, 20
	b := &Bot{
		sayQueue:    make(chan sayRequest, capacity),
		ircSayQueue: make(chan ircSayRequest, capacity),
	}

	sendWithin(t, 2*time.Second, "concurrent senders", func() {
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perWorker; i++ {
					for _, sink := range sendSinks {
						sink.send(b, sink.id, "hi")
					}
				}
			}()
		}
		wg.Wait()
	})

	if got := len(b.sayQueue); got != capacity {
		t.Fatalf("sayQueue len %d, want %d (full)", got, capacity)
	}
	if got := len(b.ircSayQueue); got != capacity {
		t.Fatalf("ircSayQueue len %d, want %d (full)", got, capacity)
	}
	out := buf.String()
	// say+announce share sayQueue: 2 sends per iteration into it.
	if n, want := strings.Count(out, queueFullLog+"4242\n"), 2*workers*perWorker-capacity; n != want {
		t.Fatalf("sayQueue drops logged %d, want %d", n, want)
	}
	if n, want := strings.Count(out, queueFullLog+"somechannel\n"), workers*perWorker-capacity; n != want {
		t.Fatalf("ircSayQueue drops logged %d, want %d", n, want)
	}
}
