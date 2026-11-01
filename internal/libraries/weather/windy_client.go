package weather

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

type windyResponseBody struct {
	Ts                  []int                  `json:"ts"`
	Units               windyResponseBodyUnits `json:"units"`
	TempSurface         []float64              `json:"temp-surface"`
	Past3hPrecipSurface []float64              `json:"past3hprecip-surface"`
	WindUSurface        []float64              `json:"wind_u-surface"`
	WindVSurface        []float64              `json:"wind_v-surface"`
	PressureSurface     []float64              `json:"pressure-surface"`
	LCloudsSurface      []float64              `json:"lclouds-surface"`
	MCloudsSurface      []float64              `json:"mclouds-surface"`
	HCloudsSurface      []float64              `json:"hclouds-surface"`
}

type windyResponseBodyUnits struct {
	TempSurface         string `json:"temp-surface"`
	Past3hPrecipSurface string `json:"past3hprecip-surface"`
	WindUSurface        string `json:"wind_u-surface"`
	WindVSurface        string `json:"wind_v-surface"`
	PressureSurface     string `json:"pressure-surface"`
	LCloudsSurface      string `json:"lclouds-surface"`
	MCloudsSurface      string `json:"mclouds-surface"`
	HCloudsSurface      string `json:"hclouds-surface"`
}

// windyWeatherReportRequest based on docs: https://api.windy.com/point-forecast/docs
type windyWeatherReportRequest struct {
	Lat        float64  `json:"lat"`
	Lon        float64  `json:"lon"`
	Model      string   `json:"model"`
	Parameters []string `json:"parameters"`
	Levels     []string `json:"levels"`
	Key        string   `json:"key"` // Api Key
}

// GetWindyWeatherReportUrl is a var (not a const) so tests can point it at an
// httptest server.
var GetWindyWeatherReportUrl = "https://api.windy.com/api/point-forecast/v2"

// windyTimeout bounds every Windy request so a slow upstream can't hang the
// weather endpoint.
const windyTimeout = 10 * time.Second

// WindyClient is the only code that knows the Windy point-forecast API. It
// implements forecastProvider.
type WindyClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewWindyClient builds a Windy client with the API key injected and a
// 10-second HTTP timeout.
func NewWindyClient(apiKey string) *WindyClient {
	return &WindyClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: windyTimeout},
	}
}

// Forecast fetches the point forecast for lat/lon and returns the average
// surface temperature in °F.
func (c *WindyClient) Forecast(ctx context.Context, lat, lon float64) (Forecast, error) {
	data, err := c.fetchData(ctx, c.buildRequestBody(lat, lon))
	if err != nil {
		return Forecast{}, err
	}

	return Forecast{TempF: kelvinAvgToF(data.TempSurface)}, nil
}

// buildRequestBody - handles building request body for weather api request
func (c *WindyClient) buildRequestBody(lat, lon float64) windyWeatherReportRequest {
	return windyWeatherReportRequest{
		Lat:   lat,
		Lon:   lon,
		Model: "gfs",
		Parameters: []string{
			"temp",
			"precip",
			"wind",
			"pressure",
			"lclouds",
			"mclouds",
			"hclouds",
		},
		Levels: []string{"surface"},
		Key:    c.apiKey,
	}
}

func (c *WindyClient) fetchData(ctx context.Context, body windyWeatherReportRequest) (*windyResponseBody, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("error marshalling weather report request in Forecast for (%s): %s",
			string(jsonBody),
			err.Error(),
		)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		GetWindyWeatherReportUrl,
		bytes.NewBuffer(jsonBody),
	)
	if err != nil {
		return nil, fmt.Errorf("error building windy request: %s", err.Error())
	}
	req.Header.Set("Content-Type", "application/json")

	windyResponse, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error fetching report from windy: %s", err.Error())
	}
	defer windyResponse.Body.Close()

	rawBody, err := io.ReadAll(windyResponse.Body)
	if err != nil {
		return nil, fmt.Errorf(
			"error reading report from windy: %s",
			err.Error(),
		)
	}

	var response windyResponseBody
	if err := json.Unmarshal(rawBody, &response); err != nil {
		return nil, fmt.Errorf(
			"error unmarshalling report from windy: %s",
			err.Error(),
		)
	}

	return &response, nil
}

// kelvinAvgToF - Windy returns "surface temp" as a []float64 where each value
// is in Kelvin. This averages the values, converts to °F and rounds to the
// nearest integer. An empty series yields 0.
func kelvinAvgToF(values []float64) int {
	if len(values) == 0 {
		return 0
	}

	var sum float64
	for _, v := range values {
		sum += v
	}

	avgKelvin := sum / float64(len(values))
	avgFahrenheit := (avgKelvin-273.15)*9/5 + 32

	return int(math.Round(avgFahrenheit))
}
