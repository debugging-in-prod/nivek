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
	GetUserFishScore() (*FishScore, error)
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

func (s *nivekFishingServiceImpl) GetUserFishScore() (*FishScore, error) {
	var fishScore FishScore

	if err := s.fishingTable.Find(db.Cond{
		"channelname": s.channel,
	}).One(&fishScore); err != nil {
		return nil, err
	}

	return &fishScore, nil
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

	return fmt.Sprintf(
		"%s You've caught %d fish, and %d trash. Your total score is %d",
		msg,
		len(fishScore.Fish),
		fishScore.TrashCaught,
		fishScore.Score,
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
			newFishScore := FishScore{
				ChannelName: s.channel,
				ChatterName: chatter,
				Fish:        FishArray{}, // Empty array
			}

			// Insert the new record
			if id, err := s.fishingTable.Insert(newFishScore); err != nil {
				return nil, fmt.Errorf("failed to create fish score record: %w", err)
			} else {
				newFishScore.ID = int(id.ID().(int64))
			}

			// Return the newly created record
			return &newFishScore, nil
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
