package weather

import (
	"testing"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/weather"
)

type failingReporter struct{}

func (failingReporter) GetReport(string) (*weather.NivekWeatherReport, error) { return nil, errForced }

func TestServiceFailureIsGeneric500(t *testing.T) {
	orig := newReporter
	newReporter = func(nivek.NivekService) reporter { return failingReporter{} }
	t.Cleanup(func() { newReporter = orig })

	svc, hook := newFakeNivek()
	rec := run(t, NewGetWeatherEndpoint(svc), `{"ip":"203.0.113.9"}`, "", nil)
	assertGeneric500(t, rec, hook)
}
