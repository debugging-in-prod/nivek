package fishing

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const TableFishing = "fish_score"

type FishScore struct {
	ID          int       `db:"id" json:"id"`
	ChannelName string    `db:"channelname" json:"channelname"`
	ChatterName string    `db:"chattername" json:"chattername"`
	Score       int       `db:"score" json:"score"`
	Fish        FishArray `db:"fish" json:"fish"`
	TrashCaught int       `db:"trash_caught" json:"trash_caught"`
	TimesFished int       `db:"times_fished" json:"times_fished"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
}

// FishArray Custom type for Fish array in FishScore record
type FishArray []Fish

func (fa FishArray) Value() (driver.Value, error) {
	if len(fa) == 0 {
		return []byte(`[]`), nil
	}
	return json.Marshal(fa)
}

func (fa *FishArray) Scan(value interface{}) error {
	if value == nil {
		*fa = FishArray{}
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("unsupported type: %T", value)
	}

	return json.Unmarshal(bytes, fa)
}

type Fish struct {
	Value    int    `db:"value" json:"value"`
	Name     string `db:"name" json:"name"`
	Scarcity int    `db:"scarcity" json:"scarcity"`
}

// rather than hardcode in static db table, just leave in code where it's easier to find
func (s *nivekFishingServiceImpl) initFish() []Fish {
	return []Fish{
		{Value: 10, Name: "Trout", Scarcity: 1},
		{Value: 25, Name: "Redfish", Scarcity: 10},
		{Value: 50, Name: "Snook", Scarcity: 100},
	}
}
