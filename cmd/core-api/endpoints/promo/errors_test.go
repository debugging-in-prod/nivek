package promo

import (
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	promoSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/promo"
)

type failingPromo struct{ promoSvc.NivekPromoService }

func (failingPromo) ListForChannel(string) ([]promoSvc.Promo, error) { return nil, errForced }
func (failingPromo) Create(string, string, string, int) (promoSvc.Promo, error) {
	return promoSvc.Promo{}, errForced
}
func (failingPromo) Update(string, int, string, int, bool, *int) error { return errForced }
func (failingPromo) Remove(string, int) error                          { return errForced }

func TestServiceFailureIsGeneric500(t *testing.T) {
	orig := newPromoService
	newPromoService = func(nivek.NivekService) promoSvc.NivekPromoService { return failingPromo{} }
	t.Cleanup(func() { newPromoService = orig })

	cases := []struct {
		name string
		ctor func(nivek.NivekService) echo.HandlerFunc
		body string
		id   string
	}{
		{"get", NewGetPromosEndpoint, "", ""},
		{"create", NewCreatePromoEndpoint, `{"message":"hi","interval_seconds":600}`, ""},
		{"update", NewUpdatePromoEndpoint, `{"message":"hi","interval_seconds":600,"enabled":true}`, "7"},
		{"delete", NewDeletePromoEndpoint, "", "7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, hook := newFakeNivek()
			rec := run(t, tc.ctor(svc), tc.body, tc.id, linkedUser())
			assertGeneric500(t, rec, hook)
		})
	}
}
