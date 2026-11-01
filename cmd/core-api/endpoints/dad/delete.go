package dad

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/utilities"
)

// NewDeleteDadResponseEndpoint removes one of the logged-in channel's own !dad
// responses. Globals and other channels' rows are unaffected (enforced in the
// service).
func NewDeleteDadResponseEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	svc := newDadService(nivek)

	return func(c echo.Context) error {
		user, err := utilities.GetUserFromContext(c, nivek.Logger())
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
			})
		}

		id, errId := strconv.Atoi(c.Param("id"))
		if errId != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "invalid id parameter",
			})
		}

		if user.TwitchLogin == nil {
			nivek.Logger().Errorf("[Dad] user %d has no twitch_login", user.Id)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		if errDel := svc.Remove(*user.TwitchLogin, id); errDel != nil {
			nivek.Logger().Errorf("[Dad] delete response for %s: %v", *user.TwitchLogin, errDel)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		return c.JSON(http.StatusOK, map[string]string{})
	}
}
