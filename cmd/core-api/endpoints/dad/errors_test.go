package dad

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	dadSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/dad"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	userlib "github.com/tim-the-toolman-taylor/nivek/internal/libraries/user"
)

type failingDad struct{ dadSvc.NivekDadService }

func (failingDad) ListForChannel(string) ([]dadSvc.DadResponse, error) { return nil, errForced }
func (failingDad) Add(string, string) (dadSvc.DadResponse, error) {
	return dadSvc.DadResponse{}, errForced
}
func (failingDad) Remove(string, int) error { return errForced }

var dadCases = []struct {
	name string
	ctor func(nivek.NivekService) echo.HandlerFunc
	body string
	id   string
}{
	{"get", NewGetDadResponsesEndpoint, "", ""},
	{"create", NewCreateDadResponseEndpoint, `{"response":"hi hungry, I'm dad"}`, ""},
	{"delete", NewDeleteDadResponseEndpoint, "", "7"},
}

func TestServiceFailureIsGeneric500(t *testing.T) {
	orig := newDadService
	newDadService = func(nivek.NivekService) dadSvc.NivekDadService { return failingDad{} }
	t.Cleanup(func() { newDadService = orig })

	for _, tc := range dadCases {
		t.Run(tc.name, func(t *testing.T) {
			svc, hook := newFakeNivek()
			rec := run(t, tc.ctor(svc), tc.body, tc.id, linkedUser())
			assertGeneric500(t, rec, hook)
		})
	}
}

func TestNoUserContextIs401Unauthorized(t *testing.T) {
	for _, tc := range dadCases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newFakeNivek()
			rec := run(t, tc.ctor(svc), tc.body, tc.id, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["error"] != "unauthorized" {
				t.Fatalf("body error = %q, want %q", body["error"], "unauthorized")
			}
		})
	}
}

func TestMissingTwitchLoginDoesNotLeakUserID(t *testing.T) {
	for _, tc := range dadCases {
		t.Run(tc.name, func(t *testing.T) {
			svc, hook := newFakeNivek()
			rec := run(t, tc.ctor(svc), tc.body, tc.id, &userlib.User{Id: 98765})
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", rec.Code)
			}
			if strings.Contains(rec.Body.String(), "98765") {
				t.Fatalf("body leaks user id: %s", rec.Body.String())
			}
			entry := hook.LastEntry()
			if entry == nil || !strings.Contains(entry.Message, "98765") {
				t.Fatalf("want an Error log line naming the user id, got %+v", entry)
			}
		})
	}
}
