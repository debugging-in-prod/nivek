package commands

import (
	"errors"
	"strings"
	"testing"
)

func intp(i int) *int       { return &i }
func boolp(b bool) *bool    { return &b }
func strp(s string) *string { return &s }

func TestNormalizeTrigger(t *testing.T) {
	ok := map[string]string{
		"!discord":                    "!discord",
		"  !Steam  ":                  "!steam",
		"wishlist":                    "!wishlist",
		"!c_source":                   "!c_source",
		"!a1":                         "!a1",
		"!" + strings.Repeat("a", 25): "!" + strings.Repeat("a", 25),
	}
	for in, want := range ok {
		got, err := NormalizeTrigger(in)
		if err != nil || got != want {
			t.Errorf("NormalizeTrigger(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	bad := []string{
		"", "!", "!!x", "!two words", "!emoji😀", "!dash-name", "!caret^",
		"!" + strings.Repeat("a", 26),
		// built-ins, in any case / without the bang
		"!dad", "!SO", "translate", "!stalk", "!newpromo", "!df",
	}
	for _, in := range bad {
		if _, err := NormalizeTrigger(in); !errors.Is(err, ErrValidation) {
			t.Errorf("NormalizeTrigger(%q) err = %v; want ErrValidation", in, err)
		}
	}
}

func TestValidateResponse(t *testing.T) {
	if got, err := ValidateResponse("  Join the discord: https://x  "); err != nil || got != "Join the discord: https://x" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := ValidateResponse(strings.Repeat("é", MaxResponseLen)); err != nil {
		t.Fatalf("500 runes should pass: %v", err)
	}
	bad := []string{
		"", "   ",
		strings.Repeat("a", MaxResponseLen+1),
		"line one\nline two",
		"/ban someone", ".timeout x", "!so someone",
		"  /ban someone", " .clear", "\t/me hi", // leading whitespace is trimmed first
	}
	for _, in := range bad {
		if _, err := ValidateResponse(in); !errors.Is(err, ErrValidation) {
			t.Errorf("ValidateResponse(%q) err = %v; want ErrValidation", in, err)
		}
	}
	// "/" or "." later in the text is fine.
	if _, err := ValidateResponse("see example.com/schedule"); err != nil {
		t.Errorf("mid-text slash/dot should pass: %v", err)
	}
}

func TestValidateCooldown(t *testing.T) {
	for _, ok := range []int{MinCooldownSecs, 30, MaxCooldownSecs} {
		if err := ValidateCooldown(ok); err != nil {
			t.Errorf("ValidateCooldown(%d) = %v", ok, err)
		}
	}
	for _, bad := range []int{-1, 0, MinCooldownSecs - 1, MaxCooldownSecs + 1} {
		if err := ValidateCooldown(bad); !errors.Is(err, ErrValidation) {
			t.Errorf("ValidateCooldown(%d) = %v; want ErrValidation", bad, err)
		}
	}
}

func TestValidateMinRole(t *testing.T) {
	for _, r := range []string{"everyone", "sub", "vip", "mod", "Broadcaster"} {
		if _, err := ValidateMinRole(r); err != nil {
			t.Errorf("ValidateMinRole(%q) = %v", r, err)
		}
	}
	for _, r := range []string{"", "admin", "moderator"} {
		if _, err := ValidateMinRole(r); !errors.Is(err, ErrValidation) {
			t.Errorf("ValidateMinRole(%q) = %v; want ErrValidation", r, err)
		}
	}
}

func TestValidateDescription(t *testing.T) {
	if _, err := ValidateDescription(""); err != nil {
		t.Fatalf("empty description should be allowed: %v", err)
	}
	if _, err := ValidateDescription(strings.Repeat("a", MaxDescriptionLen+1)); !errors.Is(err, ErrValidation) {
		t.Fatalf("over-long description should fail, got %v", err)
	}
}

func TestCustomCommandInputNormalize(t *testing.T) {
	out, err := CustomCommandInput{
		Trigger:      "Discord",
		ResponseTmpl: " https://discord.gg/x ",
		CooldownSecs: intp(30),
	}.Normalize()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Trigger != "!discord" || out.ResponseTmpl != "https://discord.gg/x" ||
		out.MinRole != "everyone" || *out.CooldownSecs != 30 || !*out.Enabled {
		t.Fatalf("unexpected normalized output: %+v", out)
	}

	disabled, err := CustomCommandInput{
		Trigger: "!x", ResponseTmpl: "y", MinRole: "mod", CooldownSecs: intp(5), Enabled: boolp(false),
	}.Normalize()
	if err != nil || *disabled.Enabled || disabled.MinRole != "mod" {
		t.Fatalf("got %+v, %v", disabled, err)
	}

	cases := []CustomCommandInput{
		{Trigger: "!x", ResponseTmpl: "y"},                                       // cooldown missing
		{Trigger: "!x", ResponseTmpl: "y", CooldownSecs: intp(0)},                // cooldown too low
		{Trigger: "!x", ResponseTmpl: "y", CooldownSecs: intp(4)},                // cooldown too low
		{Trigger: "!x", ResponseTmpl: "y", CooldownSecs: intp(10), MinRole: "x"}, // bad role
		{Trigger: "!fish", ResponseTmpl: "y", CooldownSecs: intp(10)},            // built-in
		{Trigger: "!x", ResponseTmpl: "", CooldownSecs: intp(10)},                // empty response
	}
	for i, c := range cases {
		if _, err := c.Normalize(); !errors.Is(err, ErrValidation) {
			t.Errorf("case %d: err = %v; want ErrValidation", i, err)
		}
	}
}

func TestCommandSettingInputNormalize(t *testing.T) {
	// All nil = inherit everything.
	if out, err := (CommandSettingInput{}).Normalize(); err != nil || out.Enabled != nil || out.MinRole != nil || out.CooldownSecs != nil {
		t.Fatalf("got %+v, %v", out, err)
	}
	out, err := CommandSettingInput{Enabled: boolp(false), MinRole: strp("MOD"), CooldownSecs: intp(60)}.Normalize()
	if err != nil || *out.Enabled || *out.MinRole != "mod" || *out.CooldownSecs != 60 {
		t.Fatalf("got %+v, %v", out, err)
	}
	if _, err := (CommandSettingInput{CooldownSecs: intp(1)}).Normalize(); !errors.Is(err, ErrValidation) {
		t.Fatalf("cooldown 1 should fail, got %v", err)
	}
	if _, err := (CommandSettingInput{MinRole: strp("owner")}).Normalize(); !errors.Is(err, ErrValidation) {
		t.Fatalf("bad role should fail, got %v", err)
	}
}

func TestReservedTriggersCoverSeededBuiltins(t *testing.T) {
	// Keep in sync with the global seed in database/init.sql.
	for _, trig := range []string{
		"!dad", "!fish", "!bread", "!lurk", "!joinme", "!banish", "!pbcommands",
		"!newpromo", "!stalk", "!hangman", "!so", "!translate",
	} {
		if !IsReservedTrigger(trig) {
			t.Errorf("%s should be reserved", trig)
		}
	}
}

func TestNullable(t *testing.T) {
	if nullable[int](nil) != nil {
		t.Fatal("nil pointer should map to nil")
	}
	if nullable(intp(7)) != 7 {
		t.Fatal("non-nil pointer should map to its value")
	}
}
