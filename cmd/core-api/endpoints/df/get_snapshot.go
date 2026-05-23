package df

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

// NewGetSnapshotEndpoint returns the most recently received MapSnapshot.
// No auth — the dashboard is public per the locked design (Vultr fans out
// to anonymous viewers). 200 with the snapshot JSON, or 404 if the store
// has nothing (shouldn't happen post-Phase-1 since init seeds the fixture).
func NewGetSnapshotEndpoint(_ nivek.NivekService) echo.HandlerFunc {
	return func(c echo.Context) error {
		snap := store.get()
		if snap == nil {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "no snapshot available",
			})
		}
		return c.JSON(http.StatusOK, snap)
	}
}
