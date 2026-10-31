package promo

import (
	"strings"
	"testing"
)

func TestValidateMessage(t *testing.T) {
	if got, err := ValidateMessage("  hello  "); err != nil || got != "hello" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := ValidateMessage(strings.Repeat("ü", MaxMessageLen)); err != nil {
		t.Fatalf("%d runes should pass: %v", MaxMessageLen, err)
	}
	for _, bad := range []string{
		"", "   ", strings.Repeat("a", MaxMessageLen+1),
		"/ban someone", ".timeout someone 600", "  /clear", "\t.me hi",
	} {
		if _, err := ValidateMessage(bad); err == nil {
			t.Errorf("ValidateMessage(%.30q) should fail", bad)
		}
	}
	// A slash or dot later in the message is fine.
	if _, err := ValidateMessage("check out example.com/stream"); err != nil {
		t.Errorf("mid-message slash/dot should pass: %v", err)
	}
}
