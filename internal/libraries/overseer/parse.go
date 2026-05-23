package overseer

import (
	"fmt"
	"strconv"
	"strings"
)

// itemVocab is the set of chat-facing item nouns the parser recognizes for v0.
// Plural and case variations are normalized before lookup (the trailing "s"
// is stripped before lookup, so all entries are singular even when the
// DFHack-side job_type is plural — e.g. `block` -> `ConstructBlocks`).
var itemVocab = map[string]struct{}{
	"table":     {},
	"bed":       {},
	"door":      {},
	"chair":     {},
	"coffin":    {},
	"block":     {},
	"cabinet":   {},
	"chest":     {},
	"statue":    {},
	"floodgate": {},
}

// itemMaterialAllowlist restricts which materials are accepted for specific
// items where DF has a strict requirement that would otherwise queue an
// unbuildable order. Items not present here accept any material in
// materialVocab (the loose default — DF rejects the impossibility itself).
var itemMaterialAllowlist = map[string]map[string]struct{}{
	"bed": {"wood": {}},
}

// materialVocab is the set of chat-facing materials accepted in v0.
// "metal" is a meta-token meaning "any available metal, executor picks".
var materialVocab = map[string]struct{}{
	"stone": {}, "wood": {}, "bone": {}, "leather": {}, "cloth": {}, "shell": {},
	"iron": {}, "copper": {}, "bronze": {}, "steel": {}, "silver": {}, "gold": {},
	"metal": {},
}

// materialAliases normalizes adjectival forms to the canonical material token.
var materialAliases = map[string]string{
	"wooden": "wood",
}

// fillerWords are chat-tolerance tokens stripped before grammar matching.
var fillerWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "some": {},
	"me": {}, "us": {}, "please": {},
}

// ParseCommand parses the arguments of a `!DF` chat command into an Action.
// The caller is expected to have stripped the `!df` prefix before passing.
//
// Verbs (v0):
//   - `make [qty] <material> <item>` — manufacture (material is required;
//     DF Manager refuses to execute orders without one)
//   - `pause` — pause DF
//   - `unpause` — unpause DF
//   - `camera <x> <y> <z>` — recenter DF camera on the given tile (coords
//     accept space- and/or comma-separated forms: `137 115 150`,
//     `137,115,150`, `137, 115, 150` all parse the same)
//
// Tolerances (apply to all verbs): case-insensitive, whitespace-collapsing,
// filler-word stripping (a, an, the, some, me, us, please). Manufacture
// additionally allows plural-stripping on the item and the wooden->wood
// adjectival alias on the material.
func ParseCommand(args string) (Action, error) {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(args)))
	tokens = stripFillerWords(tokens)
	if len(tokens) == 0 {
		return Action{}, fmt.Errorf("empty command")
	}
	verb := tokens[0]
	rest := tokens[1:]
	switch verb {
	case "make":
		return parseManufacture(rest)
	case "pause":
		return parsePause(rest)
	case "unpause":
		return parseUnpause(rest)
	case "camera":
		return parseCamera(rest)
	default:
		return Action{}, fmt.Errorf("unknown verb: %q", verb)
	}
}

func parseManufacture(tokens []string) (Action, error) {
	if len(tokens) == 0 {
		return Action{}, fmt.Errorf("missing item")
	}

	qty := 1
	if n, parseErr := strconv.Atoi(tokens[0]); parseErr == nil {
		if n <= 0 {
			return Action{}, fmt.Errorf("quantity must be positive")
		}
		qty = n
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return Action{}, fmt.Errorf("missing item")
	}

	itemToken := strings.TrimSuffix(tokens[len(tokens)-1], "s")
	if _, ok := itemVocab[itemToken]; !ok {
		return Action{}, fmt.Errorf("unknown item: %q", itemToken)
	}

	var material *string
	pre := tokens[:len(tokens)-1]
	switch len(pre) {
	case 0:
		// DF Manager requires a material — orders without one queue as
		// "unknown material" and can never execute.
		return Action{}, fmt.Errorf("missing material")
	case 1:
		matToken := normalizeMaterial(pre[0])
		if _, ok := materialVocab[matToken]; !ok {
			return Action{}, fmt.Errorf("unknown material: %q", pre[0])
		}
		material = &matToken
	default:
		return Action{}, fmt.Errorf("extra tokens: %q", strings.Join(pre, " "))
	}

	if allowed, restricted := itemMaterialAllowlist[itemToken]; restricted {
		if _, ok := allowed[*material]; !ok {
			return Action{}, fmt.Errorf("material %q not allowed for item %q", *material, itemToken)
		}
	}

	return Action{
		Kind:     ActionKindManufacture,
		Item:     itemToken,
		Material: material,
		Quantity: qty,
	}, nil
}

func parsePause(rest []string) (Action, error) {
	if len(rest) > 0 {
		return Action{}, fmt.Errorf("extra tokens: %q", strings.Join(rest, " "))
	}
	return Action{Kind: ActionKindPause}, nil
}

func parseUnpause(rest []string) (Action, error) {
	if len(rest) > 0 {
		return Action{}, fmt.Errorf("extra tokens: %q", strings.Join(rest, " "))
	}
	return Action{Kind: ActionKindUnpause}, nil
}

func parseCamera(rest []string) (Action, error) {
	// Accept commas as separators in addition to whitespace:
	// `137,115,150`, `137, 115, 150`, `137 115 150` all parse the same.
	joined := strings.ReplaceAll(strings.Join(rest, " "), ",", " ")
	parts := strings.Fields(joined)
	if len(parts) != 3 {
		return Action{}, fmt.Errorf("camera needs 3 coordinates, got %d", len(parts))
	}
	coords := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return Action{}, fmt.Errorf("invalid coordinate %q", p)
		}
		coords[i] = n
	}
	return Action{
		Kind:     ActionKindCamera,
		Position: &Position{X: coords[0], Y: coords[1], Z: coords[2]},
	}, nil
}

func stripFillerWords(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if _, ok := fillerWords[t]; ok {
			continue
		}
		out = append(out, t)
	}
	return out
}

func normalizeMaterial(token string) string {
	if alias, ok := materialAliases[token]; ok {
		return alias
	}
	return token
}
