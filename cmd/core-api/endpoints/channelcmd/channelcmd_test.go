package channelcmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	commandSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
	userlib "github.com/tim-the-toolman-taylor/nivek/internal/libraries/user"
)

// fakeEditor records the channel id every call was scoped to.
type fakeEditor struct {
	channels []string
	ids      []int
	err      error
}

func (f *fakeEditor) seen(ch string, id int) {
	f.channels = append(f.channels, ch)
	f.ids = append(f.ids, id)
}

func (f *fakeEditor) ListCustom(ch string) ([]commandSvc.Commands, error) {
	f.seen(ch, 0)
	return []commandSvc.Commands{}, f.err
}
func (f *fakeEditor) CreateCustom(ch string, in commandSvc.CustomCommandInput) (commandSvc.Commands, error) {
	f.seen(ch, 0)
	if f.err != nil {
		return commandSvc.Commands{}, f.err
	}
	if _, err := in.Normalize(); err != nil {
		return commandSvc.Commands{}, err
	}
	return commandSvc.Commands{Id: 1, Trigger: in.Trigger}, nil
}
func (f *fakeEditor) UpdateCustom(ch string, id int, _ commandSvc.CustomCommandInput) (commandSvc.Commands, error) {
	f.seen(ch, id)
	return commandSvc.Commands{Id: id}, f.err
}
func (f *fakeEditor) DeleteCustom(ch string, id int) error { f.seen(ch, id); return f.err }
func (f *fakeEditor) ListSettings(ch string) ([]commandSvc.CommandSetting, error) {
	f.seen(ch, 0)
	return []commandSvc.CommandSetting{}, f.err
}
func (f *fakeEditor) PutSetting(ch string, id int, _ commandSvc.CommandSettingInput) error {
	f.seen(ch, id)
	return f.err
}
func (f *fakeEditor) DeleteSetting(ch string, id int) error { f.seen(ch, id); return f.err }

func strp(s string) *string { return &s }

func run(t *testing.T, h echo.HandlerFunc, method, body, id string, u *userlib.User) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
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
	return rec, h(c)
}

func httpStatus(t *testing.T, rec *httptest.ResponseRecorder, err error) int {
	t.Helper()
	if err != nil {
		var he *echo.HTTPError
		if !errors.As(err, &he) {
			t.Fatalf("unexpected non-HTTP error: %v", err)
		}
		return he.Code
	}
	return rec.Code
}

func owner() *userlib.User {
	return &userlib.User{Id: 9, TwitchID: strp("1111"), TwitchLogin: strp("owner")}
}

// The channel is always the session user's Twitch id — a channel id smuggled
// into the body is ignored.
func TestCreateScopedToSessionChannel(t *testing.T) {
	f := &fakeEditor{}
	body := `{"trigger":"!discord","response_tmpl":"hi","cooldown_secs":30,"channel_twitch_id":"2222"}`
	rec, err := run(t, createCustom(nil, f), http.MethodPost, body, "", owner())
	if got := httpStatus(t, rec, err); got != http.StatusOK {
		t.Fatalf("status = %d, body %s", got, rec.Body.String())
	}
	if len(f.channels) != 1 || f.channels[0] != "1111" {
		t.Fatalf("editor scoped to %v; want [1111]", f.channels)
	}
}

