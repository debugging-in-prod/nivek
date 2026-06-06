package bot

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/bread"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

type breadIncrementRequest struct {
	Channel string `json:"channel"`
	Chatter string `json:"chatter"`
}

// NewPostBreadIncrementEndpoint mirrors bread.IncrementCount. The bot calls
// this on every `!bread` message and gets back the chatter's new per-day count.
func NewPostBreadIncrementEndpoint(nivekSvc nivek.NivekService) echo.HandlerFunc {
	svc := bread.NewService(nivekSvc)
	return func(c echo.Context) error {
		var req breadIncrementRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "decode body"})
		}
		if req.Channel == "" || req.Chatter == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "channel and chatter required"})
		}
		count, err := svc.IncrementCount(req.Channel, req.Chatter)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "increment failed"})
		}
		return c.JSON(http.StatusOK, map[string]int{"count": count})
	}
}

// NewGetBreadTotalEndpoint returns the channel-wide bread total for the last
// 24h, mirroring bread.GetTotalBreadForChannel. Channel comes in as a query
// param since this is a read.
func NewGetBreadTotalEndpoint(nivekSvc nivek.NivekService) echo.HandlerFunc {
	svc := bread.NewService(nivekSvc)
	return func(c echo.Context) error {
		channel := c.QueryParam("channel")
		if channel == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "channel query param required"})
		}
		total, err := svc.GetTotalBreadForChannel(channel)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "fetch total failed"})
		}
		return c.JSON(http.StatusOK, map[string]int{"total": total})
	}
}
