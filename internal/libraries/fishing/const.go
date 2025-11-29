package fishing

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const TableFishing = "fish_score"

type FishScore struct {
	ID          int       `gorm:"primaryKey"`
	ChannelName string    `gorm:"not null;index:idx_channel_chatter,unique"`
	ChatterName string    `gorm:"not null;index:idx_channel_chatter,unique"`
	Score       int       `gorm:"not null;default:0"`
	Fish        FishArray `gorm:"type:jsonb;not null;default:'[]'"`
	TrashCaught int       `gorm:"not null;default:0"`
	TimesFished int       `gorm:"not null;default:0"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
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
