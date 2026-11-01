package twitchbot

import (
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
)

// sharedChatPayload is a channel.chat.message event as Twitch sends it for a
// Shared Chat message relayed from channel 777 into somechannel (id 9): non-null
// source_* fields, a source_badges array, plus reply and cheer objects. The
// source_badges array used to fail decoding against a string field.
func sharedChatPayload(sourceID, text string) string {
	return `{
  "broadcaster_user_id": "9",
  "broadcaster_user_login": "somechannel",
  "broadcaster_user_name": "SomeChannel",
  "source_broadcaster_user_id": "` + sourceID + `",
  "source_broadcaster_user_login": "otherchannel",
  "source_broadcaster_user_name": "OtherChannel",
  "chatter_user_id": "42",
  "chatter_user_login": "alice",
  "chatter_user_name": "Alice",
  "message_id": "relay-1",
  "source_message_id": "orig-1",
  "message": {
    "text": "` + text + `",
    "fragments": [
      {"type": "text", "text": "` + text + `", "cheermote": null, "emote": null, "mention": null}
    ]
  },
  "color": "#00FF7F",
  "badges": [
    {"set_id": "subscriber", "id": "12", "info": "16"}
  ],
  "source_badges": [
    {"set_id": "moderator", "id": "1", "info": ""},
    {"set_id": "subscriber", "id": "0", "info": "3"}
  ],
  "message_type": "text",
  "cheer": {"bits": 100},
  "reply": {
    "parent_message_id": "p-1",
    "parent_message_body": "first message",
    "parent_user_id": "43",
    "parent_user_name": "Bob",
    "parent_user_login": "bob",
    "thread_message_id": "t-1",
    "thread_user_id": "43",
    "thread_user_name": "Bob",
    "thread_user_login": "bob"
  },
  "channel_points_custom_reward_id": null,
  "channel_points_animation_id": null
}`
}

func TestChatMessageEventDecodesSharedChatPayload(t *testing.T) {
	t.Parallel()
	var ev chatMessageEvent
	if err := json.Unmarshal([]byte(sharedChatPayload("777", "!fish")), &ev); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.SourceBroadcasterUserId == nil || *ev.SourceBroadcasterUserId != "777" {
		t.Errorf("source_broadcaster_user_id = %v, want 777", ev.SourceBroadcasterUserId)
	}
	if len(ev.SourceBadges) != 2 || ev.SourceBadges[0].SetId != "moderator" || ev.SourceBadges[1].Info != "3" {
		t.Errorf("source_badges = %+v", ev.SourceBadges)
	}
	if ev.Cheer == nil || ev.Cheer.Bits != 100 {
		t.Errorf("cheer = %+v, want bits 100", ev.Cheer)
	}
	want := chatReply{
		ParentMessageId: "p-1", ParentMessageBody: "first message",
		ParentUserId: "43", ParentUserName: "Bob", ParentUserLogin: "bob",
		ThreadMessageId: "t-1", ThreadUserId: "43", ThreadUserName: "Bob", ThreadUserLogin: "bob",
	}
	if ev.Reply == nil || *ev.Reply != want {
		t.Errorf("reply = %+v, want %+v", ev.Reply, want)
	}
	if ev.Message.Text != "!fish" || ev.ChatterUserLogin != "alice" || len(ev.Badges) != 1 {
		t.Errorf("core fields decoded wrong: %+v", ev)
	}
	if !ev.isForeignSharedChat() {
		t.Error("isForeignSharedChat = false for a relay from another channel")
	}
}

func TestChatMessageEventDecodesNormalPayload(t *testing.T) {
	t.Parallel()
	payload := `{
  "broadcaster_user_id": "9",
  "broadcaster_user_login": "somechannel",
  "broadcaster_user_name": "SomeChannel",
  "source_broadcaster_user_id": null,
  "source_broadcaster_user_login": null,
  "source_broadcaster_user_name": null,
  "chatter_user_id": "42",
  "chatter_user_login": "alice",
  "chatter_user_name": "Alice",
  "message_id": "m-1",
  "source_message_id": null,
  "message": {"text": "Hi chat", "fragments": [{"type": "text", "text": "Hi chat", "cheermote": null, "emote": null, "mention": null}]},
  "color": "",
  "badges": [],
  "source_badges": null,
  "message_type": "text",
  "cheer": null,
  "reply": null,
  "channel_points_custom_reward_id": null,
  "channel_points_animation_id": null
}`
	var ev chatMessageEvent
	if err := json.Unmarshal([]byte(payload), &ev); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.SourceBroadcasterUserId != nil || ev.SourceBadges != nil || ev.Cheer != nil || ev.Reply != nil {
		t.Errorf("null fields decoded non-nil: %+v", ev)
	}
	if ev.Message.Text != "Hi chat" || ev.BroadcasterUserId != "9" {
		t.Errorf("core fields decoded wrong: %+v", ev)
	}
	if ev.isForeignSharedChat() {
		t.Error("isForeignSharedChat = true for an ordinary message")
	}
}

// A command relayed into this channel from another channel of a Shared Chat
// session must produce no reply, run no handler, and touch no counters.
func TestHandleWebhookMessageIgnoresForeignSharedChat(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, _ := dispatchBot(&calls)
	b.chatLines = newChatLineCounter()

	for i, text := range []string{"!fish", "!hi", "^ ^"} {
		ev := strings.Replace(sharedChatPayload("777", text), `"relay-1"`, `"relay-`+string(rune('a'+i))+`"`, 1)
		b.handleWebhookMessage(&EventSubSubscriptionResponse{Event: []byte(ev)})
	}

	if calls.Load() != 0 {
		t.Errorf("builtin handler ran %d times, want 0", calls.Load())
	}
	if got := drainSay(b); len(got) != 0 {
		t.Errorf("queued %q, want nothing", got)
	}
	if got := b.chatLines.get("somechannel"); got != 0 {
		t.Errorf("chat lines counted = %d, want 0", got)
	}
}

// The origin channel's own copy of a shared-chat message (source == broadcaster)
// is handled exactly like an ordinary message.
func TestHandleWebhookMessageHandlesOriginSharedChat(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	b, _ := dispatchBot(&calls)
	b.chatLines = newChatLineCounter()

	b.handleWebhookMessage(&EventSubSubscriptionResponse{Event: []byte(sharedChatPayload("9", "!fish"))})
	if calls.Load() != 1 {
		t.Fatalf("!fish ran %d times, want 1", calls.Load())
	}
	if got := b.chatLines.get("somechannel"); got != 1 {
		t.Errorf("chat lines counted = %d, want 1", got)
	}

	// Custom command reply still goes out for the origin copy.
	ev := strings.Replace(sharedChatPayload("9", "!hi"), `"relay-1"`, `"relay-2"`, 1)
	b.handleWebhookMessage(&EventSubSubscriptionResponse{Event: []byte(ev)})
	if got := drainSay(b); len(got) != 1 || got[0] != "hello" {
		t.Errorf("queued %q, want [\"hello\"]", got)
	}
}

// An empty source id is treated as "not shared" (same as null).
func TestIsForeignSharedChatEmptySource(t *testing.T) {
	t.Parallel()
	empty := ""
	ev := &chatMessageEvent{BroadcasterUserId: "9", SourceBroadcasterUserId: &empty}
	if ev.isForeignSharedChat() {
		t.Error("empty source id treated as foreign")
	}
}
