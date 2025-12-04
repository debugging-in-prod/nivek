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
	dbConditions := db.Cond{"channelname": channel, "chattername": chatter}
	if err := s.breadTable.Find(dbConditions).
		Update(db.Raw("bread_count = bread_count + 1")); err != nil {
		return 0, fmt.Errorf("error incrementing bread count: %v", err)
	}

	// Fetch the updated count
	var bread Bread
	if err := s.breadTable.Find(dbConditions).One(&bread); err != nil {
		return 0, fmt.Errorf("error getting updated bread count: %v", err)
	}

	return bread.Count, nil
}
