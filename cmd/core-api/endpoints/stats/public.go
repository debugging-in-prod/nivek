// Package stats serves the public usage-analytics endpoint (issue #177).
package stats

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/coreconfig"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	statsLib "github.com/tim-the-toolman-taylor/nivek/internal/libraries/stats"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/twitcheventsub"
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
	logger := nivekSvc.Logger()
	cfg, _ := nivekSvc.CustomConfig().(coreconfig.CoreAPIConfig)
	svc := statsLib.NewPublicService(
		statsLib.NewStore(nivekSvc.Postgres().GetDefaultConnection()),
		newDisplayNameResolver(cfg, logger.Infof),
		func(err error) { logger.Warnf("[Stats] top_fishers display names (showing logins): %v", err) },
	)
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

// newDisplayNameResolver reuses core-api's existing Twitch app credentials for
// Helix Get Users. With no usable credentials it returns nil, which turns
// display-name resolution off (top_fishers shows logins); it never blocks
// startup and adds no env vars.
func newDisplayNameResolver(cfg coreconfig.CoreAPIConfig, logf func(string, ...any)) statsLib.DisplayNameResolver {
	client, err := twitcheventsub.NewClient(twitcheventsub.Config{
		ClientID:       cfg.TwitchClientID,
		ClientSecret:   cfg.TwitchClientSecret,
		EventSubSecret: cfg.TwitchEventSubSecret,
		CallbackURL:    cfg.TwitchEventSubCallbackURL,
	})
	if err != nil {
		logf("[Stats] top_fishers display names disabled, showing logins (%v)", err)
		return nil
	}
	return client
}
