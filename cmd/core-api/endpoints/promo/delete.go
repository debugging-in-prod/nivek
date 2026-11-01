package promo

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/utilities"
)

// NewDeletePromoEndpoint removes one of the logged-in channel's own promos.
func NewDeletePromoEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
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

		svc := newPromoService(nivek)
		if errDel := svc.Remove(channel, id); errDel != nil {
			nivek.Logger().Errorf("[Promo] delete promo for %s: %v", channel, errDel)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		return c.JSON(http.StatusOK, map[string]string{})
	}
}
