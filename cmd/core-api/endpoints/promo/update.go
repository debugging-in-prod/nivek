package promo

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	promoSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/promo"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/utilities"
)

// NewUpdatePromoEndpoint rewrites one of the logged-in channel's own promos:
// its message, interval, enabled flag, and (optionally) its min_chat_lines
// chat-activity threshold. Scoped to the channel in the service.
func NewUpdatePromoEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := utilities.GetUserFromContext(c, nivek.Logger())
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "internal server error",
			})
		}

		channel, errCh := channelLogin(user)
		if errCh != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": errCh.Error()})
		}

		id, errId := strconv.Atoi(c.Param("id"))
		if errId != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id parameter"})
		}

		var payload struct {
			Message         string `json:"message"`
			IntervalSeconds int    `json:"interval_seconds"`
			Enabled         bool   `json:"enabled"`
			// Optional: omitted leaves the promo's chat-line threshold unchanged.
			MinChatLines *int `json:"min_chat_lines"`
		}
		if errBind := c.Bind(&payload); errBind != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		}
		msg, errMsg := promoSvc.ValidateMessage(payload.Message)
		if errMsg != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": errMsg.Error()})
		}
		payload.Message = msg
		if payload.IntervalSeconds <= 0 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "interval_seconds required"})
		}

		svc := promoSvc.NewService(nivek)
		if errUpd := svc.Update(channel, id, payload.Message, payload.IntervalSeconds, payload.Enabled, payload.MinChatLines); errUpd != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("error updating promo for channel [%s]: %s", channel, errUpd.Error()),
			})
		}

		return c.JSON(http.StatusOK, map[string]string{})
	}
}
