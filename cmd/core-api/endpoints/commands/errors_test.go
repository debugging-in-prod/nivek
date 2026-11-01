package commands

import (
	"testing"

	commandSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

type failingCommands struct{ commandSvc.CommandsService }

func (failingCommands) GetGlobalEnabledCommands() ([]commandSvc.Commands, error) {
	return nil, errForced
}

func TestServiceFailureIsGeneric500(t *testing.T) {
	orig := newCommandsService
	newCommandsService = func(nivek.NivekService) commandSvc.CommandsService { return failingCommands{} }
	t.Cleanup(func() { newCommandsService = orig })

	svc, hook := newFakeNivek()
	rec := run(t, NewGetCommandsEndpoint(svc), "", "", nil)
	assertGeneric500(t, rec, hook)
}
