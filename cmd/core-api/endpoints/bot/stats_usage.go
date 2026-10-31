package bot

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/stats"
)

// NewPostCommandUsageEndpoint ingests the bot's batched per-channel command
// usage counters (issue #177) into nivek.command_usage. botAuth-gated. Invalid
// rows are skipped; the bot drops a batch on any non-2xx, so the only cost of
// a failure is lost analytics, never chat behavior.
func NewPostCommandUsageEndpoint(nivekSvc nivek.NivekService) echo.HandlerFunc {
	store := stats.NewStore(nivekSvc.Postgres().GetDefaultConnection())
	return postCommandUsageHandler(store, time.Now, func(err error) {
		nivekSvc.Logger().Errorf("bot stats usage: %s", err.Error())
	})
}

func postCommandUsageHandler(store stats.Store, now func() time.Time, logErr func(error)) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req stats.UsageBatch
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "decode body"})
		}
		if len(req.Counts) > stats.MaxUsageBatch {
			return c.JSON(http.StatusRequestEntityTooLarge, map[string]string{"error": "too many rows"})
		}
		n, err := stats.IngestUsage(c.Request().Context(), store, req.Counts, now())
		if err != nil {
			if logErr != nil {
				logErr(err)
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "store failed"})
		}
		return c.JSON(http.StatusOK, map[string]int{"accepted": n})
	}
}
