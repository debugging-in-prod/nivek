package bot

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/hangman"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

const TableHangman = "hangman"

type hangmanRequest struct {
	Game    hangman.Game `json:"game"`
	Channel string       `json:"channel"`
}

func NewPostHangmanGameState(nivekSvc nivek.NivekService) echo.HandlerFunc {
	table := nivekSvc.Postgres().GetDefaultConnection().Collection(TableHangman)
	return func(c echo.Context) error {
		var req hangmanRequest
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("failed to decode body: %s", err.Error())})
		}

		const query = `
			INSERT INTO nivek.hangman (channelname, word, guesses)
			VALUES ($1, $2, $3)
			ON CONFLICT (channelname) DO UPDATE
			SET channelname = $1,
			word = $2,
			guesses = $3
		`

		if _, err := table.Session().SQL().Exec(query, req.Channel, req.Game.Word, strings.Join(req.Game.Guesses, ",")); err != nil {
			return fmt.Errorf("failed to upsert hangman record for channel %s - %s", req.Channel, err.Error())
		}

		return nil
	}
}
