package commands

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// memEditor is an in-memory ChannelEditorService that mimics the real
// editor's normalization, conflict and limit checks for one channel.
type memEditor struct {
	cmds    map[int]Commands
	nextID  int
	err     error // forced internal error for every call
	updated []CustomCommandInput
}

func newMemEditor() *memEditor { return &memEditor{cmds: map[int]Commands{}, nextID: 1} }

func (m *memEditor) ListCustom(string) ([]Commands, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := []Commands{}
	for _, c := range m.cmds {
		out = append(out, c)
	}
	return out, nil
}

func (m *memEditor) CreateCustom(ch string, in CustomCommandInput) (Commands, error) {
	if m.err != nil {
		return Commands{}, m.err
	}
	in, err := in.Normalize()
	if err != nil {
		return Commands{}, err
	}
	if len(m.cmds) >= MaxCustomCommandsPerChannel {
		return Commands{}, fmt.Errorf("%w: at most %d custom commands per channel", ErrLimit, MaxCustomCommandsPerChannel)
	}
	for _, c := range m.cmds {
		if c.Trigger == in.Trigger {
			return Commands{}, fmt.Errorf("%w: you already have a %s command", ErrConflict, in.Trigger)
		}
	}
	resp := in.ResponseTmpl
	c := Commands{Id: m.nextID, Trigger: in.Trigger, Kind: "custom", ResponseTmpl: &resp, Scope: "channel",
		ChannelTwitchId: &ch, Enabled: *in.Enabled, MinRole: in.MinRole, CooldownSecs: *in.CooldownSecs}
	m.cmds[c.Id] = c
	m.nextID++
	return c, nil
}

func (m *memEditor) UpdateCustom(_ string, id int, in CustomCommandInput) (Commands, error) {
	if m.err != nil {
		return Commands{}, m.err
	}
	in, err := in.Normalize()
	if err != nil {
		return Commands{}, err
	}
	c, ok := m.cmds[id]
	if !ok {
		return Commands{}, ErrNotFound
	}
	m.updated = append(m.updated, in)
	resp := in.ResponseTmpl
	c.ResponseTmpl, c.Description, c.MinRole, c.CooldownSecs, c.Enabled = &resp, in.Description, in.MinRole, *in.CooldownSecs, *in.Enabled
	m.cmds[id] = c
	return c, nil
}

func (m *memEditor) DeleteCustom(_ string, id int) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.cmds[id]; !ok {
		return ErrNotFound
	}
	delete(m.cmds, id)
	return nil
}

func (m *memEditor) ListSettings(string) ([]CommandSetting, error)     { return nil, nil }
func (m *memEditor) PutSetting(string, int, CommandSettingInput) error { return nil }
func (m *memEditor) DeleteSetting(string, int) error                   { return nil }

func manage(t *testing.T, ed ChannelEditorService, action, trigger, response string) ChatManageResult {
	t.Helper()
	res, err := ManageFromChat(ed, ChatManageRequest{ChannelTwitchID: "9", Action: action, Trigger: trigger, Response: response})
	if err != nil {
		t.Fatalf("ManageFromChat(%s %s): unexpected error %v", action, trigger, err)
	}
	return res
}

