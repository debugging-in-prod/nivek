package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestWindy points GetWindyWeatherReportUrl at handler and returns a
// client with a test key. Tests using it must not run in parallel: the URL
// is package state.
func newTestWindy(t *testing.T, handler http.HandlerFunc) *WindyClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	old := GetWindyWeatherReportUrl
	GetWindyWeatherReportUrl = srv.URL
	t.Cleanup(func() { GetWindyWeatherReportUrl = old })

	return NewWindyClient("test-key")
}

func TestNewWindyClientHasTimeout(t *testing.T) {
	c := NewWindyClient("k")
	if c.httpClient.Timeout != windyTimeout {
		t.Errorf("timeout = %v, want %v", c.httpClient.Timeout, windyTimeout)
	}
}

func TestForecastKelvinToFahrenheit(t *testing.T) {
	var got windyWeatherReportRequest
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

	f, err := c.Forecast(context.Background(), 40.5, -74.25)
	if err != nil {
		t.Fatalf("Forecast: %v", err)
	}
	if f.TempF != 72 {
		t.Errorf("TempF = %d, want 72", f.TempF)
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

func TestForecastEmptySeries(t *testing.T) {
	c := newTestWindy(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ts":[],"temp-surface":[]}`))
	})

	f, err := c.Forecast(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("Forecast: %v", err)
	}
	if f.TempF != 0 {
		t.Errorf("TempF = %d, want 0", f.TempF)
	}
}

func TestForecastDecodeError(t *testing.T) {
	c := newTestWindy(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})

	f, err := c.Forecast(context.Background(), 0, 0)
	if err == nil {
		t.Fatalf("Forecast: want error, got %+v", f)
	}
	if !strings.Contains(err.Error(), "unmarshalling") {
		t.Errorf("err = %v, want an unmarshalling error", err)
	}
	if f != (Forecast{}) {
		t.Errorf("forecast = %+v, want zero value on error", f)
	}
}

func TestForecastHonoursContext(t *testing.T) {
	c := newTestWindy(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"temp-surface":[273.15]}`))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.Forecast(ctx, 0, 0); err == nil {
		t.Fatal("Forecast with cancelled context: want error, got nil")
	}
}

func TestKelvinAvgToF(t *testing.T) {
	cases := []struct {
		name   string
		values []float64
		want   int
	}{
		{"freezing", []float64{273.15}, 32},
		{"boiling", []float64{373.15}, 212},
		{"below zero F", []float64{255.37}, 0},
		{"rounds to nearest", []float64{273.65}, 33},
		{"nil", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := kelvinAvgToF(tc.values); got != tc.want {
				t.Errorf("kelvinAvgToF(%v) = %d, want %d", tc.values, got, tc.want)
			}
		})
	}
}
