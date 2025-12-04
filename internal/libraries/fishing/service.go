package fishing

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/labstack/gommon/log"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/upper/db/v4"
)

type NivekFishingService interface {
	GoFishing(chatter string) string
	GetChannelFishScore() ([]FishScore, error)
	GetUserFishScore() ([]FishScore, error)
}

type nivekFishingServiceImpl struct {
	channel      string
	nivek        nivek.NivekService
	fishingTable db.Collection
}

func NewService(service nivek.NivekService, channel string) NivekFishingService {
	return &nivekFishingServiceImpl{
		channel:      channel,
		nivek:        service,
		fishingTable: service.Postgres().GetDefaultConnection().Collection(TableFishing),
	}
}

// GetChannelFishScore gets score of every chatter who has fished in this channel
func (s *nivekFishingServiceImpl) GetChannelFishScore() ([]FishScore, error) {
	var fishScore []FishScore

	err := s.fishingTable.
		Find(db.Cond{
			"channelname":    s.channel,
			"chattername !=": s.channel,
		}).
		OrderBy("-score"). // -score = descending (highest first)
		All(&fishScore)

	if err != nil {
		return nil, err
	}

	return fishScore, nil
}

// GetUserFishScore gets this user's score from every chat they have fished in
func (s *nivekFishingServiceImpl) GetUserFishScore() ([]FishScore, error) {
	var fishScore []FishScore

	if err := s.fishingTable.Find(db.Cond{
		"chattername": s.channel,
	}).All(&fishScore); err != nil {
		return nil, err
	}

	return fishScore, nil
}

func (s *nivekFishingServiceImpl) GoFishing(chatter string) string {
	fishScore, err := s.getChatterFishScore(chatter)
	if err != nil {
		log.Errorf("error fetching fish score: %s", err.Error())
		return "error fetching fish score"
	}

	// increment times fished
	fishScore.TimesFished++

	// prepare response message
	var msg string

	// calculate fish caught, trash caught, or nothing caught
	fsh, trashCaught, nothingCaught := s.rollForFish()
	if fsh != nil {
		fishScore.Fish = append(fishScore.Fish, *fsh)
		fishScore.Score = fishScore.Score + fsh.Value
		msg = fmt.Sprintf("🎣 You caught a %s worth %d points!", fsh.Name, fsh.Value)
	}

	if trashCaught {
		fishScore.TrashCaught++
		msg = "🗑️ You caught an old boot! At least it's not nothing?"
	}

	if nothingCaught {
		msg = "🎣 Nothing bit... Better luck next time!"
	}

	if fsh == nil && !trashCaught && !nothingCaught {
		msg = "something went wrong"
	}

	if errUpdate := s.updateScore(fishScore); errUpdate != nil {
		log.Errorf("error updating fish score: %s", err)
	}

	totalScoreAllChats := s.getChatterFishScoreAcrossAllChats(chatter)

	return fmt.Sprintf(
		"%s You've caught %d fish, and %d trash. Your total score for this chat is %d and your total score across all chats is %d",
		msg,
		len(fishScore.Fish),
		fishScore.TrashCaught,
		fishScore.Score,
		totalScoreAllChats,
	)
}

// returns: fishCaught, trashCaught, nothingCaught, error
func (s *nivekFishingServiceImpl) rollForFish() (*Fish, bool, bool) {
	// fetch hardcoded fish
	fishes := s.initFish()

	// Calculate total weight (inverse of scarcity)
	// Trout: 100, Redfish: 10, Snook: 1 = 111 total weight
	totalWeight := 0
	for _, f := range fishes {
		totalWeight += 100 / f.Scarcity
	}

	// Add weight for catching nothing (40% chance)
	nothingWeight := 50
	// Add weight for catching trash (20% chance)
	trashWeight := 25

	totalWeight += nothingWeight + trashWeight

	// Roll the dice
	roll := rand.Intn(totalWeight)

	// Check if caught nothing
	if roll < nothingWeight {
		return nil, false, true
	}
	roll -= nothingWeight

	// Check if caught trash
	if roll < trashWeight {
		return nil, true, false
	}
	roll -= trashWeight

	// Check which Fish was caught
	currentWeight := 0
	for _, fsh := range fishes {
		fishWeight := 100 / fsh.Scarcity
		currentWeight += fishWeight

		if roll < currentWeight {
			return &fsh, false, false
		}
	}

	return nil, false, false
}

func (s *nivekFishingServiceImpl) getChatterFishScoreAcrossAllChats(chatter string) int {
	var result struct {
		Total int `db:"total"`
	}

	err := s.fishingTable.Find(db.Cond{"chattername": chatter}).
		Select(db.Raw("COALESCE(SUM(score), 0) AS total")).
		One(&result)

	if err != nil {
		log.Errorf("error getting fish score for chatter %s: %s", chatter, err.Error())
		return 0
	}

	return result.Total
}

func (s *nivekFishingServiceImpl) getChatterFishScore(chatter string) (*FishScore, error) {
	var fishScore FishScore

	err := s.fishingTable.Find(db.Cond{
		"channelname": s.channel,
		"chattername": chatter,
	}).One(&fishScore)

	if err != nil {
		// Check if error is "not found"
		if errors.Is(err, db.ErrNoMoreRows) {
			// Record doesn't exist - create it
			newFishScore := map[string]any{
				"channelname": s.channel,
				"chattername": chatter,
				"fish":        FishArray{},
			}

			// Insert the new record
			result, errInsert := s.fishingTable.Insert(newFishScore)
			if errInsert != nil {
				return nil, fmt.Errorf("failed to create fish score record: %w", err)
			}

			// Get the auto-generated ID
			insertedID, ok := result.ID().(int64)
			if !ok {
				return nil, fmt.Errorf("failed to get inserted ID")
			}

			fishScoreReturn := FishScore{
				ID:          int(insertedID),
				ChannelName: s.channel,
				ChatterName: chatter,
				Fish:        FishArray{},
			}

			// Return the newly created record
			return &fishScoreReturn, nil
		}

		// Some other error occurred
		return nil, fmt.Errorf("failed to find fish score record: %w", err)
	}

	return &fishScore, nil
}

func (s *nivekFishingServiceImpl) updateScore(fishScore *FishScore) error {
	if err := s.fishingTable.UpdateReturning(fishScore); err != nil {
		return fmt.Errorf("failed to save updated fish score: %w", err)
	}

	return nil
}
