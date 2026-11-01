package autoshout

import (
	"testing"

	"github.com/labstack/echo/v4"
	autoShoutSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/autoshout"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

// failingAutoShout fails every call, or only UpdateAutoShoutChatter when
// failUpdateOnly is set (to reach the update endpoint's second 500).
type failingAutoShout struct {
	autoShoutSvc.NivekAutoShoutService
	failUpdateOnly bool
}

func (failingAutoShout) GetAutoShoutChatters(int) ([]autoShoutSvc.ShoutChatter, error) {
	return nil, errForced
}
func (f failingAutoShout) GetAutoShoutChatter(_ int, name string) (*autoShoutSvc.ShoutChatter, error) {
	if f.failUpdateOnly {
		return &autoShoutSvc.ShoutChatter{ChatterName: name}, nil
	}
	return nil, errForced
}
func (failingAutoShout) CreateAutoShoutChatter(int, string) (int, error)         { return 0, errForced }
func (failingAutoShout) UpdateAutoShoutChatter(*autoShoutSvc.ShoutChatter) error { return errForced }
func (failingAutoShout) DeleteAutoShoutChatter(int, int) error                   { return errForced }

func TestServiceFailureIsGeneric500(t *testing.T) {
	orig := newAutoShoutService
	t.Cleanup(func() { newAutoShoutService = orig })

	cases := []struct {
		name string
		fake failingAutoShout
		ctor func(nivek.NivekService) echo.HandlerFunc
		body string
		id   string
	}{
		{"get", failingAutoShout{}, NewGetAutoShoutChattersEndpoint, "", ""},
		{"create", failingAutoShout{}, NewCreateAutoShoutChatterEndpoint, `{"chattername":"someone"}`, ""},
		{"update fetch", failingAutoShout{}, NewUpdateAutoShoutChatterEndpoint, `{"chattername":"someone"}`, ""},
		{"update write", failingAutoShout{failUpdateOnly: true}, NewUpdateAutoShoutChatterEndpoint, `{"chattername":"someone"}`, ""},
		{"delete", failingAutoShout{}, NewDeleteAutoShoutChatterEndpoint, "", "7"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := tc.fake
			newAutoShoutService = func(nivek.NivekService) autoShoutSvc.NivekAutoShoutService { return fake }
			svc, hook := newFakeNivek()
			rec := run(t, tc.ctor(svc), tc.body, tc.id, linkedUser())
			assertGeneric500(t, rec, hook)
		})
	}
}
