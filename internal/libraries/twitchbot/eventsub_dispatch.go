package twitchbot

import (
	"hash/fnv"
	"log"
	"runtime/debug"
)

// Chat-message dispatch sizing. The EventSub endpoint ACKs (204) before any
// command handler runs so slow core-api round trips can't trigger Twitch
// retries; these bound the work queued behind that ACK.
const (
	chatDispatchWorkers   = 4
	chatDispatchQueueSize = 256
)

// chatDispatcher runs channel.chat.message notifications off the HTTP request
// path on a fixed pool of workers. Messages are sharded by broadcaster so each
// channel's messages are handled in arrival order (handlers mutate per-channel
// state such as auto-shout lists and !dad counters), while one slow channel
// can't stall the others. Enqueue never blocks: when a shard's queue is full
// the message is dropped and logged rather than growing goroutines unboundedly.
type chatDispatcher struct {
	shards []chan *EventSubSubscriptionResponse
	handle func(*EventSubSubscriptionResponse)
}

func newChatDispatcher(workers, queueSize int, handle func(*EventSubSubscriptionResponse)) *chatDispatcher {
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 1
	}
	d := &chatDispatcher{
		shards: make([]chan *EventSubSubscriptionResponse, workers),
		handle: handle,
	}
	for i := range d.shards {
		ch := make(chan *EventSubSubscriptionResponse, queueSize)
		d.shards[i] = ch
		go d.run(ch)
	}
	return d
}

// enqueue queues n for its broadcaster's worker. It reports false (and logs)
// if that worker's queue is full and the message was dropped.
func (d *chatDispatcher) enqueue(n *EventSubSubscriptionResponse) bool {
	key := n.Subscription.Condition["broadcaster_user_id"]
	h := fnv.New32a()
	h.Write([]byte(key))
	select {
	case d.shards[h.Sum32()%uint32(len(d.shards))] <- n:
		return true
	default:
		log.Printf("[EVENTSUB] chat dispatch queue full; dropping chat message for broadcaster_id=%s", key)
		return false
	}
}

func (d *chatDispatcher) run(ch <-chan *EventSubSubscriptionResponse) {
	for n := range ch {
		d.safeHandle(n)
	}
}

// safeHandle isolates a panicking handler so the worker keeps draining.
func (d *chatDispatcher) safeHandle(n *EventSubSubscriptionResponse) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[EVENTSUB] panic handling chat message: %v\n%s", r, debug.Stack())
		}
	}()
	d.handle(n)
}
