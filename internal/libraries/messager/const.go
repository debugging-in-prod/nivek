package messager

import "time"

const TableMessages = "message"

type Message struct {
	Id        int       `db:"id" json:"id"`
	Sender    string    `db:"sender" json:"sender"`
	Message   string    `db:"message" json:"message"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
