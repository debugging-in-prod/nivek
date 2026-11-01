package stalk

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/utilities"
)

// NewClearStalkEndpoint removes the logged-in channel's !stalk target.
func NewClearStalkEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	svc := newStalkService(nivek)

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

		found, errClear := svc.Clear(channel)
		if errClear != nil {
			nivek.Logger().Errorf("[Stalk] clear stalk target for %s: %v", channel, errClear)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		return c.JSON(http.StatusOK, map[string]any{"found": found})
	}
}
