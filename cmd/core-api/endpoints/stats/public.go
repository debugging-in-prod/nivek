// Package stats serves the public usage-analytics endpoint (issue #177).
package stats

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	statsLib "github.com/tim-the-toolman-taylor/nivek/internal/libraries/stats"
)

// publicStatsGetter is the slice of *statsLib.PublicService the handler uses.
type publicStatsGetter interface {
	Get(ctx context.Context) (statsLib.PublicStats, error)
}

// NewGetPublicStatsEndpoint serves GET /api/stats/public. No auth: the
// payload is aggregate counts plus public Twitch logins/display names only.
// Results are cached in memory for statsLib.CacheTTL (one PublicService per
// process), so request volume never reaches the database.
func NewGetPublicStatsEndpoint(nivekSvc nivek.NivekService) echo.HandlerFunc {
	svc := statsLib.NewPublicService(statsLib.NewStore(nivekSvc.Postgres().GetDefaultConnection()))
	return getPublicStatsHandler(svc, func(err error) {
		nivekSvc.Logger().Errorf("public stats: %s", err.Error())
	})
}

func getPublicStatsHandler(svc publicStatsGetter, logErr func(error)) echo.HandlerFunc {
	return func(c echo.Context) error {
		out, err := svc.Get(c.Request().Context())
		if err != nil {
			if logErr != nil {
				logErr(err)
			}
			// Generic message: never echo database errors on a public route.
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "stats temporarily unavailable"})
		}
		c.Response().Header().Set("Cache-Control", "public, max-age=60")
		return c.JSON(http.StatusOK, out)
	}
}
