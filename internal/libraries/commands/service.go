package commands

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/upper/db/v4"
)

type CommandsService interface {
	GetGlobalEnabledCommands() ([]Commands, error)
	GetChannelEnabledCommands(channelTwitchID string) ([]Commands, error)
	IncrementChannelCommandUse(commandID int, channelTwitchID string) (count int, found bool, err error)
}

type commandsImpl struct {
	nivek         nivek.NivekService
	commandsTable db.Collection
}

func NewService(service nivek.NivekService) CommandsService {
	return &commandsImpl{
		nivek:         service,
		commandsTable: service.Postgres().GetDefaultConnection().Collection(TableCommands),
	}
}

func (s *commandsImpl) GetGlobalEnabledCommands() ([]Commands, error) {
	var commands []Commands

	if err := s.commandsTable.Find(db.Cond{
		"scope":   "global",
		"enabled": true,
	}).All(&commands); err != nil {
		return nil, err
	}

	return commands, nil
}

// GetChannelEnabledCommands returns the enabled channel-scoped (per-channel
// custom) commands for one broadcaster, keyed by their Twitch id. These are the
// rows the bot loads when a channel goes live so its custom triggers respond.
// Global commands are excluded — those load once at boot via
// GetGlobalEnabledCommands.
func (s *commandsImpl) GetChannelEnabledCommands(channelTwitchID string) ([]Commands, error) {
	var commands []Commands

	if err := s.commandsTable.Find(db.Cond{
		"scope":             "channel",
		"channel_twitch_id": channelTwitchID,
		"enabled":           true,
	}).All(&commands); err != nil {
		return nil, err
	}

	return commands, nil
}

// IncrementChannelCommandUse atomically bumps use_count on one channel-scoped
// command and returns the new value. The row must belong to channelTwitchID,
// so a caller can only count its own channel's commands. found is false when
// no such row exists.
func (s *commandsImpl) IncrementChannelCommandUse(commandID int, channelTwitchID string) (int, bool, error) {
	const query = `
		UPDATE nivek.command
		   SET use_count = use_count + 1
		 WHERE id = $1
		   AND scope = 'channel'
		   AND channel_twitch_id = $2
		RETURNING use_count`

	row, err := s.commandsTable.Session().SQL().QueryRow(query, commandID, channelTwitchID)
	if err != nil {
		return 0, false, fmt.Errorf("increment command use: %w", err)
	}

	var count int
	if err := row.Scan(&count); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("scan command use count: %w", err)
	}
	return count, true, nil
}