func TestNormalizeChatAction(t *testing.T) {
	for in, want := range map[string]string{
		"add": ChatActionAdd, "ADD": ChatActionAdd, "edit": ChatActionEdit,
		"del": ChatActionDelete, "delete": ChatActionDelete, "Remove": ChatActionDelete,
	} {
		if got, ok := NormalizeChatAction(in); !ok || got != want {
			t.Errorf("NormalizeChatAction(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := NormalizeChatAction("frob"); ok {
		t.Error("unknown verb must be rejected")
	}
}

func TestManageFromChatAddEditDelete(t *testing.T) {
	ed := newMemEditor()

	res := manage(t, ed, "add", "Hello", "Hi there!")
	if !res.OK || res.Trigger != "!hello" || res.Command == nil || *res.Command.ResponseTmpl != "Hi there!" {
		t.Fatalf("add: %+v", res)
	}
	if res.Command.CooldownSecs != ChatDefaultCooldownSecs || res.Command.MinRole != "everyone" || !res.Command.Enabled {
		t.Fatalf("add defaults wrong: %+v", res.Command)
	}

	if res := manage(t, ed, "add", "!hello", "again"); res.OK || !strings.Contains(res.Error, "already have") {
		t.Fatalf("duplicate add should be refused: %+v", res)
	}

	// Edit keeps the existing settings and only swaps the response.
	id := res.Command.Id
	c := ed.cmds[id]
	c.MinRole, c.CooldownSecs, c.Description = "vip", 60, "desc"
	ed.cmds[id] = c
	res = manage(t, ed, "edit", "!hello", "New text")
	if !res.OK || *res.Command.ResponseTmpl != "New text" {
		t.Fatalf("edit: %+v", res)
	}
	if got := ed.updated[0]; got.MinRole != "vip" || *got.CooldownSecs != 60 || got.Description != "desc" || got.Trigger != "!hello" {
		t.Fatalf("edit must preserve existing fields, sent %+v", got)
	}

	res = manage(t, ed, "delete", "!hello", "")
	if !res.OK || res.Trigger != "!hello" || len(ed.cmds) != 0 {
		t.Fatalf("delete: %+v (left %d)", res, len(ed.cmds))
	}
	if res := manage(t, ed, "delete", "!hello", ""); res.OK || !strings.Contains(res.Error, "doesn't exist") {
		t.Fatalf("delete missing should be refused: %+v", res)
	}
	if res := manage(t, ed, "edit", "!nope", "x"); res.OK || !strings.Contains(res.Error, "doesn't exist") {
		t.Fatalf("edit missing should be refused: %+v", res)
	}
}

func TestManageFromChatValidation(t *testing.T) {
	ed := newMemEditor()
	for _, c := range []struct{ action, trigger, response, want string }{
		{"add", "!fish", "x", "built-in"},
		{"add", "!cmd", "x", "built-in"},
		{"edit", "!fish", "x", "built-in"},
		{"add", "!bad-name", "x", "trigger must be"},
		{"add", "!ok", "/ban someone", "can't start with"},
		{"add", "!ok", ".me hi", "can't start with"},
		{"add", "!ok", "!fish", "can't start with"},
		{"add", "!ok", strings.Repeat("a", MaxResponseLen+1), "at most"},
		{"add", "!ok", "", "response is required"},
		{"frob", "!ok", "x", "unknown action"},
	} {
		res := manage(t, ed, c.action, c.trigger, c.response)
		if res.OK || !strings.Contains(res.Error, c.want) {
			t.Errorf("%s %s %q: got %+v, want error containing %q", c.action, c.trigger, c.response, res, c.want)
		}
		if strings.HasPrefix(res.Error, "invalid input") || strings.HasPrefix(res.Error, "conflict") {
			t.Errorf("sentinel prefix leaked to chat: %q", res.Error)
		}
	}
	if len(ed.cmds) != 0 {
		t.Fatalf("refused requests created %d rows", len(ed.cmds))
	}
}

func TestManageFromChatLimit(t *testing.T) {
	ed := newMemEditor()
	for i := range MaxCustomCommandsPerChannel {
		if res := manage(t, ed, "add", fmt.Sprintf("!c%d", i), "x"); !res.OK {
			t.Fatalf("add %d refused: %+v", i, res)
		}
	}
	if res := manage(t, ed, "add", "!onemore", "x"); res.OK || !strings.Contains(res.Error, "at most") {
		t.Fatalf("51st command should hit the cap: %+v", res)
	}
}

func TestManageFromChatInternalErrorPropagates(t *testing.T) {
	ed := newMemEditor()
	ed.err = errors.New("db down")
	for _, action := range []string{"add", "edit", "del"} {
		if _, err := ManageFromChat(ed, ChatManageRequest{ChannelTwitchID: "9", Action: action, Trigger: "!x", Response: "y"}); err == nil {
			t.Errorf("%s: internal error should be returned, not shown to chat", action)
		}
	}
}

func TestManageFromChatRequiresChannel(t *testing.T) {
	res, err := ManageFromChat(newMemEditor(), ChatManageRequest{Action: "add", Trigger: "!x", Response: "y"})
	if err != nil || res.OK {
		t.Fatalf("missing channel should be refused: %+v %v", res, err)
	}
}

// TestManageFromChatKeepsResponseVariables guards the #169 contract: ${...}
// response variables are stored verbatim (expanded by the bot at reply time),
// so chat add/edit must not reject or rewrite them.
func TestManageFromChatKeepsResponseVariables(t *testing.T) {
	ed := newMemEditor()
	tmpl := "hi ${touser}, ${user} says ${random.pick 'a' \"b\"} (#${count}, ${random.1-100}, ${1})"
	res := manage(t, ed, "add", "!vars", tmpl)
	if !res.OK || res.Command == nil || *res.Command.ResponseTmpl != tmpl {
		t.Fatalf("add with response variables: %+v", res)
	}
	edited := "now ${channel} ${2}"
	if res = manage(t, ed, "edit", "!vars", edited); !res.OK || *res.Command.ResponseTmpl != edited {
		t.Fatalf("edit with response variables: %+v", res)
	}
}
