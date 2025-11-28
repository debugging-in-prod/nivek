package twitchbot

import (
	"fmt"
	"log"
	"math/rand"
)

type FishScore struct {
	Score       int    `json:"score"`
	Fish        []fish `json:"fish"`
	TrashCaught int    `json:"trash_caught"`
	TimesFished int    `json:"times_fished"`
}

type fish struct {
	Value    int    `json:"value"`
	Name     string `json:"name"`
	Scarcity int    `json:"scarcity"`
}

func (b *Bot) goFishing() (*fish, bool, string) {
	// Seed random (do this once in bot initialization, not here)
	// rand.Seed(time.Now().UnixNano())

	fishes := b.initFish()

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
		return nil, false, "🎣 Nothing bit... Better luck next time!"
	}
	roll -= nothingWeight

	// Check if caught trash
	if roll < trashWeight {
		return nil, true, "🗑️ You caught an old boot! At least it's not nothing?"
	}
	roll -= trashWeight

	// Check which fish was caught
	currentWeight := 0
	for _, fsh := range fishes {
		fishWeight := 100 / fsh.Scarcity
		currentWeight += fishWeight

		if roll < currentWeight {
			return &fsh, false, fmt.Sprintf("🎣 You caught a %s worth %d points!", fsh.Name, fsh.Value)
		}
	}

	return nil, false, "something went wrong"
}

func (b *Bot) initFish() []fish {
	return []fish{
		{Value: 10, Name: "Trout", Scarcity: 1},
		{Value: 25, Name: "Redfish", Scarcity: 10},
		{Value: 50, Name: "Snook", Scarcity: 100},
	}
}
