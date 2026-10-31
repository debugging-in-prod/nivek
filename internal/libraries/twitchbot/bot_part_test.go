package twitchbot

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func servePart(t *testing.T, bot *Bot, body string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/internal/part", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	if err := newPartChannelEndpoint(bot)(echo.New().NewContext(req, rec)); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return rec.Code
}

func TestPartEndpointRejectsMissingLogin(t *testing.T) {
	if code := servePart(t, &Bot{}, `{}`); code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400", code)
	}
}

// The creator's and bot's own channels are never left; the handler must refuse
// before touching the IRC client (nil here, so reaching it would panic).
func TestPartEndpointRefusesPermanentChannels(t *testing.T) {
	bot := &Bot{config: Config{BotUsername: "PeanutBudderBot"}}
	for _, login := range []string{botCreatorChannel, "peanutbudderbot", "TimAllenFanClubOfficial"} {
		if code := servePart(t, bot, `{"twitch_login":"`+login+`"}`); code != http.StatusConflict {
			t.Fatalf("%s: got %d, want 409", login, code)
		}
	}
}

func TestUntrackChannelRemovesOnlyTarget(t *testing.T) {
	a, b := "alice", "bob"
	bot := &Bot{}
	bot.config.Channels = append(bot.config.Channels, channelWithLogin(a), channelWithLogin(b))
	bot.untrackChannel(a)
	if bot.isTrackedChannel(a) || !bot.isTrackedChannel(b) {
		t.Fatalf("untrackChannel(%q) left channels %+v", a, bot.config.Channels)
	}
}

func channelWithLogin(login string) BotUser {
	var u BotUser
	u.TwitchLogin = &login
	return u
}
