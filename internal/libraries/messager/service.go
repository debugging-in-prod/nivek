package messager

import (
	"fmt"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/upper/db/v4"
)

type NivekMessagerService interface {
	CreateMessage(message Message) error
	GetMessages() ([]Message, error)
}

type nivekMessagerServiceImpl struct {
	nivek        nivek.NivekService
	messageTable db.Collection
}

func NewService(service nivek.NivekService) NivekMessagerService {
	return &nivekMessagerServiceImpl{
		nivek:        service,
		messageTable: service.Postgres().GetDefaultConnection().Collection(TableMessages),
	}
}

func (s *nivekMessagerServiceImpl) CreateMessage(newMessage Message) error {
	if err := s.messageTable.InsertReturning(newMessage); err != nil {
		return fmt.Errorf("failed to insert new message in db: %w", err)
	}

	return nil
}

func (s *nivekMessagerServiceImpl) GetMessages() ([]Message, error) {
	var messages []Message

	if err := s.messageTable.Find().All(&messages); err != nil {
		return nil, fmt.Errorf("[Messenger] failed to fetch all messages: %w", err)
	}

	return messages, nil
}
