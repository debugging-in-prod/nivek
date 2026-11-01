package weather

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/config"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

// fakeNivek satisfies nivek.NivekService; only CommonConfig is used by the
// Windy client. Any other method panics via the nil embedded interface.
type fakeNivek struct {
	nivek.NivekService
	cfg config.Config
}

func (f fakeNivek) CommonConfig() config.Config { return f.cfg }

// newTestWindy points GetWindyWeatherReportUrl at handler and returns a
// client using a fake config. Tests using it must not run in parallel:
// the URL is package state.
func newTestWindy(t *testing.T, handler http.HandlerFunc) *WindyClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	old := GetWindyWeatherReportUrl
	GetWindyWeatherReportUrl = srv.URL
	t.Cleanup(func() { GetWindyWeatherReportUrl = old })

	var cfg config.Config
	cfg.Windy.Token = "test-key"
	return NewWindyClient(fakeNivek{cfg: cfg})
}

func TestGetTempKelvinToFahrenheit(t *testing.T) {
	var got WindyWeatherReportRequest
	c := newTestWindy(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		// Average 295.15K = 22°C = 71.6°F, rounds to 72.
		_, _ = w.Write([]byte(`{"ts":[1,2],"temp-surface":[294.15,296.15]}`))
	})

	temp, err := c.GetTemp(40.5, -74.25)
	if err != nil {
		t.Fatalf("GetTemp: %v", err)
	}
	if temp != "72°F" {
		t.Errorf("temp = %q, want 72°F", temp)
	}
	if got.Lat != 40.5 || got.Lon != -74.25 {
		t.Errorf("lat/lon = %v/%v, want 40.5/-74.25", got.Lat, got.Lon)
	}
	if got.Key != "test-key" {
		t.Errorf("key = %q, want test-key", got.Key)
	}
	if got.Model != "gfs" {
		t.Errorf("model = %q, want gfs", got.Model)
	}
}

func TestGetTempEmptySeries(t *testing.T) {
	c := newTestWindy(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ts":[],"temp-surface":[]}`))
	})

	temp, err := c.GetTemp(0, 0)
	if err != nil {
		t.Fatalf("GetTemp: %v", err)
	}
	if temp != "0°F" {
		t.Errorf("temp = %q, want 0°F", temp)
	}
}

func TestGetTempDecodeError(t *testing.T) {
	c := newTestWindy(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})

	temp, err := c.GetTemp(0, 0)
	if err == nil {
		t.Fatalf("GetTemp: want error, got temp %q", temp)
	}
	if !strings.Contains(err.Error(), "unmarshalling") {
		t.Errorf("err = %v, want an unmarshalling error", err)
	}
	if temp != "" {
		t.Errorf("temp = %q, want empty on error", temp)
	}
}

func TestFormatTemp(t *testing.T) {
	cases := []struct {
		name   string
		values []float64
		want   string
	}{
		{"freezing", []float64{273.15}, "32°F"},
		{"boiling", []float64{373.15}, "212°F"},
		{"below zero F", []float64{255.37}, "0°F"},
		{"rounds to nearest", []float64{273.65}, "33°F"},
		{"nil", nil, "0°F"},
	}
	var c WindyClient
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.formatTemp(tc.values); got != tc.want {
				t.Errorf("formatTemp(%v) = %q, want %q", tc.values, got, tc.want)
			}
		})
	}
}
