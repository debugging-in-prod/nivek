package user

import (
	"testing"

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
		name     string
		s        twitcheventsub.EventSubSubscription
		callback string
		want     bool
	}{
		{"bot online sub", sub("stream.online", "42", botCB), botCB, true},
		{"bot chat sub", sub("channel.chat.message", "42", botCB), botCB, true},
		{"other broadcaster", sub("stream.online", "7", botCB), botCB, false},
		{"overlay type kept", sub("channel.cheer", "42", botCB), botCB, false},
		{"overlay callback kept", sub("stream.online", "42", "https://x/api/overlay/eventsub"), botCB, false},
		{"no callback configured matches by type", sub("stream.offline", "42", "anything"), "", true},
	}
	for _, tc := range cases {
		if got := isBotSubscriptionFor(tc.s, "42", tc.callback); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
