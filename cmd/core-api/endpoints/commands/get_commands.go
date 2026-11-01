package commands

import (
	"net/http"

	"github.com/labstack/echo/v4"
	commandSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

// newCommandsService builds the commands service. It is a var so tests can
// swap in a fake.
var newCommandsService = commandSvc.NewService

func NewGetCommandsEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	commandsService := newCommandsService(nivek)

	return func(c echo.Context) error {
		cmds, err := commandsService.GetGlobalEnabledCommands()
		if err != nil {
			nivek.Logger().Errorf("[Commands] fetch global enabled commands: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		return c.JSON(http.StatusOK, map[string]any{"commands": cmds})
	}
}
