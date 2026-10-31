package twitchbot

import (
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
)

func ptr(s string) *string { return &s }

func TestBuildCommandMaps(t *testing.T) {
	t.Parallel()

	cmds, rowErrs := buildCommandMaps([]commands.Commands{
		{Trigger: "!fish", Kind: "builtin", HandlerKey: ptr("fish"), Enabled: true},
		{Trigger: "!bread", Kind: "builtin", HandlerKey: ptr("bread"), Enabled: true},
		{Trigger: "!off", Kind: "builtin", HandlerKey: ptr("fish"), Enabled: false},
		{Trigger: "!custom", Kind: "custom", ResponseTmpl: ptr("hi"), Enabled: true},
	})
	if len(rowErrs) != 0 {
		t.Fatalf("unexpected row errors: %v", rowErrs)
	}

	if len(cmds) != 2 {
		t.Fatalf("expected 2 dispatchable commands, got %d", len(cmds))
	}
	if _, ok := cmds["!off"]; ok {
		t.Fatal("disabled rows must not reach the dispatch map")
	}
	if _, ok := cmds["!custom"]; ok {
		t.Fatal("custom rows have no compiled handler and must not reach the dispatch map")
	}
}

func TestBuildCommandMapsSkipsBadRows(t *testing.T) {
	t.Parallel()

	// Each invalid row is reported loudly and left out, but never takes the
	// valid rows (or a running bot) down with it. A mixed-case trigger can
	// never match (messages are lowercased before lookup); an unknown or null
	// handler_key has no compiled code to run.
	cmds, rowErrs := buildCommandMaps([]commands.Commands{
		{Trigger: "!Fish", Kind: "builtin", HandlerKey: ptr("fish"), Enabled: true},
		{Trigger: "!nope", Kind: "builtin", HandlerKey: ptr("no_such_handler"), Enabled: true},
		{Trigger: "!null", Kind: "builtin", Enabled: true},
		{Trigger: "!bread", Kind: "builtin", HandlerKey: ptr("bread"), Enabled: true},
	})
	if len(rowErrs) != 3 {
		t.Fatalf("expected 3 row errors, got %d: %v", len(rowErrs), rowErrs)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected only the valid row to load, got %d", len(cmds))
	}
	if _, ok := cmds["!bread"]; !ok {
		t.Fatal("the valid row must still load")
	}
}
