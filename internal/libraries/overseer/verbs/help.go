package verbs

import (
	"fmt"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// ParseHelp handles `help` — no arguments. The help verb is intercepted
// by the chat bot (no executor round-trip) and answered with the verb
// list directly to chat, so there is no SubmitHelp.
func ParseHelp(rest []string) (wire.Action, error) {
	if len(rest) > 0 {
		return wire.Action{}, fmt.Errorf("extra tokens: %q", strings.Join(rest, " "))
	}
	return wire.Action{Kind: wire.ActionKindHelp}, nil
}
