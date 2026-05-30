package verbs

import (
	"fmt"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// ParsePause handles `pause` — no arguments.
func ParsePause(rest []string) (wire.Action, error) {
	if len(rest) > 0 {
		return wire.Action{}, fmt.Errorf("extra tokens: %q", strings.Join(rest, " "))
	}
	return wire.Action{Kind: wire.ActionKindPause}, nil
}

// ParseUnpause handles `unpause` — no arguments.
func ParseUnpause(rest []string) (wire.Action, error) {
	if len(rest) > 0 {
		return wire.Action{}, fmt.Errorf("extra tokens: %q", strings.Join(rest, " "))
	}
	return wire.Action{Kind: wire.ActionKindUnpause}, nil
}

// SubmitPause flips DF's global pause flag on.
func SubmitPause(ex Executor, _ wire.Action) error {
	return ex.RunLua("df.global.pause_state=true")
}

// SubmitUnpause flips DF's global pause flag off.
func SubmitUnpause(ex Executor, _ wire.Action) error {
	return ex.RunLua("df.global.pause_state=false")
}