func TestEveryHandlerUsesSessionChannel(t *testing.T) {
	handlers := map[string]func(f *fakeEditor) echo.HandlerFunc{
		"list":     func(f *fakeEditor) echo.HandlerFunc { return listCustom(nil, f) },
		"update":   func(f *fakeEditor) echo.HandlerFunc { return updateCustom(nil, f) },
		"delete":   func(f *fakeEditor) echo.HandlerFunc { return deleteCustom(nil, f) },
		"settings": func(f *fakeEditor) echo.HandlerFunc { return listSettings(nil, f) },
		"put":      func(f *fakeEditor) echo.HandlerFunc { return putSetting(nil, f) },
		"reset":    func(f *fakeEditor) echo.HandlerFunc { return deleteSetting(nil, f) },
	}
	for name, mk := range handlers {
		f := &fakeEditor{}
		rec, err := run(t, mk(f), http.MethodPut, `{}`, "7", owner())
		if got := httpStatus(t, rec, err); got != http.StatusOK {
			t.Errorf("%s: status = %d", name, got)
		}
		if len(f.channels) != 1 || f.channels[0] != "1111" {
			t.Errorf("%s: editor scoped to %v; want [1111]", name, f.channels)
		}
	}
}

func TestRejectsWithoutSessionOrTwitchLink(t *testing.T) {
	f := &fakeEditor{}
	rec, err := run(t, listCustom(nil, f), http.MethodGet, "", "", nil)
	if got := httpStatus(t, rec, err); got != http.StatusUnauthorized {
		t.Fatalf("no session: status = %d; want 401", got)
	}
	rec, err = run(t, listCustom(nil, f), http.MethodGet, "", "", &userlib.User{Id: 1})
	if got := httpStatus(t, rec, err); got != http.StatusBadRequest {
		t.Fatalf("no twitch link: status = %d; want 400", got)
	}
	if len(f.channels) != 0 {
		t.Fatalf("editor must not be called, got %v", f.channels)
	}
}

func TestBadIDRejected(t *testing.T) {
	for _, id := range []string{"abc", "0", "-3"} {
		f := &fakeEditor{}
		rec, err := run(t, deleteCustom(nil, f), http.MethodDelete, "", id, owner())
		if got := httpStatus(t, rec, err); got != http.StatusBadRequest {
			t.Errorf("id %q: status = %d; want 400", id, got)
		}
		if len(f.channels) != 0 {
			t.Errorf("id %q: editor must not be called", id)
		}
	}
}

func TestValidationErrorIs400(t *testing.T) {
	f := &fakeEditor{}
	body := `{"trigger":"!so","response_tmpl":"hi","cooldown_secs":30}`
	rec, err := run(t, createCustom(nil, f), http.MethodPost, body, "", owner())
	if got := httpStatus(t, rec, err); got != http.StatusBadRequest {
		t.Fatalf("built-in trigger: status = %d; want 400", got)
	}
	body = `{"trigger":"!ok","response_tmpl":"hi","cooldown_secs":1}`
	rec, err = run(t, createCustom(nil, f), http.MethodPost, body, "", owner())
	if got := httpStatus(t, rec, err); got != http.StatusBadRequest {
		t.Fatalf("cooldown 1s: status = %d; want 400", got)
	}
}

func TestStatusFor(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{fmt.Errorf("%w: bad", commandSvc.ErrValidation), http.StatusBadRequest},
		{commandSvc.ErrNotFound, http.StatusNotFound},
		{fmt.Errorf("%w: dup", commandSvc.ErrConflict), http.StatusConflict},
		{fmt.Errorf("%w: cap", commandSvc.ErrLimit), http.StatusConflict},
	}
	for _, tc := range cases {
		if got, _ := statusFor(tc.err); got != tc.want {
			t.Errorf("statusFor(%v) = %d; want %d", tc.err, got, tc.want)
		}
	}
	got, msg := statusFor(errors.New("pq: connection refused at 10.0.0.1"))
	if got != http.StatusInternalServerError || strings.Contains(msg, "10.0.0.1") {
		t.Fatalf("internal error leaked or wrong status: %d %q", got, msg)
	}
}

// A not-found from the editor (e.g. another channel's command id) is a 404.
func TestNotOwnedIs404(t *testing.T) {
	f := &fakeEditor{err: commandSvc.ErrNotFound}
	rec, err := run(t, deleteCustom(nil, f), http.MethodDelete, "", "42", owner())
	if got := httpStatus(t, rec, err); got != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", got)
	}
}
