package dad

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/utilities"
)

// NewGetDadResponsesEndpoint returns the logged-in channel's !dad pool: the
// shared globals plus the channel's own responses (with usage counts).
func NewGetDadResponsesEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := utilities.GetUserFromContext(c, nivek.Logger())
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
			})
		}

		if user.TwitchLogin == nil {
			nivek.Logger().Errorf("[Dad] user %d has no twitch_login", user.Id)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		svc := newDadService(nivek)
		responses, errList := svc.ListForChannel(*user.TwitchLogin)
		if errList != nil {
			nivek.Logger().Errorf("[Dad] list responses for %s: %v", *user.TwitchLogin, errList)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		return c.JSON(http.StatusOK, responses)
	}
}
