package weather

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	userlib "github.com/tim-the-toolman-taylor/nivek/internal/libraries/user"
)

// secret is the internal error text that must reach the log but never the
// response body.
const secret = "pq: connection refused to 10.0.0.5"

var errForced = errors.New(secret)

// fakeNivek only supplies a logger. Any other NivekService call panics, which
// proves the endpoint never reached the real DB.
type fakeNivek struct {
	nivek.NivekService
	logger *logrus.Logger
}

func (f fakeNivek) Logger() *logrus.Logger { return f.logger }

func newFakeNivek() (fakeNivek, *logtest.Hook) {
	l, hook := logtest.NewNullLogger()
	return fakeNivek{logger: l}, hook
}

func strp(s string) *string { return &s }

func linkedUser() *userlib.User {
	return &userlib.User{Id: 42, TwitchLogin: strp("SomeChannel"), TwitchID: strp("12345")}
}

func run(t *testing.T, h echo.HandlerFunc, body, id string, u *userlib.User) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if id != "" {
		c.SetParamNames("id")
		c.SetParamValues(id)
	}
	if u != nil {
		c.Set("user", u)
	}
	if err := h(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	return rec
}

// assertGeneric500 checks the response is a 500 with the generic body and that
// the internal detail went to an Error log line instead.
func assertGeneric500(t *testing.T, rec *httptest.ResponseRecorder, hook *logtest.Hook) {
	t.Helper()
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "internal server error" {
		t.Fatalf("body error = %q, want generic message", body["error"])
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("body leaks internal error: %s", rec.Body.String())
	}
	entry := hook.LastEntry()
	if entry == nil || entry.Level != logrus.ErrorLevel || !strings.Contains(entry.Message, secret) {
		t.Fatalf("want an Error log line containing the internal error, got %+v", entry)
	}
}
