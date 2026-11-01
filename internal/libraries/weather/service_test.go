package weather

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ipinfo/go/v2/ipinfo"
)

type fakeForecaster struct {
	forecast Forecast
	err      error
	lat, lon float64
}

func (f *fakeForecaster) Forecast(_ context.Context, lat, lon float64) (Forecast, error) {
	f.lat, f.lon = lat, lon
	return f.forecast, f.err
}

// newTestReportService wires a ReportService to a stub IPInfo server that
// returns body, and to the given forecaster.
func newTestReportService(t *testing.T, body string, fc forecastProvider) *ReportService {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	info := ipinfo.NewClient(srv.Client(), nil, "")
	base, err := url.Parse(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	info.BaseURL = base

	return &ReportService{infoClient: info, forecaster: fc}
}

func TestGetReportFormatsForecast(t *testing.T) {
	fc := &fakeForecaster{forecast: Forecast{TempF: 72}}
	s := newTestReportService(t, `{"ip":"8.8.8.8","city":"Springfield","loc":"40.5,-74.25"}`, fc)

	report, err := s.GetReport("8.8.8.8")
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if report.City != "Springfield" {
		t.Errorf("City = %q, want Springfield", report.City)
	}
	if report.Temp != "72°F" {
		t.Errorf("Temp = %q, want 72°F", report.Temp)
	}
	if fc.lat != 40.5 || fc.lon != -74.25 {
		t.Errorf("forecaster got lat/lon %v/%v, want 40.5/-74.25", fc.lat, fc.lon)
	}
}

func TestGetReportZeroAndNegativeTemps(t *testing.T) {
	for _, tc := range []struct {
		tempF int
		want  string
	}{{0, "0°F"}, {-5, "-5°F"}} {
		fc := &fakeForecaster{forecast: Forecast{TempF: tc.tempF}}
		s := newTestReportService(t, `{"city":"Nome","loc":"64.5,-165.4"}`, fc)
		report, err := s.GetReport("8.8.8.8")
		if err != nil {
			t.Fatalf("GetReport: %v", err)
		}
		if report.Temp != tc.want {
			t.Errorf("Temp = %q, want %q", report.Temp, tc.want)
		}
	}
}

func TestGetReportForecastError(t *testing.T) {
	errForced := errors.New("forced")
	fc := &fakeForecaster{err: errForced}
	s := newTestReportService(t, `{"city":"Springfield","loc":"40.5,-74.25"}`, fc)

	report, err := s.GetReport("8.8.8.8")
	if !errors.Is(err, errForced) {
		t.Fatalf("err = %v, want %v", err, errForced)
	}
	if report != nil {
		t.Errorf("report = %+v, want nil on error", report)
	}
}
