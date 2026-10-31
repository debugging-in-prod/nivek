package commands

import (
	"errors"
	"fmt"
	"strings"
)

// Chat-side management of a channel's custom commands ("!cmd add|edit|del").
// The bot parses the chat message and a mod/broadcaster check, then posts a
// ChatManageRequest to core-api; core-api runs it through ManageFromChat, which
// reuses the dashboard editor (and therefore all of its validation: trigger
// format, built-in names, response rules, the per-channel cap).

// Chat actions accepted by ManageFromChat (after NormalizeChatAction).
const (
	ChatActionAdd    = "add"
	ChatActionEdit   = "edit"
	ChatActionDelete = "delete"
)

// ChatDefaultCooldownSecs is the cooldown given to a command created from chat
// (the dashboard can change it later). Must stay within
// MinCooldownSecs..MaxCooldownSecs.
const ChatDefaultCooldownSecs = 10

// ChatManageRequest is the bot -> core-api body for POST /bot/commands/manage.
type ChatManageRequest struct {
	ChannelTwitchID string `json:"channel_twitch_id"`
	Action          string `json:"action"`
	Trigger         string `json:"trigger"`
	Response        string `json:"response"`
}

// ChatManageResult is the core-api -> bot reply. OK=false with Error set means
// the request was refused for a user-facing reason (validation, conflict,
// limit, not found); the message is safe to echo to chat. Command is the saved
// row for add/edit.
type ChatManageResult struct {
	OK      bool      `json:"ok"`
	Error   string    `json:"error,omitempty"`
	Action  string    `json:"action"`
	Trigger string    `json:"trigger"`
	Command *Commands `json:"command,omitempty"`
}

// NormalizeChatAction maps the chat verbs onto the canonical actions. ok is
// false for anything else.
func NormalizeChatAction(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "add":
		return ChatActionAdd, true
	case "edit":
		return ChatActionEdit, true
	case "del", "delete", "remove":
		return ChatActionDelete, true
	}
	return "", false
}

// ManageFromChat applies one chat !cmd request to the channel's custom
// commands through editor. A non-nil error is an internal failure (DB etc.);
// user-facing refusals come back as OK=false with a short Error message.
func ManageFromChat(editor ChannelEditorService, req ChatManageRequest) (ChatManageResult, error) {
	action, ok := NormalizeChatAction(req.Action)
	if !ok {
		return refused(req.Action, req.Trigger, "unknown action, use add, edit or del"), nil
	}
	channelID := strings.TrimSpace(req.ChannelTwitchID)
	if channelID == "" {
		return refused(action, req.Trigger, "missing channel"), nil
	}
	trigger, err := NormalizeTrigger(req.Trigger)
	if err != nil {
		return refusedFor(action, req.Trigger, err)
	}

	switch action {
	case ChatActionAdd:
		cd := ChatDefaultCooldownSecs
		created, err := editor.CreateCustom(channelID, CustomCommandInput{
			Trigger:      trigger,
			ResponseTmpl: req.Response,
			MinRole:      "everyone",
			CooldownSecs: &cd,
		})
		if err != nil {
			return refusedFor(action, trigger, err)
		}
		return ChatManageResult{OK: true, Action: action, Trigger: created.Trigger, Command: &created}, nil

	case ChatActionEdit:
		existing, found, err := findCustomByTrigger(editor, channelID, trigger)
		if err != nil {
			return ChatManageResult{}, err
		}
		if !found {
			return refused(action, trigger, fmt.Sprintf("%s doesn't exist, add it with !cmd add", trigger)), nil
		}
		cd := existing.CooldownSecs
		if cd < MinCooldownSecs || cd > MaxCooldownSecs {
			cd = ChatDefaultCooldownSecs
		}
		enabled := existing.Enabled
		updated, err := editor.UpdateCustom(channelID, existing.Id, CustomCommandInput{
			Trigger:      existing.Trigger,
			ResponseTmpl: req.Response,
			Description:  existing.Description,
			MinRole:      existing.MinRole,
			CooldownSecs: &cd,
			Enabled:      &enabled,
		})
		if err != nil {
			return refusedFor(action, trigger, err)
		}
		return ChatManageResult{OK: true, Action: action, Trigger: updated.Trigger, Command: &updated}, nil

	default: // ChatActionDelete
		existing, found, err := findCustomByTrigger(editor, channelID, trigger)
		if err != nil {
			return ChatManageResult{}, err
		}
		if !found {
			return refused(action, trigger, fmt.Sprintf("%s doesn't exist", trigger)), nil
		}
		if err := editor.DeleteCustom(channelID, existing.Id); err != nil {
			return refusedFor(action, trigger, err)
		}
		return ChatManageResult{OK: true, Action: action, Trigger: existing.Trigger}, nil
	}
}

// findCustomByTrigger scans the channel's custom commands (at most
// MaxCustomCommandsPerChannel rows) for trigger.
func findCustomByTrigger(editor ChannelEditorService, channelID, trigger string) (Commands, bool, error) {
	cmds, err := editor.ListCustom(channelID)
	if err != nil {
		return Commands{}, false, err
	}
	for _, c := range cmds {
		if strings.EqualFold(c.Trigger, trigger) {
			return c, true, nil
		}
	}
	return Commands{}, false, nil
}

func refused(action, trigger, msg string) ChatManageResult {
	return ChatManageResult{OK: false, Action: action, Trigger: trigger, Error: msg}
}

// refusedFor turns a known editor error into a user-facing refusal and passes
// anything else through as an internal error.
func refusedFor(action, trigger string, err error) (ChatManageResult, error) {
	if msg, ok := UserFacingError(err); ok {
		return refused(action, trigger, msg), nil
	}
	return ChatManageResult{}, err
}

// UserFacingError returns a short message for the editor's sentinel errors,
// with the sentinel prefix ("invalid input: ", "conflict: ", ...) stripped.
// ok is false for internal errors, which must not be shown to users.
func UserFacingError(err error) (string, bool) {
	for _, sentinel := range []error{ErrValidation, ErrConflict, ErrLimit, ErrNotFound} {
		if errors.Is(err, sentinel) {
			msg := strings.TrimPrefix(err.Error(), sentinel.Error()+": ")
			if errors.Is(err, ErrNotFound) {
				msg = "command not found"
			}
			return msg, true
		}
	}
	return "", false
}
