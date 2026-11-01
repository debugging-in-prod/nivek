package weather

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/weather"
)

// reporter is the part of weather.ReportService this endpoint uses.
type reporter interface {
	GetReport(ip string) (*weather.NivekWeatherReport, error)
}

// newReporter builds the weather report service. It is a var so tests can
// swap in a fake.
var newReporter = func(svc nivek.NivekService) reporter {
	return weather.NewWeatherReportService(svc)
}

func NewGetWeatherEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	return func(c echo.Context) error {
		ip := getIP(c)

		weatherService := newReporter(nivek)
		report, err := weatherService.GetReport(ip)
		if err != nil {
			nivek.Logger().Errorf("[Weather] fetch report: %v", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}

		return c.JSON(http.StatusOK, report)
	}
}

// getIP gets IP off request. If a IP value exists in the JSON body, then use that value instead
func getIP(c echo.Context) string {
	ip := c.RealIP()

	var fetchedIP struct {
		Ip string `json:"ip"`
	}
	err := c.Bind(&fetchedIP)
	if err == nil {
		ip = fetchedIP.Ip
	}

	return ip
}
