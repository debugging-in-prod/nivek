package overseer

import (
	"fmt"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/verbs"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// ParseCommand parses the arguments of a `!DF` chat command into a
// wire.Action by dispatching to the right per-verb parser in package
// verbs. The caller is expected to have stripped the `!df` prefix
// before passing.
//
// Verbs supported:
//   - make [qty] <material> <item> — manufacture via workorder
//   - place <item> <x> <y> <z> — queue a building (via DFHack buildingplan)
//   - mine / channel / digramp / cuttree <coords> — region designations
//   - stockpile <category> <coords> — top-level-category stockpile
//   - zone <type> <coords> — office/bedroom/dormitory civzone
//   - brew [qty] <fruit|plant> — brewing workorder
//   - taskat #<workshop_id> [qty] <material> <item> — direct workshop job
//   - camera <x> <y> <z> — recenter DF camera (z is elevation)
//   - appoint <position> <id> — noble assignment
//   - pause / unpause — flip DF's global pause flag
//   - help — bot-side handled; short-circuits before the executor
//
// Tolerances (apply to all verbs): case-insensitive,
// whitespace-collapsing, filler-word stripping (a, an, the, some, me,
// us, please, drink, from). Per-verb tolerances live in their verb
// files in package verbs.
func ParseCommand(args string) (wire.Action, error) {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(args)))
	tokens = verbs.StripFillerWords(tokens)
	if len(tokens) == 0 {
		return wire.Action{}, fmt.Errorf("empty command")
	}
	verb := tokens[0]
	rest := tokens[1:]
	switch verb {
	case "make":
		return verbs.ParseManufacture(rest)
	case "pause":
		return verbs.ParsePause(rest)
	case "unpause":
		return verbs.ParseUnpause(rest)
	case "camera":
		return verbs.ParseCamera(rest)
	case "help":
		return verbs.ParseHelp(rest)
	case "place":
		return verbs.ParsePlace(rest)
	case "brew":
		return verbs.ParseBrew(rest)
	case "mine":
		return verbs.ParseMine(rest)
	case "channel":
		return verbs.ParseChannel(rest)
	case "digramp":
		return verbs.ParseDigRamp(rest)
	case "cuttree":
		return verbs.ParseCutTree(rest)
	case "stockpile":
		return verbs.ParseStockpile(rest)
	case "zone":
		return verbs.ParseZone(rest)
	case "taskat":
		return verbs.ParseTaskat(rest)
	case "appoint":
		return verbs.ParseAppoint(rest)
	default:
		return wire.Action{}, fmt.Errorf("unknown verb: %q", verb)
	}
}

// RejectReason is re-exported here so external callers (bot.go) can
// continue importing it via the overseer package, matching the existing
// public API surface. The concrete type lives in wire.
type RejectReason = wire.RejectReason
