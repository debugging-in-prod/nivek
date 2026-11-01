package autoshout

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/utilities"
)

func NewGetAutoShoutChattersEndpoint(nivekSvc nivek.NivekService) echo.HandlerFunc {
	autoShoutService := newAutoShoutService(nivekSvc)

	return func(c echo.Context) error {

		user, err := utilities.GetUserFromContext(c, nivekSvc.Logger())
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "internal server error",
			})
		}

		bid, errBid := broadcasterID(user)
		if errBid != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": errBid.Error()})
		}

		chatters, errChat := autoShoutService.GetAutoShoutChatters(bid)
		if errChat != nil {
			nivekSvc.Logger().Errorf("[AutoShout] list chatters for broadcaster %d: %v", bid, errChat)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		return c.JSON(http.StatusOK, chatters)
	}
}
