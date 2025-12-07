package bread

import (
	"fmt"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/upper/db/v4"
)

type NivekBreadService interface {
	IncrementCount(channel, chatter string) (int, error)
	GetTotalBreadForChannel(channel string) (int, error)
}

type nivekBreadServiceImpl struct {
	nivek.NivekService
	breadTable db.Collection
}

func NewService(niveksvc nivek.NivekService) NivekBreadService {
	return &nivekBreadServiceImpl{
		niveksvc,
		niveksvc.Postgres().GetDefaultConnection().Collection(TableBread),
	}
}

func (s *nivekBreadServiceImpl) GetTotalBreadForChannel(channel string) (int, error) {
	var result struct {
		Total int `db:"total"`
	}

	err := s.breadTable.Find(db.Cond{"channelname": channel}).
		Select(db.Raw("COALESCE(SUM(bread_count), 0) AS total")).
		One(&result)

	if err != nil {
		return 0, fmt.Errorf("error getting bread for channel %s: %s", channel, err.Error())
	}

	return result.Total, nil
}

func (s *nivekBreadServiceImpl) IncrementCount(channel, chatter string) (int, error) {
	var result struct {
		Count int `db:"bread_count"`
	}

	query := `
        INSERT INTO bread (channelname, chattername, bread_count, created_at, updated_at)
        VALUES ($1, $2, 1, NOW(), NOW())
        ON CONFLICT (channelname, chattername)
        DO UPDATE SET 
            bread_count = bread.bread_count + 1,
            updated_at = NOW()
        RETURNING bread_count
    `

	res, err := s.breadTable.Session().SQL().
		QueryRow(query, channel, chatter)

	if err != nil {
		return 0, fmt.Errorf("error upserting bread count: %v", err)
	}

	if errScan := res.Scan(&result.Count); errScan != nil {
		return 0, fmt.Errorf("error formatting updated bread count: %v", err)
	}

	return result.Count, nil
}
