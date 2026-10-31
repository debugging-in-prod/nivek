package commands

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/upper/db/v4"
)

// TableChannelCommandSettings holds per-channel overrides of global commands.
const TableChannelCommandSettings = "channel_command_settings"

var (
	// ErrNotFound: the row doesn't exist or isn't owned by the channel.
	ErrNotFound = errors.New("not found")
	// ErrConflict: the channel already has a command with this trigger, or the
	// trigger collides with a global built-in.
	ErrConflict = errors.New("conflict")
	// ErrLimit: the channel already owns MaxCustomCommandsPerChannel commands.
	ErrLimit = errors.New("limit reached")
)

// CommandSetting is a global command merged with the channel's override. The
// Override* fields are nil when the channel inherits the default.
type CommandSetting struct {
	CommandId           int     `json:"command_id"`
	Trigger             string  `json:"trigger"`
	Description         string  `json:"description"`
	DefaultEnabled      bool    `json:"default_enabled"`
	DefaultMinRole      string  `json:"default_min_role"`
	DefaultCooldownSecs int     `json:"default_cooldown_secs"`
	Enabled             *bool   `json:"enabled"`
	MinRole             *string `json:"min_role"`
	CooldownSecs        *int    `json:"cooldown_secs"`
}

// ChannelEditorService backs the dashboard editors. Every method is scoped to
// channelTwitchID, which callers must take from the authenticated session —
// never from the request — so a user can only touch their own channel.
type ChannelEditorService interface {
	ListCustom(channelTwitchID string) ([]Commands, error)
	CreateCustom(channelTwitchID string, in CustomCommandInput) (Commands, error)
	UpdateCustom(channelTwitchID string, id int, in CustomCommandInput) (Commands, error)
	DeleteCustom(channelTwitchID string, id int) error

	ListSettings(channelTwitchID string) ([]CommandSetting, error)
	PutSetting(channelTwitchID string, commandID int, in CommandSettingInput) error
	DeleteSetting(channelTwitchID string, commandID int) error
}

type channelEditorImpl struct {
	commandsTable db.Collection
	settingsTable db.Collection
}

func NewChannelEditorService(service nivek.NivekService) ChannelEditorService {
	sess := service.Postgres().GetDefaultConnection()
	return &channelEditorImpl{
		commandsTable: sess.Collection(TableCommands),
		settingsTable: sess.Collection(TableChannelCommandSettings),
	}
}

func customCond(channelTwitchID string) db.Cond {
	return db.Cond{
		"scope":             "channel",
		"kind":              "custom",
		"channel_twitch_id": channelTwitchID,
	}
}

func (s *channelEditorImpl) ListCustom(channelTwitchID string) ([]Commands, error) {
	cmds := []Commands{}
	if err := s.commandsTable.Find(customCond(channelTwitchID)).OrderBy("trigger").All(&cmds); err != nil {
		return nil, fmt.Errorf("list custom commands for %s: %w", channelTwitchID, err)
	}
	return cmds, nil
}

// triggerTaken reports whether trigger is a global command or another of this
// channel's commands (excluding excludeID, for renames).
func (s *channelEditorImpl) triggerTaken(channelTwitchID, trigger string, excludeID int) error {
	n, err := s.commandsTable.Find(db.Cond{"scope": "global", "trigger": trigger}).Count()
	if err != nil {
		return fmt.Errorf("check global trigger %s: %w", trigger, err)
	}
	if n > 0 {
		return fmt.Errorf("%w: %s is a built-in command", ErrConflict, trigger)
	}
	cond := db.Cond{"scope": "channel", "channel_twitch_id": channelTwitchID, "trigger": trigger}
	if excludeID > 0 {
		cond["id"] = db.NotEq(excludeID)
	}
	n, err = s.commandsTable.Find(cond).Count()
	if err != nil {
		return fmt.Errorf("check channel trigger %s: %w", trigger, err)
	}
	if n > 0 {
		return fmt.Errorf("%w: you already have a %s command", ErrConflict, trigger)
	}
	return nil
}

func (s *channelEditorImpl) CreateCustom(channelTwitchID string, in CustomCommandInput) (Commands, error) {
	if strings.TrimSpace(channelTwitchID) == "" {
		return Commands{}, invalid("no linked twitch account")
	}
	in, err := in.Normalize()
	if err != nil {
		return Commands{}, err
	}

	count, err := s.commandsTable.Find(db.Cond{"scope": "channel", "channel_twitch_id": channelTwitchID}).Count()
	if err != nil {
		return Commands{}, fmt.Errorf("count custom commands for %s: %w", channelTwitchID, err)
	}
	if count >= MaxCustomCommandsPerChannel {
		return Commands{}, fmt.Errorf("%w: at most %d custom commands per channel", ErrLimit, MaxCustomCommandsPerChannel)
	}
	if err := s.triggerTaken(channelTwitchID, in.Trigger, 0); err != nil {
		return Commands{}, err
	}

	// A map insert lets the DB fill id/created_at/updated_at defaults (the
	// Commands struct has no omitempty on those columns).
	res, err := s.commandsTable.Insert(map[string]any{
		"trigger":           in.Trigger,
		"kind":              "custom",
		"response_tmpl":     in.ResponseTmpl,
		"description":       in.Description,
		"scope":             "channel",
		"channel_twitch_id": channelTwitchID,
		"enabled":           *in.Enabled,
		"min_role":          in.MinRole,
		"cooldown_secs":     *in.CooldownSecs,
	})
	if err != nil {
		return Commands{}, fmt.Errorf("create custom command %s for %s: %w", in.Trigger, channelTwitchID, err)
	}

	var created Commands
	if err := s.commandsTable.Find(db.Cond{"id": res.ID()}).One(&created); err != nil {
		return Commands{}, fmt.Errorf("reload created command %s for %s: %w", in.Trigger, channelTwitchID, err)
	}
	return created, nil
}

