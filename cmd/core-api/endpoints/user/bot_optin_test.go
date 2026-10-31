package user

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/twitcheventsub"
)

func sub(typ, broadcaster, callback string) twitcheventsub.EventSubSubscription {
	var s twitcheventsub.EventSubSubscription
	s.Type = typ
	s.Condition.BroadcasterUserID = broadcaster
	s.Transport.Callback = callback
	return s
}

func TestIsBotSubscriptionFor(t *testing.T) {
	const botCB = "https://peanutbudderbot.com/eventsub"
	cases := []struct {
		name string
		s    twitcheventsub.EventSubSubscription
		want bool
	}{
		{"bot online sub", sub("stream.online", "42", botCB), true},
		{"bot chat sub", sub("channel.chat.message", "42", botCB), true},
		{"other broadcaster", sub("stream.online", "7", botCB), false},
		{"overlay type kept", sub("channel.cheer", "42", botCB), false},
		{"legacy callback still matched", sub("stream.offline", "42", "https://old.example/eventsub"), true},
	}
	for _, tc := range cases {
		if got := isBotSubscriptionFor(tc.s, "42"); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// Many toggles while a sync is running coalesce into one rerun, never more
// than one worker runs per user, and further requests are rejected as busy.
func TestScheduleReconcileCoalesces(t *testing.T) {
	release := make(chan struct{})
	var runs, active, maxActive int32
	orig := runReconcile
	runReconcile = func(_ nivek.NivekService, _ int, _, _ string) {
		n := atomic.AddInt32(&active, 1)
		if n > atomic.LoadInt32(&maxActive) {
			atomic.StoreInt32(&maxActive, n)
		}
		atomic.AddInt32(&runs, 1)
		<-release
		atomic.AddInt32(&active, -1)
	}
	defer func() { runReconcile = orig }()

	const uid = 4242
	if reconcileBusy(uid) {
		t.Fatal("idle user reported busy")
	}
	scheduleReconcile(nil, uid, "x", "1") // starts worker
	for i := 0; i < 100; i++ {
		scheduleReconcile(nil, uid, "x", "1") // all coalesce into one pending rerun
	}
	if !reconcileBusy(uid) {
		t.Fatal("running+pending user not reported busy")
	}
	release <- struct{}{} // finish first run; worker sleeps the gap then reruns
	release <- struct{}{} // finish the single coalesced rerun

	deadline := time.Now().Add(5 * time.Second)
	for {
		reconcileMu.Lock()
		_, exists := reconcileUsers[uid]
		reconcileMu.Unlock()
		if !exists {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not exit")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&runs); got != 2 {
		t.Errorf("runs = %d, want 2", got)
	}
	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Errorf("max concurrent runs = %d, want 1", got)
	}
}
