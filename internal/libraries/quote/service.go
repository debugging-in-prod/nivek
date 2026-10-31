package quote

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/upper/db/v4"
)

// addRetries bounds how often Add retries when two concurrent adds in one
// channel race for the same quote_number (the loser's insert hits the unique
// constraint, does nothing, and tries again with the next number).
const addRetries = 5

// ErrInvalid is returned for bad input (missing channel, empty/too-long text).
var ErrInvalid = errors.New("invalid quote request")

type NivekQuoteService interface {
	// Random returns a random quote for the channel. found is false when the
	// channel has none.
	Random(channelTwitchId string) (Quote, bool, error)
	// Get returns the channel's quote #number. found is false when it doesn't
	// exist.
	Get(channelTwitchId string, number int) (Quote, bool, error)
	// Add stores a new quote and returns it with its assigned per-channel
	// quote_number (max existing + 1).
	Add(channelTwitchId, text, addedBy string, game *string) (Quote, error)
	// Delete removes the channel's quote #number. found is false when there
	// was nothing to delete.
	Delete(channelTwitchId string, number int) (bool, error)
}

type nivekQuoteServiceImpl struct {
	quoteTable db.Collection
}

func NewService(service nivek.NivekService) NivekQuoteService {
	return &nivekQuoteServiceImpl{
		quoteTable: service.Postgres().GetDefaultConnection().Collection(TableQuote),
	}
}

func (s *nivekQuoteServiceImpl) Random(channelTwitchId string) (Quote, bool, error) {
	channelTwitchId = strings.TrimSpace(channelTwitchId)
	if channelTwitchId == "" {
		return Quote{}, false, ErrInvalid
	}
	var q Quote
	err := s.quoteTable.Find(db.Cond{"channel_twitch_id": channelTwitchId}).
		OrderBy(db.Raw("random()")).
		Limit(1).
		One(&q)
	if err != nil {
		if errors.Is(err, db.ErrNoMoreRows) {
			return Quote{}, false, nil
		}
		return Quote{}, false, fmt.Errorf("error fetching random quote for %s: %w", channelTwitchId, err)
	}
	return q, true, nil
}

func (s *nivekQuoteServiceImpl) Get(channelTwitchId string, number int) (Quote, bool, error) {
	channelTwitchId = strings.TrimSpace(channelTwitchId)
	if channelTwitchId == "" || number <= 0 {
		return Quote{}, false, ErrInvalid
	}
	var q Quote
	err := s.quoteTable.Find(db.Cond{
		"channel_twitch_id": channelTwitchId,
		"quote_number":      number,
	}).One(&q)
	if err != nil {
		if errors.Is(err, db.ErrNoMoreRows) {
			return Quote{}, false, nil
		}
		return Quote{}, false, fmt.Errorf("error fetching quote #%d for %s: %w", number, channelTwitchId, err)
	}
	return q, true, nil
}

func (s *nivekQuoteServiceImpl) Add(channelTwitchId, text, addedBy string, game *string) (Quote, error) {
	channelTwitchId = strings.TrimSpace(channelTwitchId)
	text, ok := ValidText(text)
	if channelTwitchId == "" || !ok {
		return Quote{}, ErrInvalid
	}
	addedBy = strings.ToLower(strings.TrimSpace(addedBy))
	if game != nil {
		g := Clean(*game)
		if g == "" {
			game = nil
		} else {
			game = &g
		}
	}

	// Assign max+1 in the same statement. A concurrent add can take the same
	// number first; ON CONFLICT DO NOTHING then returns no row and we retry.
	const query = `
        INSERT INTO quote (channel_twitch_id, quote_number, text, added_by, game)
        SELECT $1, COALESCE(MAX(quote_number), 0) + 1, $2, $3, $4
          FROM quote
         WHERE channel_twitch_id = $1
        ON CONFLICT (channel_twitch_id, quote_number) DO NOTHING
        RETURNING id, quote_number, created_at
    `
	for range addRetries {
		row, err := s.quoteTable.Session().SQL().QueryRow(query, channelTwitchId, text, addedBy, game)
		if err != nil {
			return Quote{}, fmt.Errorf("error adding quote for %s: %w", channelTwitchId, err)
		}
		q := Quote{
			ChannelTwitchId: channelTwitchId,
			Text:            text,
			AddedBy:         addedBy,
			Game:            game,
		}
		if err := row.Scan(&q.Id, &q.QuoteNumber, &q.CreatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return Quote{}, fmt.Errorf("error adding quote for %s: %w", channelTwitchId, err)
		}
		return q, nil
	}
	return Quote{}, fmt.Errorf("error adding quote for %s: quote number contention", channelTwitchId)
}

func (s *nivekQuoteServiceImpl) Delete(channelTwitchId string, number int) (bool, error) {
	channelTwitchId = strings.TrimSpace(channelTwitchId)
	if channelTwitchId == "" || number <= 0 {
		return false, ErrInvalid
	}
	res := s.quoteTable.Find(db.Cond{
		"channel_twitch_id": channelTwitchId,
		"quote_number":      number,
	})
	n, err := res.Count()
	if err != nil {
		return false, fmt.Errorf("error looking up quote #%d for %s: %w", number, channelTwitchId, err)
	}
	if n == 0 {
		return false, nil
	}
	if err := res.Delete(); err != nil {
		return false, fmt.Errorf("error deleting quote #%d for %s: %w", number, channelTwitchId, err)
	}
	return true, nil
}
