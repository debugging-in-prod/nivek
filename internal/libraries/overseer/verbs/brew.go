package verbs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// brewSourceToReaction maps chat-facing brew sources to DFHack reaction
// IDs used by the workorder script's CustomReaction job_type. Confirmed
// via `orders export` — these are the two reaction strings DF Premium
// uses for the default brewing templates.
var brewSourceToReaction = map[string]string{
	"fruit": "BREW_DRINK_FROM_PLANT_GROWTH",
	"plant": "BREW_DRINK_FROM_PLANT",
}

// customReactionRequest is the workorder JSON payload for CustomReaction
// jobs (brewing, etc.) — these don't use job_type names like MakeTable
// but instead specify "job":"CustomReaction" + "reaction":<reaction_id>.
type customReactionRequest struct {
	Job         string `json:"job"`
	Reaction    string `json:"reaction"`
	AmountTotal int    `json:"amount_total"`
}

// ParseBrew handles `brew [qty] <source>` — source is fruit or plant.
// The verbose chatter-friendly forms like `brew drink from fruit` parse
// the same because `drink` and `from` are filler-stripped.
func ParseBrew(tokens []string) (wire.Action, error) {
	if len(tokens) == 0 {
		return wire.Action{}, fmt.Errorf("brew needs a source (fruit or plant)")
	}
	qty := 1
	if n, parseErr := strconv.Atoi(tokens[0]); parseErr == nil {
		if n <= 0 {
			return wire.Action{}, fmt.Errorf("quantity must be positive")
		}
		qty = n
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return wire.Action{}, fmt.Errorf("brew needs a source (fruit or plant)")
	}
	// Source is the last token (plural-stripped); anything before it is
	// unexpected and gets rejected so typos don't quietly succeed.
	source := strings.TrimSuffix(tokens[len(tokens)-1], "s")
	if _, ok := brewSourceToReaction[source]; !ok {
		return wire.Action{}, fmt.Errorf("unknown brew source: %q (expected fruit or plant)", source)
	}
	if len(tokens) > 1 {
		return wire.Action{}, fmt.Errorf("extra tokens: %q", strings.Join(tokens[:len(tokens)-1], " "))
	}
	return wire.Action{
		Kind:     wire.ActionKindBrew,
		Item:     source,
		Quantity: qty,
	}, nil
}

// SubmitBrew queues a CustomReaction workorder for the requested
// brewing source.
func SubmitBrew(ex Executor, action wire.Action) error {
	reaction, ok := brewSourceToReaction[action.Item]
	if !ok {
		return fmt.Errorf("no DFHack reaction mapping for brew source: %s", action.Item)
	}
	qty := action.Quantity
	if qty <= 0 {
		qty = 1
	}
	payload, err := json.Marshal(customReactionRequest{
		Job:         "CustomReaction",
		Reaction:    reaction,
		AmountTotal: qty,
	})
	if err != nil {
		return fmt.Errorf("marshal brew workorder json: %w", err)
	}
	return ex.RunDFHack("workorder", string(payload))
}
