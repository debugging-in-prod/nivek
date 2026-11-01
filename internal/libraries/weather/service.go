package weather

import (
	"context"
	"fmt"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

type NivekWeatherReport struct {
	City string `json:"city"`
	Temp string `json:"temp"`
}

// forecastProvider is what ReportService needs from a weather API.
// WindyClient implements it; tests pass a fake.
type forecastProvider interface {
	Forecast(ctx context.Context, lat, lon float64) (Forecast, error)
}

// locator is what ReportService needs from a geolocation API.
// IPInfoClient implements it; tests pass a fake.
type locator interface {
	Locate(ip string) (Location, error)
}

type ReportService struct {
	nivek.NivekService
	locator    locator
	forecaster forecastProvider
}

func NewWeatherReportService(nivek nivek.NivekService) *ReportService {
	return &ReportService{
		NivekService: nivek,
		locator:      NewIPInfoClient(nivek.CommonConfig().IPInfo.Token),
		forecaster:   NewWindyClient(nivek.CommonConfig().Windy.Token),
	}
}

func (s *ReportService) GetReport(ip string) (*NivekWeatherReport, error) {
	loc, err := s.locator.Locate(ip)
	if err != nil {
		return nil, fmt.Errorf(
			"error fetching IP info during GetTemp for ip (%s): %w", ip, err,
		)
	}

	// GetReport has no caller context yet; the client's HTTP timeout bounds
	// the request.
	forecast, err := s.forecaster.Forecast(context.Background(), loc.Lat, loc.Lon)
	if err != nil {
		return nil, err
	}

	nivekReport := NivekWeatherReport{
		City: loc.City,
		Temp: fmt.Sprintf("%d°F", forecast.TempF),
	}

	return &nivekReport, nil
}