func (s *channelEditorImpl) findOwned(channelTwitchID string, id int) (db.Result, error) {
	cond := customCond(channelTwitchID)
	cond["id"] = id
	res := s.commandsTable.Find(cond)
	n, err := res.Count()
	if err != nil {
		return nil, fmt.Errorf("find custom command %d for %s: %w", id, channelTwitchID, err)
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	return res, nil
}

func (s *channelEditorImpl) UpdateCustom(channelTwitchID string, id int, in CustomCommandInput) (Commands, error) {
	in, err := in.Normalize()
	if err != nil {
		return Commands{}, err
	}
	res, err := s.findOwned(channelTwitchID, id)
	if err != nil {
		return Commands{}, err
	}
	if err := s.triggerTaken(channelTwitchID, in.Trigger, id); err != nil {
		return Commands{}, err
	}

	if err := res.Update(map[string]any{
		"trigger":       in.Trigger,
		"response_tmpl": in.ResponseTmpl,
		"description":   in.Description,
		"enabled":       *in.Enabled,
		"min_role":      in.MinRole,
		"cooldown_secs": *in.CooldownSecs,
		"updated_at":    time.Now(),
	}); err != nil {
		return Commands{}, fmt.Errorf("update custom command %d for %s: %w", id, channelTwitchID, err)
	}

	var updated Commands
	if err := s.commandsTable.Find(db.Cond{"id": id}).One(&updated); err != nil {
		return Commands{}, fmt.Errorf("reload custom command %d for %s: %w", id, channelTwitchID, err)
	}
	return updated, nil
}

func (s *channelEditorImpl) DeleteCustom(channelTwitchID string, id int) error {
	res, err := s.findOwned(channelTwitchID, id)
	if err != nil {
		return err
	}
	if err := res.Delete(); err != nil {
		return fmt.Errorf("delete custom command %d for %s: %w", id, channelTwitchID, err)
	}
	return nil
}

// ListSettings returns every global command with the channel's override (if
// any) merged in.
func (s *channelEditorImpl) ListSettings(channelTwitchID string) ([]CommandSetting, error) {
	const query = `
		SELECT c.id, c.trigger, c.description, c.enabled, c.min_role, c.cooldown_secs,
		       s.enabled, s.min_role, s.cooldown_secs
		FROM nivek.command c
		LEFT JOIN nivek.channel_command_settings s
		  ON s.command_id = c.id AND s.channel_twitch_id = $1
		WHERE c.scope = 'global'
		ORDER BY c.trigger
	`
	rows, err := s.commandsTable.Session().SQL().Query(query, channelTwitchID)
	if err != nil {
		return nil, fmt.Errorf("list command settings for %s: %w", channelTwitchID, err)
	}
	defer rows.Close()

	out := []CommandSetting{}
	for rows.Next() {
		var cs CommandSetting
		if err := rows.Scan(
			&cs.CommandId, &cs.Trigger, &cs.Description,
			&cs.DefaultEnabled, &cs.DefaultMinRole, &cs.DefaultCooldownSecs,
			&cs.Enabled, &cs.MinRole, &cs.CooldownSecs,
		); err != nil {
			return nil, fmt.Errorf("scan command setting for %s: %w", channelTwitchID, err)
		}
		out = append(out, cs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate command settings for %s: %w", channelTwitchID, err)
	}
	return out, nil
}

// PutSetting upserts the channel's override of one global command. Overrides
// are only allowed on global rows; a channel's own custom commands carry their
// settings directly.
func (s *channelEditorImpl) PutSetting(channelTwitchID string, commandID int, in CommandSettingInput) error {
	if strings.TrimSpace(channelTwitchID) == "" {
		return invalid("no linked twitch account")
	}
	in, err := in.Normalize()
	if err != nil {
		return err
	}
	n, err := s.commandsTable.Find(db.Cond{"id": commandID, "scope": "global"}).Count()
	if err != nil {
		return fmt.Errorf("find global command %d: %w", commandID, err)
	}
	if n == 0 {
		return ErrNotFound
	}

	const query = `
		INSERT INTO nivek.channel_command_settings
		    (channel_twitch_id, command_id, enabled, min_role, cooldown_secs, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (channel_twitch_id, command_id) DO UPDATE
		SET enabled = EXCLUDED.enabled,
		    min_role = EXCLUDED.min_role,
		    cooldown_secs = EXCLUDED.cooldown_secs,
		    updated_at = now()
	`
	if _, err := s.settingsTable.Session().SQL().Exec(query,
		channelTwitchID, commandID, nullable(in.Enabled), nullable(in.MinRole), nullable(in.CooldownSecs),
	); err != nil {
		return fmt.Errorf("save setting for command %d in %s: %w", commandID, channelTwitchID, err)
	}
	return nil
}

// DeleteSetting removes the channel's override, reverting to the defaults.
// Deleting an override that doesn't exist is a no-op.
func (s *channelEditorImpl) DeleteSetting(channelTwitchID string, commandID int) error {
	if err := s.settingsTable.Find(db.Cond{
		"channel_twitch_id": channelTwitchID,
		"command_id":        commandID,
	}).Delete(); err != nil {
		return fmt.Errorf("reset setting for command %d in %s: %w", commandID, channelTwitchID, err)
	}
	return nil
}

// nullable turns a nil pointer into an untyped nil (SQL NULL) and a non-nil one
// into its value, independent of how the driver treats typed pointers.
func nullable[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}
