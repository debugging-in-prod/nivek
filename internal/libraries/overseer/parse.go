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

// ParseManufacture parses the arguments of a `!DF` chat command into an Action.
// The caller is expected to have stripped the `!df` prefix before passing.
//
// Grammar (strict positional, v0 subset): `make [qty] <item>`.
// Material slot defined by the full grammar is not yet implemented; if present
// it will currently land in the item position and be rejected.
func ParseManufacture(args string) (Action, error) {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(args)))
	if len(tokens) == 0 {
		return Action{}, fmt.Errorf("empty command")
	}
	if tokens[0] != "make" {
		return Action{}, fmt.Errorf("unknown verb: %q", tokens[0])
	}
	tokens = tokens[1:]
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

	return Action{
		Kind:     ActionKindManufacture,
		Item:     itemToken,
		Quantity: qty,
	}, nil
}
