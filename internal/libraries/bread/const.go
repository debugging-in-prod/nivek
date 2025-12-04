package bread

import "time"

const TableBread = "bread"

type Bread struct {
	ID          int       `db:"id" json:"id"`
	ChannelName string    `db:"channelname" json:"channelname"`
	ChatterName string    `db:"chattername" json:"chattername"`
	Count       int       `db:"bread_count" json:"bread_count"`
	UpdatedAt   time.Time `db:"updated_at" json:"updated_at"`
	CreatedAt   time.Time `db:"created_at" json:"created_at"`
}
