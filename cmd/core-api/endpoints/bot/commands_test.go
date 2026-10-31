package bot

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	commandSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/commands"
)

// stubEditor implements just enough of ChannelEditorService for the manage
// endpoint; unused methods panic via the nil embedded interface.
type stubEditor struct {
	commandSvc.ChannelEditorService
	err      error
	channels []string
}

func (s *stubEditor) CreateCustom(ch string, in commandSvc.CustomCommandInput) (commandSvc.Commands, error) {
	s.channels = append(s.channels, ch)
	if s.err != nil {
		return commandSvc.Commands{}, s.err
	}
	in, err := in.Normalize()
	if err != nil {
		return commandSvc.Commands{}, err
	}
	return commandSvc.Commands{Id: 1, Trigger: in.Trigger, ResponseTmpl: &in.ResponseTmpl, Enabled: true}, nil
}

func postManage(t *testing.T, ed commandSvc.ChannelEditorService, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := postManageChannelCommand(nil, ed)(e.NewContext(req, rec)); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return rec
}

func TestPostManageChannelCommand(t *testing.T) {
	ed := &stubEditor{}
	rec := postManage(t, ed, `{"channel_twitch_id":"9","action":"add","trigger":"!yo","response":"hi"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var res commandSvc.ChatManageResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Trigger != "!yo" || res.Command == nil || len(ed.channels) != 1 || ed.channels[0] != "9" {
		t.Fatalf("unexpected result %+v (channels %v)", res, ed.channels)
	}

	// Validation refusal is a 200 with ok=false and a chat-safe message.
	rec = postManage(t, ed, `{"channel_twitch_id":"9","action":"add","trigger":"!fish","response":"hi"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":false`) {
		t.Fatalf("refusal: %d %s", rec.Code, rec.Body)
	}

	if rec := postManage(t, ed, `{"action":"add","trigger":"!yo","response":"hi"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing channel: status %d", rec.Code)
	}
	if rec := postManage(t, ed, `not json`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad body: status %d", rec.Code)
	}

	// Internal errors are a generic 500 (details only in the server log).
	ed.err = errors.New("db exploded: secret detail")
	rec = postManage(t, ed, `{"channel_twitch_id":"9","action":"add","trigger":"!yo","response":"hi"}`)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("internal error: %d %s", rec.Code, rec.Body)
	}
}
