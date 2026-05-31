package verbs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

func TestParseBrew(t *testing.T) {
	tests := []struct {
		name       string
		input      []string
		wantSource string
		wantQty    int
		wantErr    bool
		errMatch   string
	}{
		{name: "bare fruit", input: []string{"fruit"}, wantSource: "fruit", wantQty: 1},
		{name: "plural", input: []string{"fruits"}, wantSource: "fruit", wantQty: 1},
		{name: "qty + source", input: []string{"5", "fruit"}, wantSource: "fruit", wantQty: 5},
		{name: "plant", input: []string{"plant"}, wantSource: "plant", wantQty: 1},
		{name: "missing args", input: []string{}, wantErr: true},
		{name: "zero qty rejected", input: []string{"0", "fruit"}, wantErr: true},
		{name: "unknown source", input: []string{"beer"}, wantErr: true, errMatch: "unknown brew source"},
		{name: "extra tokens", input: []string{"foo", "fruit"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBrew(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.errMatch != "" && !strings.Contains(err.Error(), tt.errMatch) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMatch)
				}
				return
			}
			if got.Item != tt.wantSource || got.Quantity != tt.wantQty {
				t.Errorf("got source=%q qty=%d, want source=%q qty=%d",
					got.Item, got.Quantity, tt.wantSource, tt.wantQty)
			}
		})
	}
}

func TestSubmitBrew_WorkorderPayload(t *testing.T) {
	ex := &fakeExecutor{}
	err := SubmitBrew(ex, wire.Action{
		Kind:     wire.ActionKindBrew,
		Item:     "fruit",
		Quantity: 3,
	})
	if err != nil {
		t.Fatalf("SubmitBrew: %v", err)
	}
	if len(ex.DFHackCalls) != 1 {
		t.Fatalf("expected 1 dfhack call, got %d", len(ex.DFHackCalls))
	}
	call := ex.DFHackCalls[0]
	if call[0] != "workorder" {
		t.Errorf("call[0] = %q, want workorder", call[0])
	}
	var req customReactionRequest
	if err := json.Unmarshal([]byte(call[1]), &req); err != nil {
		t.Fatalf("payload not valid JSON: %v", err)
	}
	if req.Job != "CustomReaction" {
		t.Errorf("Job = %q, want CustomReaction", req.Job)
	}
	if req.Reaction != "BREW_DRINK_FROM_PLANT_GROWTH" {
		t.Errorf("Reaction = %q, want BREW_DRINK_FROM_PLANT_GROWTH", req.Reaction)
	}
	if req.AmountTotal != 3 {
		t.Errorf("AmountTotal = %d, want 3", req.AmountTotal)
	}
}

func TestSubmitBrew_DefaultQty(t *testing.T) {
	ex := &fakeExecutor{}
	if err := SubmitBrew(ex, wire.Action{Item: "plant"}); err != nil {
		t.Fatalf("SubmitBrew: %v", err)
	}
	var req customReactionRequest
	if err := json.Unmarshal([]byte(ex.DFHackCalls[0][1]), &req); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if req.AmountTotal != 1 {
		t.Errorf("default qty = %d, want 1", req.AmountTotal)
	}
}

func TestSubmitBrew_UnknownSource(t *testing.T) {
	ex := &fakeExecutor{}
	if err := SubmitBrew(ex, wire.Action{Item: "beer"}); err == nil {
		t.Error("expected error for unknown brew source")
	}
}
