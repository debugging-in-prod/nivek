package autoshout

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/utilities"
)

func NewDeleteAutoShoutChatterEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	autoShoutService := newAutoShoutService(nivek)

	return func(c echo.Context) error {
		user, err := utilities.GetUserFromContext(c, nivek.Logger())
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "internal server error",
			})
		}

		id, errId := strconv.Atoi(c.Param("id"))
		if errId != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "invalid id parameter",
			})
		}

		bid, errBid := broadcasterID(user)
		if errBid != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": errBid.Error()})
		}

		if errChat := autoShoutService.DeleteAutoShoutChatter(bid, id); errChat != nil {
			nivek.Logger().Errorf("[AutoShout] delete chatter for broadcaster %d: %v", bid, errChat)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		return c.JSON(http.StatusOK, map[string]string{})
	}
}
