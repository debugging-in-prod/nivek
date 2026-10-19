package twitchbot

import (
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
)

func ptr(s string) *string { return &s }

func TestBuildCommandMaps(t *testing.T) {
	t.Parallel()

	cmds, err := buildCommandMaps([]commands.Commands{
		{Trigger: "!fish", Kind: "builtin", HandlerKey: ptr("fish"), Enabled: true},
		{Trigger: "!bread", Kind: "builtin", HandlerKey: ptr("bread"), Enabled: true},
		{Trigger: "!off", Kind: "builtin", HandlerKey: ptr("fish"), Enabled: false},
		{Trigger: "!custom", Kind: "custom", ResponseTmpl: ptr("hi"), Enabled: true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
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

func TestBuildCommandMapsRejectsBadRows(t *testing.T) {
	t.Parallel()

	// A trigger stored with upper case can never match: the message is
	// lowercased before lookup but the map is keyed on the column verbatim, so
	// it must fail loud at boot rather than ship a silently-dead command.
	if _, err := buildCommandMaps([]commands.Commands{
		{Trigger: "!Fish", Kind: "builtin", HandlerKey: ptr("fish"), Enabled: true},
	}); err == nil {
		t.Fatal("a mixed-case trigger should fail at boot")
	}

	if _, err := buildCommandMaps([]commands.Commands{
		{Trigger: "!nope", Kind: "builtin", HandlerKey: ptr("no_such_handler"), Enabled: true},
	}); err == nil {
		t.Fatal("an unknown handler_key should fail at boot")
	}

	if _, err := buildCommandMaps([]commands.Commands{
		{Trigger: "!nope", Kind: "builtin", Enabled: true},
	}); err == nil {
		t.Fatal("a null handler_key on a builtin should fail at boot")
	}
}
