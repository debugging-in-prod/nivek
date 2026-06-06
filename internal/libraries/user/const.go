package user

const TableUser = "users"

type User struct {
	Id                int    `db:"id" json:"id"`
	Username          string `db:"username" json:"username"`
	Email             string `db:"email,omitempty" json:"email,omitempty"`
	Password          string `db:"password,omitempty" json:"-"`
	TwitchID          string `db:"twitch_id" json:"twitch_id"`
	TwitchLogin       string `db:"twitch_login" json:"twitch_login"`
	TwitchDisplayName string `db:"twitch_display_name" json:"twitch_display_name"`
	CreatedAt         string `db:"created_at" json:"created_at"`
}
