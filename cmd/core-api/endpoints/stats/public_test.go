package stats

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	statsLib "github.com/tim-the-toolman-taylor/nivek/internal/libraries/stats"
)

type fakeGetter struct {
	out statsLib.PublicStats
	err error
}

func (f fakeGetter) Get(context.Context) (statsLib.PublicStats, error) { return f.out, f.err }

func serve(h echo.HandlerFunc) *httptest.ResponseRecorder {
	e := echo.New()
	rec := httptest.NewRecorder()
	_ = h(e.NewContext(httptest.NewRequest(http.MethodGet, "/api/stats/public", nil), rec))
	return rec
}

func TestGetPublicStatsContract(t *testing.T) {
	t.Parallel()
	gen := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	rec := serve(getPublicStatsHandler(fakeGetter{out: statsLib.PublicStats{
		ActiveChats:           3,
		ActiveChatsDefinition: statsLib.ActiveChatsDefinition,
		TopChannels:           []statsLib.ChannelUses{{DisplayName: "Foo", Uses: 12}},
		TopChannelsWindow:     statsLib.TopChannelsWindow,
		TopChannelsWindowDays: statsLib.TopChannelsWindowDays,
		TopFishers:            []statsLib.Fisher{{DisplayName: "bar", Score: 100, Catches: 4}},
		GeneratedAt:           gen,
	}}, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("Cache-Control = %q", cc)
	}
	// Pin the exact JSON contract the frontend consumes.
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{"active_chats", "active_chats_definition", "top_channels", "top_channels_window", "top_channels_window_days", "top_fishers", "generated_at"}
	if len(got) != len(wantKeys) {
		t.Errorf("keys = %v", got)
	}
	for _, k := range wantKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}
	// All-time window (issue #184); the legacy day count stays, as 0.
	if got["top_channels_window"] != "all_time" || got["top_channels_window_days"] != float64(0) {
		t.Errorf("window = %v / %v", got["top_channels_window"], got["top_channels_window_days"])
	}
	if got["generated_at"] != "2026-10-31T12:00:00Z" {
		t.Errorf("generated_at = %v", got["generated_at"])
	}
	ch := got["top_channels"].([]any)[0].(map[string]any)
	if len(ch) != 2 || ch["display_name"] != "Foo" || ch["uses"] != float64(12) {
		t.Errorf("top_channels[0] = %v", ch)
	}
	f := got["top_fishers"].([]any)[0].(map[string]any)
	if len(f) != 3 || f["display_name"] != "bar" || f["score"] != float64(100) || f["catches"] != float64(4) {
		t.Errorf("top_fishers[0] = %v", f)
	}
}

func TestGetPublicStatsErrorIsGeneric(t *testing.T) {
	t.Parallel()
	var logged error
	rec := serve(getPublicStatsHandler(fakeGetter{err: errors.New(`pq: relation "nivek.secret" does not exist`)},
		func(err error) { logged = err }))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "nivek.secret") {
		t.Errorf("db error leaked: %s", rec.Body.String())
	}
	if logged == nil {
		t.Error("error not logged")
	}
}
