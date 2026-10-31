package commands

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Limits for dashboard-managed per-channel commands. These are enforced by the
// core-api editor endpoints only; the bot's runtime behaviour is unchanged.
const (
	// MinCooldownSecs is the floor for any cooldown set from the dashboard, so
	// an editor can never configure a command (or override) without one.
	MinCooldownSecs = 5
	// MaxCooldownSecs caps cooldowns at one hour.
	MaxCooldownSecs = 3600
	// MaxResponseLen matches Twitch's 500-character chat message limit.
	MaxResponseLen = 500
	// MaxDescriptionLen bounds the optional blurb shown on the dashboard.
	MaxDescriptionLen = 200
	// MaxCustomCommandsPerChannel caps how many custom commands one channel can
	// own, keeping the bot's per-channel dispatch map small.
	MaxCustomCommandsPerChannel = 50
)

// Roles accepted for min_role, mirroring the CHECK constraint on
// nivek.command.min_role.
var validRoles = map[string]bool{
	"everyone":    true,
	"sub":         true,
	"vip":         true,
	"mod":         true,
	"broadcaster": true,
}

// triggerPattern: "!" followed by 1-25 lowercase letters, digits or
// underscores. No spaces (the bot matches a single whitespace-separated word)
// and lowercase only (the bot lowercases incoming chat before matching).
var triggerPattern = regexp.MustCompile(`^![a-z0-9_]{1,25}$`)

// reservedTriggers are built-in command names a channel may never reuse for a
// custom command. The authoritative list is the global rows in nivek.command
// (checked by the service as well); this static copy covers triggers the bot
// intercepts before the DB-driven dispatch, plus suspended ones, so a missing
// seed row can't open the name up.
var reservedTriggers = map[string]bool{
	"!dad":        true,
	"!fish":       true,
	"!bread":      true,
	"!lurk":       true,
	"!joinme":     true,
	"!banish":     true,
	"!pbcommands": true,
	"!newpromo":   true,
	"!stalk":      true,
	"!hangman":    true,
	"!so":         true,
	"!translate":  true,
	"!df":         true,
	"!cmd":        true,
}

// ErrValidation wraps every input-validation failure so handlers can map it to
// a 400 with errors.Is.
var ErrValidation = errors.New("invalid input")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

// IsReservedTrigger reports whether trigger is one of the bot's built-in names.
func IsReservedTrigger(trigger string) bool {
	return reservedTriggers[strings.ToLower(strings.TrimSpace(trigger))]
}

// NormalizeTrigger trims and lowercases a trigger, adding a leading "!" when the
// user omitted it, then validates the format and refuses built-in names.
func NormalizeTrigger(raw string) (string, error) {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t != "" && !strings.HasPrefix(t, "!") {
		t = "!" + t
	}
	if !triggerPattern.MatchString(t) {
		return "", invalid("trigger must be \"!\" followed by 1-25 lowercase letters, digits or underscores")
	}
	if IsReservedTrigger(t) {
		return "", invalid("%s is a built-in command and can't be used as a custom trigger", t)
	}
	return t, nil
}

// ValidateResponse trims and checks a custom command's reply text.
func ValidateResponse(raw string) (string, error) {
	r := strings.TrimSpace(raw)
	if r == "" {
		return "", invalid("response is required")
	}
	if utf8.RuneCountInString(r) > MaxResponseLen {
		return "", invalid("response must be at most %d characters", MaxResponseLen)
	}
	for _, ch := range r {
		if unicode.IsControl(ch) {
			return "", invalid("response must be a single line without control characters")
		}
	}
	// A reply starting with "/" or "." could read as a chat command, and one
	// starting with "!" could trigger this or another bot — refuse all three so
	// a custom command can never kick off a reply loop.
	switch r[0] {
	case '/', '.', '!':
		return "", invalid("response can't start with \"/\", \".\" or \"!\"")
	}
	return r, nil
}

// ValidateDescription trims and length-checks the optional description.
func ValidateDescription(raw string) (string, error) {
	d := strings.TrimSpace(raw)
	if utf8.RuneCountInString(d) > MaxDescriptionLen {
		return "", invalid("description must be at most %d characters", MaxDescriptionLen)
	}
	for _, ch := range d {
		if unicode.IsControl(ch) {
			return "", invalid("description must not contain control characters")
		}
	}
	return d, nil
}

// ValidateMinRole checks a role against the allowed set.
func ValidateMinRole(role string) (string, error) {
	r := strings.ToLower(strings.TrimSpace(role))
	if !validRoles[r] {
		return "", invalid("min_role must be one of everyone, sub, vip, mod, broadcaster")
	}
	return r, nil
}

// ValidateCooldown enforces MinCooldownSecs..MaxCooldownSecs.
func ValidateCooldown(secs int) error {
	if secs < MinCooldownSecs || secs > MaxCooldownSecs {
		return invalid("cooldown_secs must be between %d and %d", MinCooldownSecs, MaxCooldownSecs)
	}
	return nil
}

// CustomCommandInput is the editable shape of a channel custom command.
type CustomCommandInput struct {
	Trigger      string `json:"trigger"`
	ResponseTmpl string `json:"response_tmpl"`
	Description  string `json:"description"`
	MinRole      string `json:"min_role"`
	CooldownSecs *int   `json:"cooldown_secs"`
	Enabled      *bool  `json:"enabled"`
}

// Normalize validates every field and returns a cleaned copy. min_role
// defaults to "everyone" and enabled to true; cooldown_secs is required.
func (in CustomCommandInput) Normalize() (CustomCommandInput, error) {
	out := CustomCommandInput{}
	var err error

	if out.Trigger, err = NormalizeTrigger(in.Trigger); err != nil {
		return out, err
	}
	if out.ResponseTmpl, err = ValidateResponse(in.ResponseTmpl); err != nil {
		return out, err
	}
	if out.Description, err = ValidateDescription(in.Description); err != nil {
		return out, err
	}
	role := in.MinRole
	if strings.TrimSpace(role) == "" {
		role = "everyone"
	}
	if out.MinRole, err = ValidateMinRole(role); err != nil {
		return out, err
	}
	if in.CooldownSecs == nil {
		return out, invalid("cooldown_secs is required")
	}
	if err = ValidateCooldown(*in.CooldownSecs); err != nil {
		return out, err
	}
	cd := *in.CooldownSecs
	out.CooldownSecs = &cd
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	out.Enabled = &enabled
	return out, nil
}

// CommandSettingInput is a per-channel override of a global built-in command.
// A nil field means "inherit the command's default".
type CommandSettingInput struct {
	Enabled      *bool   `json:"enabled"`
	MinRole      *string `json:"min_role"`
	CooldownSecs *int    `json:"cooldown_secs"`
}

// Normalize validates the override's non-nil fields.
func (in CommandSettingInput) Normalize() (CommandSettingInput, error) {
	out := CommandSettingInput{Enabled: in.Enabled}
	if in.MinRole != nil {
		r, err := ValidateMinRole(*in.MinRole)
		if err != nil {
			return out, err
		}
		out.MinRole = &r
	}
	if in.CooldownSecs != nil {
		if err := ValidateCooldown(*in.CooldownSecs); err != nil {
			return out, err
		}
		cd := *in.CooldownSecs
		out.CooldownSecs = &cd
	}
	return out, nil
}
