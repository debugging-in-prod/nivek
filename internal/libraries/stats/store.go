package stats

import (
	"context"
	"fmt"
	"time"

	"github.com/upper/db/v4"
)

// Store is the database side of the stats feature. Implemented by dbStore;
// tests substitute a fake.
type Store interface {
	ActiveChats(ctx context.Context) (int, error)
	TopChannels(ctx context.Context, since time.Time, limit int) ([]ChannelUses, error)
	TopFishers(ctx context.Context, limit int) ([]Fisher, error)
	UpsertUsage(ctx context.Context, rows []UsageCount) error
}

type dbStore struct {
	sess db.Session
}

// NewStore wraps a database session.
func NewStore(sess db.Session) Store {
	return &dbStore{sess: sess}
}

func (s *dbStore) ActiveChats(ctx context.Context) (int, error) {
	q, args := ActiveChatsQuery(HomeChannelLogins)
	row, err := s.sess.SQL().QueryRowContext(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("active chats: %w", err)
	}
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("active chats: %w", err)
	}
	return n, nil
}

func (s *dbStore) TopChannels(ctx context.Context, since time.Time, limit int) ([]ChannelUses, error) {
	q, args := TopChannelsQuery(since, HomeChannelLogins, limit)
	rows, err := s.sess.SQL().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("top channels: %w", err)
	}
	defer rows.Close()
	out := []ChannelUses{}
	for rows.Next() {
		var c ChannelUses
		if err := rows.Scan(&c.DisplayName, &c.Uses); err != nil {
			return nil, fmt.Errorf("top channels scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *dbStore) TopFishers(ctx context.Context, limit int) ([]Fisher, error) {
	q, args := TopFishersQuery(limit)
	rows, err := s.sess.SQL().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("top fishers: %w", err)
	}
	defer rows.Close()
	out := []Fisher{}
	for rows.Next() {
		var f Fisher
		if err := rows.Scan(&f.DisplayName, &f.Score, &f.Catches); err != nil {
			return nil, fmt.Errorf("top fishers scan: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// upsertChunk keeps each statement well under Postgres' 65535 bind-parameter
// limit (5 params per row).
const upsertChunk = 200

func (s *dbStore) UpsertUsage(ctx context.Context, rows []UsageCount) error {
	for start := 0; start < len(rows); start += upsertChunk {
		end := min(start+upsertChunk, len(rows))
		q, args := UpsertUsageQuery(rows[start:end])
		if _, err := s.sess.SQL().ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("upsert command usage: %w", err)
		}
	}
	return nil
}
