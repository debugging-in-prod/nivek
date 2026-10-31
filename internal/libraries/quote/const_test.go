package quote

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"  hello   world ":        "hello world",
		"line1\nline2\r\nline3":   "line1 line2 line3",
		"tab\tsep\x00null\x07bel": "tab sep null bel",
		"":                        "",
		"   \n\t ":                "",
		"日本語 ok":                  "日本語 ok",
	}
	for in, want := range cases {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidText(t *testing.T) {
	t.Parallel()
	if _, ok := ValidText("   "); ok {
		t.Error("blank text must be invalid")
	}
	if got, ok := ValidText(" a\nb "); !ok || got != "a b" {
		t.Errorf("ValidText = (%q, %v), want (\"a b\", true)", got, ok)
	}
	if _, ok := ValidText(strings.Repeat("x", MaxTextLen)); !ok {
		t.Error("text at the limit must be valid")
	}
	if _, ok := ValidText(strings.Repeat("x", MaxTextLen+1)); ok {
		t.Error("text over the limit must be invalid")
	}
	// Limit counts characters, not bytes.
	if _, ok := ValidText(strings.Repeat("é", MaxTextLen)); !ok {
		t.Error("multi-byte text at the character limit must be valid")
	}
}
