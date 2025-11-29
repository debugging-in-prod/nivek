package fishing

const TableFishing = "fish_score"

type FishScore struct {
	Id          int    `db:"id" json:"id"`
	ChannelName string `db:"channelname" json:"channelname"`
	ChatterName string `db:"chattername" json:"chattername"`
	Score       int    `db:"score" json:"score"`
	Fish        []Fish `db:"fish" json:"fish"`
	TrashCaught int    `db:"trash_caught" json:"trash_caught"`
	TimesFished int    `db:"times_fished" json:"times_fished"`
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
