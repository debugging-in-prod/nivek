package bot

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/fishing"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

type fishGoRequest struct {
	Channel string `json:"channel"`
	Chatter string `json:"chatter"`
}

// NewPostFishGoEndpoint mirrors fishing.GoFishing — runs the full fish roll +
// leaderboard logic server-side and returns the already-formatted chat string.
// Keeps the bot a thin chat client: it relays the response verbatim.
func NewPostFishGoEndpoint(nivekSvc nivek.NivekService) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req fishGoRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "decode body"})
		}
		if req.Channel == "" || req.Chatter == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "channel and chatter required"})
		}
		// Fishing service is constructed per-call because it binds the channel
		// into its state (see fishing.NewService signature).
		svc := fishing.NewService(nivekSvc, req.Channel)
		msg := svc.GoFishing(req.Chatter)
		return c.JSON(http.StatusOK, map[string]string{"message": msg})
	}
}
