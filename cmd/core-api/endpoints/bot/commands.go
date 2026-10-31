package bot

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	commandSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

// NewGetChannelCommands serves the enabled channel-scoped custom commands for a
// single broadcaster (by Twitch id in :bid). The bot calls this when a channel
// goes live — and at boot for channels already live — to load that channel's
// custom triggers. botAuth-gated: unlike the public /commands (global) fetch,
// this is bot-only plumbing, but the data isn't sensitive either way.
func NewGetChannelCommands(nivekSvc nivek.NivekService) echo.HandlerFunc {
	commandsService := commandSvc.NewService(nivekSvc)
	return func(c echo.Context) error {
		bid := c.Param("bid")
		if bid == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "missing broadcaster twitch id",
			})
		}

		cmds, err := commandsService.GetChannelEnabledCommands(bid)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("failed to fetch channel commands for %s: %s", bid, err.Error()),
			})
		}

		return c.JSON(http.StatusOK, map[string]any{
			"commands": cmds,
		})
	}
}

type commandUseRequest struct {
	CommandID       int    `json:"command_id"`
	ChannelTwitchID string `json:"channel_twitch_id"`
}

// NewPostCommandUse bumps a channel custom command's use counter and returns
// the new value. The bot calls it when a custom command whose template uses
// ${count} fires; the bot treats any failure as "no count" and still replies.
func NewPostCommandUse(nivekSvc nivek.NivekService) echo.HandlerFunc {
	commandsService := commandSvc.NewService(nivekSvc)
	return func(c echo.Context) error {
		var req commandUseRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "decode body"})
		}
		if req.CommandID <= 0 || req.ChannelTwitchID == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "command_id and channel_twitch_id required"})
		}

		count, found, err := commandsService.IncrementChannelCommandUse(req.CommandID, req.ChannelTwitchID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "increment failed"})
		}
		if !found {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "command not found"})
		}
		return c.JSON(http.StatusOK, map[string]int{"count": count})
	}
}
