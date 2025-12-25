package lurk

import "time"

const TableLurk = "lurk"

type Lurker struct {
	Id          int       `db:"id" json:"id"`
	ChannelName string    `db:"channelname" json:"channelname"`
	ChatterName string    `db:"chattername" json:"chattername"`
	LurkCount   int       `db:"lurk_count" json:"lurk_count"`
	CreatedAt   time.Time `db:"created" json:"created_at"`
	UpdatedAt   time.Time `db:"updated" json:"updated_at"`
}
