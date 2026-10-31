// Package channelcmd serves the dashboard editors for a channel's own custom
// commands and its per-channel overrides of the global built-ins. Every route
// runs behind the session JWT middleware, and the channel is always taken from
// the session user's linked Twitch id — never from the request — so a user can
// only read or change the channel they own.
//
// These endpoints only write rows; they do not touch the bot. The bot picks a
// channel's custom commands up on that channel's next go-live
// (twitchbot.loadCustomCommands on stream.online).
package channelcmd

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	commandSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	userlib "github.com/tim-the-toolman-taylor/nivek/internal/libraries/user"
)

// sessionChannelID returns the logged-in user's Twitch id, or an HTTP error
// when there is no session user or no linked Twitch account.
func sessionChannelID(c echo.Context) (string, error) {
	u, ok := c.Get("user").(*userlib.User)
	if !ok || u == nil {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "missing authenticated user")
	}
	if u.TwitchID == nil || strings.TrimSpace(*u.TwitchID) == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "no linked twitch account")
	}
	return *u.TwitchID, nil
}

func idParam(c echo.Context) (int, error) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "invalid id parameter")
	}
	return id, nil
}

// statusFor maps service errors to HTTP status codes. Validation, conflict and
// limit messages are safe to show the user; anything else is reported as a
// generic failure (and logged by the caller).
func statusFor(err error) (int, string) {
	switch {
	case errors.Is(err, commandSvc.ErrValidation):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, commandSvc.ErrNotFound):
		return http.StatusNotFound, "command not found"
	case errors.Is(err, commandSvc.ErrConflict):
		return http.StatusConflict, err.Error()
	case errors.Is(err, commandSvc.ErrLimit):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

func respondErr(svc nivek.NivekService, c echo.Context, op, channelID string, err error) error {
	status, msg := statusFor(err)
	if status == http.StatusInternalServerError {
		svc.Logger().Errorf("channel commands: %s for %s failed: %s", op, channelID, err.Error())
	}
	return c.JSON(status, map[string]string{"error": msg})
}

// NewListCustomEndpoint: GET the channel's custom commands (enabled or not).
func NewListCustomEndpoint(svc nivek.NivekService) echo.HandlerFunc {
	return listCustom(svc, commandSvc.NewChannelEditorService(svc))
}

func listCustom(svc nivek.NivekService, editor commandSvc.ChannelEditorService) echo.HandlerFunc {
	return func(c echo.Context) error {
		channelID, err := sessionChannelID(c)
		if err != nil {
			return err
		}
		cmds, err := editor.ListCustom(channelID)
		if err != nil {
			return respondErr(svc, c, "list", channelID, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"commands": cmds})
	}
}

// NewCreateCustomEndpoint: POST a new custom command for the channel.
func NewCreateCustomEndpoint(svc nivek.NivekService) echo.HandlerFunc {
	return createCustom(svc, commandSvc.NewChannelEditorService(svc))
}

func createCustom(svc nivek.NivekService, editor commandSvc.ChannelEditorService) echo.HandlerFunc {
	return func(c echo.Context) error {
		channelID, err := sessionChannelID(c)
		if err != nil {
			return err
		}
		var in commandSvc.CustomCommandInput
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		created, err := editor.CreateCustom(channelID, in)
		if err != nil {
			return respondErr(svc, c, "create", channelID, err)
		}
		return c.JSON(http.StatusOK, created)
	}
}

// NewUpdateCustomEndpoint: PUT a full replacement of one of the channel's own
// custom commands.
func NewUpdateCustomEndpoint(svc nivek.NivekService) echo.HandlerFunc {
	return updateCustom(svc, commandSvc.NewChannelEditorService(svc))
}

func updateCustom(svc nivek.NivekService, editor commandSvc.ChannelEditorService) echo.HandlerFunc {
	return func(c echo.Context) error {
		channelID, err := sessionChannelID(c)
		if err != nil {
			return err
		}
		id, err := idParam(c)
		if err != nil {
			return err
		}
		var in commandSvc.CustomCommandInput
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		updated, err := editor.UpdateCustom(channelID, id, in)
		if err != nil {
			return respondErr(svc, c, "update", channelID, err)
		}
		return c.JSON(http.StatusOK, updated)
	}
}

// NewDeleteCustomEndpoint: DELETE one of the channel's own custom commands.
func NewDeleteCustomEndpoint(svc nivek.NivekService) echo.HandlerFunc {
	return deleteCustom(svc, commandSvc.NewChannelEditorService(svc))
}

func deleteCustom(svc nivek.NivekService, editor commandSvc.ChannelEditorService) echo.HandlerFunc {
	return func(c echo.Context) error {
		channelID, err := sessionChannelID(c)
		if err != nil {
			return err
		}
		id, err := idParam(c)
		if err != nil {
			return err
		}
		if err := editor.DeleteCustom(channelID, id); err != nil {
			return respondErr(svc, c, "delete", channelID, err)
		}
		return c.JSON(http.StatusOK, map[string]string{})
	}
}

// NewListSettingsEndpoint: GET every global command merged with the channel's
// override.
func NewListSettingsEndpoint(svc nivek.NivekService) echo.HandlerFunc {
	return listSettings(svc, commandSvc.NewChannelEditorService(svc))
}

func listSettings(svc nivek.NivekService, editor commandSvc.ChannelEditorService) echo.HandlerFunc {
	return func(c echo.Context) error {
		channelID, err := sessionChannelID(c)
		if err != nil {
			return err
		}
		settings, err := editor.ListSettings(channelID)
		if err != nil {
			return respondErr(svc, c, "list settings", channelID, err)
		}
		return c.JSON(http.StatusOK, map[string]any{"settings": settings})
	}
}

// NewPutSettingEndpoint: PUT the channel's override for one global command
// (:id is the command id). Null fields inherit the command's default.
func NewPutSettingEndpoint(svc nivek.NivekService) echo.HandlerFunc {
	return putSetting(svc, commandSvc.NewChannelEditorService(svc))
}

func putSetting(svc nivek.NivekService, editor commandSvc.ChannelEditorService) echo.HandlerFunc {
	return func(c echo.Context) error {
		channelID, err := sessionChannelID(c)
		if err != nil {
			return err
		}
		id, err := idParam(c)
		if err != nil {
			return err
		}
		var in commandSvc.CommandSettingInput
		if err := c.Bind(&in); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		if err := editor.PutSetting(channelID, id, in); err != nil {
			return respondErr(svc, c, "put setting", channelID, err)
		}
		return c.JSON(http.StatusOK, map[string]string{})
	}
}

// NewDeleteSettingEndpoint: DELETE the channel's override for one global
// command, reverting it to the defaults.
func NewDeleteSettingEndpoint(svc nivek.NivekService) echo.HandlerFunc {
	return deleteSetting(svc, commandSvc.NewChannelEditorService(svc))
}

func deleteSetting(svc nivek.NivekService, editor commandSvc.ChannelEditorService) echo.HandlerFunc {
	return func(c echo.Context) error {
		channelID, err := sessionChannelID(c)
		if err != nil {
			return err
		}
		id, err := idParam(c)
		if err != nil {
			return err
		}
		if err := editor.DeleteSetting(channelID, id); err != nil {
			return respondErr(svc, c, "reset setting", channelID, err)
		}
		return c.JSON(http.StatusOK, map[string]string{})
	}
}
