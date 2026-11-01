package dad

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/utilities"
)

// NewCreateDadResponseEndpoint adds a !dad response scoped to the logged-in
// channel.
func NewCreateDadResponseEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	svc := newDadService(nivek)

	return func(c echo.Context) error {
		user, err := utilities.GetUserFromContext(c, nivek.Logger())
		if err != nil {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
			})
		}

		var payload struct {
			Response string `json:"response"`
		}
		if errBind := c.Bind(&payload); errBind != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "invalid request body",
			})
		}
		if payload.Response == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "response required",
			})
		}

		if user.TwitchLogin == nil {
			nivek.Logger().Errorf("[Dad] user %d has no twitch_login", user.Id)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		if _, errAdd := svc.Add(*user.TwitchLogin, payload.Response); errAdd != nil {
			nivek.Logger().Errorf("[Dad] add response for %s: %v", *user.TwitchLogin, errAdd)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		return c.JSON(http.StatusOK, map[string]string{})
	}
}
