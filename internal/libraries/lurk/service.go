package lurk

import (
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/upper/db/v4"
)

type NivekLurkService interface {
	OnMessage(channel, chatter string) bool
}

type nivekLurkServiceImpl struct {
	nivek     nivek.NivekService
	lurkTable db.Collection
}

func NewService(service nivek.NivekService) NivekLurkService {
	return &nivekLurkServiceImpl{
		nivek:     service,
		lurkTable: service.Postgres().GetDefaultConnection().Collection(TableLurk),
	}
}

func (s *nivekLurkServiceImpl) OnMessage(channe, chatter string) bool {
	return true
}
