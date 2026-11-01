package promo

import (
	"fmt"
	"strings"

	promoSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/promo"
	userlib "github.com/tim-the-toolman-taylor/nivek/internal/libraries/user"
)

// channelLogin resolves the promo owner key from the logged-in user. Promos are
// keyed by the lowercased Twitch login — the same value the bot posts to
// (message.Channel) — NOT the legacy `username` column. A user who has never
// linked a Twitch identity has no channel the bot can post in, so that's an error.
func channelLogin(u *userlib.User) (string, error) {
	if u == nil || u.TwitchLogin == nil || strings.TrimSpace(*u.TwitchLogin) == "" {
		return "", fmt.Errorf("no linked twitch account")
	}
	return strings.ToLower(*u.TwitchLogin), nil
}

// newPromoService builds the promo service. It is a var so tests can swap in a
// fake.
var newPromoService = promoSvc.NewService
