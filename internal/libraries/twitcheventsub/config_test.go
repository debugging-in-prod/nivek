package twitcheventsub

import "testing"

func TestConfigBotUserIDDefault(t *testing.T) {
	t.Parallel()
	if DefaultBotUserID != "1322716097" {
		t.Fatalf("DefaultBotUserID = %q, want 1322716097", DefaultBotUserID)
	}
	if got := (Config{}).botUserID(); got != DefaultBotUserID {
		t.Errorf("zero Config botUserID() = %q, want %q", got, DefaultBotUserID)
	}
	if got := (Config{BotUserID: " 555 "}).botUserID(); got != "555" {
		t.Errorf("botUserID() = %q, want 555", got)
	}
}
