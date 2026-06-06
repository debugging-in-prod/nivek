package user

const TableUser = "users"

// Twitch fields are *string (not string) because NULL is the correct value for
// legacy rows that pre-date the Twitch OAuth migration — those users exist but
// have never linked a Twitch identity, and `NULL` says that more honestly than
// `""` would. Postgres's column-level UNIQUE on twitch_id treats each NULL as
// distinct, so any number of legacy rows coexist while OAuth rows are still
// uniquely keyed.
type User struct {
	Id                int     `db:"id" json:"id"`
	Username          string  `db:"username" json:"username"`
	Email             string  `db:"email,omitempty" json:"email,omitempty"`
	Password          string  `db:"password,omitempty" json:"-"`
	TwitchID          *string `db:"twitch_id" json:"twitch_id"`
	TwitchLogin       *string `db:"twitch_login" json:"twitch_login"`
	TwitchDisplayName *string `db:"twitch_display_name" json:"twitch_display_name"`
	CreatedAt         string  `db:"created_at" json:"created_at"`
}
