package autoshout

import (
	"fmt"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/upper/db/v4"
)

type NivekAutoShoutService interface {
	GetAutoShoutChatters(channelname string) ([]ShoutChatter, error)
	GetAutoShoutChatter(channelname, chattername string) (*ShoutChatter, error)
	CreateAutoShoutChatter(channelname, chattername string) (int, error)
	UpdateAutoShoutChatter(chatter *ShoutChatter) error
	DeleteAutoShoutChatter(channelname string, chattername int) error
}

type nivekAutoShoutServiceImpl struct {
	nivek      nivek.NivekService
	shoutTable db.Collection
}

func NewService(service nivek.NivekService) NivekAutoShoutService {
	return &nivekAutoShoutServiceImpl{
		nivek:      service,
		shoutTable: service.Postgres().GetDefaultConnection().Collection(TableShout),
	}
}

func (s *nivekAutoShoutServiceImpl) GetAutoShoutChatters(channelname string) ([]ShoutChatter, error) {
	var chatters []ShoutChatter

	if err := s.shoutTable.Find(db.Cond{"channelname": channelname}).All(&chatters); err != nil {
		return nil, fmt.Errorf("error fetching auto shout chatters for channel %s - %s", channelname, err.Error())
	}

	return chatters, nil
}

func (s *nivekAutoShoutServiceImpl) GetAutoShoutChatter(channelname, chattername string) (*ShoutChatter, error) {
	var chatter ShoutChatter

	if err := s.shoutTable.Find(db.Cond{
		"channelname": channelname,
		"chattername": chattername,
	}).One(&chatter); err != nil {
		return nil, fmt.Errorf("error fetching auto shout chatter for channel %s chatter %s - %s",
			channelname, chattername, err.Error(),
		)
	}

	return &chatter, nil
}

func (s *nivekAutoShoutServiceImpl) CreateAutoShoutChatter(channelname, chattername string) (int, error) {
	insertID, err := s.shoutTable.Insert(db.Cond{"channelname": channelname, "chattername": chattername})
	if err != nil {
		return 0, fmt.Errorf(
			"error creating auto shout chatter record for channel %s chatter %s - %s",
			channelname,
			chattername,
			err.Error(),
		)
	}

	return insertID.ID().(int), nil
}

func (s *nivekAutoShoutServiceImpl) UpdateAutoShoutChatter(chatter *ShoutChatter) error {
	if err := s.shoutTable.UpdateReturning(chatter); err != nil {
		return fmt.Errorf("error updating shout chatter record for channel %s chatter %s - %s", chatter.ChannelName, chatter.ChatterName, err.Error())
	}
	return nil
}

func (s *nivekAutoShoutServiceImpl) DeleteAutoShoutChatter(channelname string, id int) error {
	if err := s.shoutTable.Find(db.Cond{"channelname": channelname, "id": id}).Delete(); err != nil {
		return fmt.Errorf(
			"error deleting auto shout chatter record for channel %s chatter id %d - %s",
			channelname,
			id,
			err.Error(),
		)
	}

	return nil
}
