package stalk

import (
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	stalkSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/stalk"
)

type failingStalk struct{ stalkSvc.NivekStalkService }

func (failingStalk) Get(string) (stalkSvc.Stalk, bool, error) {
	return stalkSvc.Stalk{}, false, errForced
}
func (failingStalk) Set(string, string, string) error { return errForced }
func (failingStalk) Clear(string) (bool, error)       { return false, errForced }

func TestServiceFailureIsGeneric500(t *testing.T) {
	orig := newStalkService
	newStalkService = func(nivek.NivekService) stalkSvc.NivekStalkService { return failingStalk{} }
	t.Cleanup(func() { newStalkService = orig })

	cases := []struct {
		name string
		ctor func(nivek.NivekService) echo.HandlerFunc
		body string
	}{
		{"get", NewGetStalkEndpoint, ""},
		{"set", NewSetStalkEndpoint, `{"target":"somebody"}`},
		{"clear", NewClearStalkEndpoint, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, hook := newFakeNivek()
			rec := run(t, tc.ctor(svc), tc.body, "", linkedUser())
			assertGeneric500(t, rec, hook)
		})
	}
}
