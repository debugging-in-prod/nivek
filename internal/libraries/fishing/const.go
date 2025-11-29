package fishing

const TableFishing = "fish_score"

type FishScore struct {
	Id          int    `json:"id"`
	ChannelName string `json:"channelname"`
	ChatterName string `json:"chattername"`
	Score       int    `json:"score"`
	Fish        []Fish `json:"fish"`
	TrashCaught int    `json:"trash_caught"`
	TimesFished int    `json:"times_fished"`
}

type Fish struct {
	Value    int    `json:"value"`
	Name     string `json:"name"`
	Scarcity int    `json:"scarcity"`
}

// rather than hardcode in static db table, just leave in code where it's easier to find
func (s *nivekFishingServiceImpl) initFish() []Fish {
	return []Fish{
		{Value: 10, Name: "Trout", Scarcity: 1},
		{Value: 25, Name: "Redfish", Scarcity: 10},
		{Value: 50, Name: "Snook", Scarcity: 100},
	}
}
