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
	called   bool
}

func (f *fakeForecaster) Forecast(_ context.Context, lat, lon float64) (Forecast, error) {
	f.called = true
	f.lat, f.lon = lat, lon
	return f.forecast, f.err
}

// newTestIPInfo builds an IPInfoClient pointed at a stub IPInfo server that
// returns body.
func newTestIPInfo(t *testing.T, body string) *IPInfoClient {
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

	return &IPInfoClient{client: info}
}

// newTestReportService wires a ReportService to a real IPInfoClient talking
// to a stub server that returns body, and to the given forecaster.
func newTestReportService(t *testing.T, body string, fc forecastProvider) *ReportService {
	t.Helper()
	return &ReportService{locator: newTestIPInfo(t, body), forecaster: fc}
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

type fakeLocator struct {
	loc Location
	err error
}

func (f fakeLocator) Locate(string) (Location, error) { return f.loc, f.err }

func TestGetReportUsesLocator(t *testing.T) {
	fc := &fakeForecaster{forecast: Forecast{TempF: 50}}
	s := &ReportService{
		locator:    fakeLocator{loc: Location{City: "Shelbyville", Lat: 1.5, Lon: -2.5}},
		forecaster: fc,
	}

	report, err := s.GetReport("8.8.8.8")
	if err != nil {
		t.Fatalf("GetReport: %v", err)
	}
	if report.City != "Shelbyville" || report.Temp != "50°F" {
		t.Errorf("report = %+v, want Shelbyville/50°F", report)
	}
	if fc.lat != 1.5 || fc.lon != -2.5 {
		t.Errorf("forecaster got lat/lon %v/%v, want 1.5/-2.5", fc.lat, fc.lon)
	}
}

func TestGetReportLocatorErrorSkipsForecast(t *testing.T) {
	fc := &fakeForecaster{}
	s := &ReportService{locator: fakeLocator{err: errors.New("forced")}, forecaster: fc}

	report, err := s.GetReport("8.8.8.8")
	if err == nil {
		t.Fatal("GetReport: want error, got nil")
	}
	if report != nil {
		t.Errorf("report = %+v, want nil on error", report)
	}
	if fc.called {
		t.Error("forecaster called after locator error")
	}
}

// A malformed or missing "loc" used to forecast for 0,0. It is now an error
// and no forecast is requested.
func TestGetReportBadLocationIsError(t *testing.T) {
	for _, body := range []string{
		`{"city":"Springfield"}`,
		`{"city":"Springfield","loc":"40.5"}`,
		`{"city":"Springfield","loc":"abc,-74.25"}`,
		`{"city":"Springfield","loc":"40.5,xyz"}`,
	} {
		fc := &fakeForecaster{}
		s := newTestReportService(t, body, fc)

		report, err := s.GetReport("8.8.8.8")
		if err == nil {
			t.Errorf("body %s: want error, got report %+v", body, report)
		}
		if fc.called {
			t.Errorf("body %s: forecaster called for unparseable location", body)
		}
	}
}

func TestIPInfoClientLocate(t *testing.T) {
	c := newTestIPInfo(t, `{"ip":"8.8.8.8","city":"Springfield","loc":" 40.5 , -74.25 "}`)

	loc, err := c.Locate("8.8.8.8")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}
	want := Location{City: "Springfield", Lat: 40.5, Lon: -74.25}
	if loc != want {
		t.Errorf("Locate = %+v, want %+v", loc, want)
	}
}
