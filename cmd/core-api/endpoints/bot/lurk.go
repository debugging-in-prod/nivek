package bot

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/lurk"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

type lurkMessageRequest struct {
	Channel string `json:"channel"`
	Chatter string `json:"chatter"`
}

// NewPostLurkMessageEndpoint mirrors lurk.OnMessage — increments the lurker's
// count and returns the new value. The bot uses `count > 0` to decide whether
// to post a thank-you, so any failure path returns count=0 (matching the
// service's behavior of swallowing errors and returning 0).
func NewPostLurkMessageEndpoint(nivekSvc nivek.NivekService) echo.HandlerFunc {
	svc := lurk.NewService(nivekSvc)
	return func(c echo.Context) error {
		var req lurkMessageRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "decode body"})
		}
		if req.Channel == "" || req.Chatter == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "channel and chatter required"})
		}
		count := svc.OnMessage(req.Channel, req.Chatter)
		return c.JSON(http.StatusOK, map[string]int{"count": count})
	}
}
