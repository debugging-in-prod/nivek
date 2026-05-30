package overseer

import (
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// TestParseCommand_DispatchesToEachVerb is the integration test for
// ParseCommand: it verifies every recognized verb is reachable from the
// top-level dispatcher and routes to the right per-verb parser, which
// is the property most likely to silently break when a new verb is
// added without wiring the switch case.
func TestParseCommand_DispatchesToEachVerb(t *testing.T) {
	tests := []struct {
		raw  string
		kind wire.ActionKind
	}{
		{"make 3 wood table", wire.ActionKindManufacture},
		{"pause", wire.ActionKindPause},
		{"unpause", wire.ActionKindUnpause},
		{"camera 50 50 100", wire.ActionKindCamera},
		{"help", wire.ActionKindHelp},
		{"place forge (5, 5, 5)", wire.ActionKindPlace},
		{"brew 5 fruit", wire.ActionKindBrew},
		{"mine (1, 2, 3) (5, 6, 3)", wire.ActionKindMine},
		{"channel (1, 2, 3)", wire.ActionKindChannel},
		{"digramp (1, 2, 3)", wire.ActionKindDigRamp},
		{"cuttree (1, 2, 3) (5, 5, 3)", wire.ActionKindCutTree},
		{"stockpile wood (1, 2, 3) (4, 5, 3)", wire.ActionKindStockpile},
		{"zone office (10, 20, 30) (15, 25, 30)", wire.ActionKindZone},
		{"taskat #2 wood table", wire.ActionKindTaskat},
		{"appoint manager #4", wire.ActionKindAppoint},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := ParseCommand(tt.raw)
			if err != nil {
				t.Fatalf("ParseCommand(%q): %v", tt.raw, err)
			}
			if got.Kind != tt.kind {
				t.Errorf("kind = %q, want %q", got.Kind, tt.kind)
			}
		})
	}
}

func TestParseCommand_StripsFillerWords(t *testing.T) {
	// Filler words ("the", "some", "please") should be stripped before
	// the verb dispatcher sees the tokens.
	got, err := ParseCommand("please make 3 the wood table")
	if err != nil {
		t.Fatalf("ParseCommand: %v", err)
	}
	if got.Kind != wire.ActionKindManufacture {
		t.Errorf("kind = %q, want manufacture", got.Kind)
	}
	if got.Item != "table" || got.Quantity != 3 {
		t.Errorf("got item=%q qty=%d, want item=table qty=3", got.Item, got.Quantity)
	}
}

func TestParseCommand_CaseInsensitive(t *testing.T) {
	got, err := ParseCommand("PAUSE")
	if err != nil {
		t.Fatalf("ParseCommand: %v", err)
	}
	if got.Kind != wire.ActionKindPause {
		t.Errorf("kind = %q, want pause", got.Kind)
	}
}

func TestParseCommand_EmptyAndUnknown(t *testing.T) {
	if _, err := ParseCommand(""); err == nil {
		t.Error("expected error for empty command")
	}
	if _, err := ParseCommand("dance"); err == nil {
		t.Error("expected error for unknown verb")
	}
}

func TestRejectReason_TypeAliasMatchesWire(t *testing.T) {
	// The overseer.RejectReason alias should be the same type as
	// wire.RejectReason so callers can errors.As either one.
	var rr *RejectReason = &wire.RejectReason{Msg: "foo"}
	if rr.Msg != "foo" {
		t.Errorf("alias did not preserve Msg field")
	}
}
