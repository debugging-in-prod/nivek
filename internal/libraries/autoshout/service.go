package autoshout

import (
	"fmt"
	"log"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/upper/db/v4"
)

type NivekAutoShoutService interface {
	OnMessage(channel, chatter string) bool
	GetAllAutoShoutChatters() ([]ShoutChatter, error)
	GetAutoShoutChatters(channelname string) ([]ShoutChatter, error)
	GetAutoShoutChatter(channelname, chattername string) (*ShoutChatter, error)
	CreateAutoShoutChatter(channelname, chattername string) (int, error)
	UpdateAutoShoutChatter(chatter *ShoutChatter) error
	DeleteAutoShoutChatter(channelname string, chattername int) error
}

type nivekAutoShoutServiceImpl struct {
	nivek      nivek.NivekService
	shoutTable db.Collection
	chatters   map[string]map[string]interface{}
}

func NewService(service nivek.NivekService) NivekAutoShoutService {
	svcImpl := &nivekAutoShoutServiceImpl{
		nivek:      service,
		shoutTable: service.Postgres().GetDefaultConnection().Collection(TableShout),
	}

	svcImpl.init()

	return &nivekAutoShoutServiceImpl{
		nivek:      service,
		shoutTable: service.Postgres().GetDefaultConnection().Collection(TableShout),
	}
}

func (s *nivekAutoShoutServiceImpl) OnMessage(channel, chatter string) bool {

	log.Println(channel)
	log.Println(s.chatters)
	log.Println(s.chatters[channel])

	if _, channelExists := s.chatters[channel]; channelExists {
		if _, chatterExists := s.chatters[channel][chatter]; chatterExists {
			s.incrementShoutCount(channel, chatter)
			delete(s.chatters[channel], chatter)
			return true
		}
	}

	return false
}

func (s *nivekAutoShoutServiceImpl) init() {
	shoutChatters, err := s.GetAllAutoShoutChatters()
	if err != nil {
		log.Printf("[AutoShout] failed to get all auto shouts: %s", err.Error())
	}

	log.Printf("[AutoShout] got all auto shouts: %s", shoutChatters[0].ChatterName)

	s.chatters = formatAutoShoutChatters(shoutChatters)
}

func (s *nivekAutoShoutServiceImpl) GetAllAutoShoutChatters() ([]ShoutChatter, error) {
	var chatters []ShoutChatter

	if err := s.shoutTable.Find().All(&chatters); err != nil {
		return nil, fmt.Errorf("[AutoShout] error fetching all auto shout chatters %s", err.Error())
	}

	for _, chatter := range chatters {
		log.Printf("[AutoShout] chatter found: %s", chatter.ChatterName)
	}

	return chatters, nil
}

func (s *nivekAutoShoutServiceImpl) GetAutoShoutChatters(channelname string) ([]ShoutChatter, error) {
	var chatters []ShoutChatter

	if err := s.shoutTable.Find(db.Cond{"channelname": channelname}).All(&chatters); err != nil {
		return nil, fmt.Errorf("[AutoShout] error fetching auto shout chatters for channel %s - %s", channelname, err.Error())
	}

	return chatters, nil
}

func (s *nivekAutoShoutServiceImpl) GetAutoShoutChatter(channelname, chattername string) (*ShoutChatter, error) {
	var chatter ShoutChatter

	if err := s.shoutTable.Find(db.Cond{
		"channelname": channelname,
		"chattername": chattername,
	}).One(&chatter); err != nil {
		return nil, fmt.Errorf("[AutoShout] error fetching auto shout chatter for channel %s chatter %s - %s",
			channelname, chattername, err.Error(),
		)
	}

	return &chatter, nil
}

func (s *nivekAutoShoutServiceImpl) CreateAutoShoutChatter(channelname, chattername string) (int, error) {
	result, err := s.shoutTable.Insert(db.Cond{"channelname": channelname, "chattername": chattername})
	if err != nil {
		return 0, fmt.Errorf(
			"[AutoShout] error creating auto shout chatter record for channel %s chatter %s - %s",
			channelname,
			chattername,
			err.Error(),
		)
	}

	insertedID, ok := result.ID().(int64)
	if !ok {
		return 0, fmt.Errorf("[AutoShout] failed to get inserted ID")
	}

	return int(insertedID), nil
}

func (s *nivekAutoShoutServiceImpl) UpdateAutoShoutChatter(chatter *ShoutChatter) error {
	if err := s.shoutTable.UpdateReturning(chatter); err != nil {
		return fmt.Errorf("[AutoShout] error updating shout chatter record for channel %s chatter %s - %s", chatter.ChannelName, chatter.ChatterName, err.Error())
	}
	return nil
}

func (s *nivekAutoShoutServiceImpl) DeleteAutoShoutChatter(channelname string, id int) error {
	if err := s.shoutTable.Find(db.Cond{"channelname": channelname, "id": id}).Delete(); err != nil {
		return fmt.Errorf(
			"[AutoShout] error deleting auto shout chatter record for channel %s chatter id %d - %s",
			channelname,
			id,
			err.Error(),
		)
	}

	return nil
}

func (s *nivekAutoShoutServiceImpl) incrementShoutCount(channel, chatter string) {
	chatterRecord, err := s.GetAutoShoutChatter(channel, chatter)
	if err != nil {
		log.Printf("[AutoShout] failed to increment chatter score! %s", err.Error())
		return
	}

	chatterRecord.ShoutCount++

	err = s.UpdateAutoShoutChatter(chatterRecord)
	if err != nil {
		log.Printf("[AutoShout] failed to save incremented chatter score to the db! %s", err.Error())
		return
	}
}

func formatAutoShoutChatters(shoutChatters []ShoutChatter) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})

	for _, chatter := range shoutChatters {
		if _, exists := result[chatter.ChannelName]; !exists {
			result[chatter.ChannelName] = make(map[string]interface{})
		}

		result[chatter.ChannelName][chatter.ChatterName] = nil
	}

	return result
}
