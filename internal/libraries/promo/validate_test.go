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
	for _, bad := range []string{"", "   ", strings.Repeat("a", MaxMessageLen+1)} {
		if _, err := ValidateMessage(bad); err == nil {
			t.Errorf("ValidateMessage(%d chars) should fail", len(bad))
		}
	}
}
