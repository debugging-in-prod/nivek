package overseer

import (
	"fmt"
	"strconv"
	"strings"
)

// itemVocab is the set of chat-facing item nouns the parser recognizes for v0.
// Plural and case variations are normalized before lookup.
var itemVocab = map[string]struct{}{
	"table": {},
	"bed":   {},
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
//   - `make [qty] [material] <item>` — manufacture
//   - `pause` — pause DF
//   - `unpause` — unpause DF
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
		// no material slot — executor picks
	case 1:
		matToken := normalizeMaterial(pre[0])
		if _, ok := materialVocab[matToken]; !ok {
			return Action{}, fmt.Errorf("unknown material: %q", pre[0])
		}
		material = &matToken
	default:
		return Action{}, fmt.Errorf("extra tokens: %q", strings.Join(pre, " "))
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
