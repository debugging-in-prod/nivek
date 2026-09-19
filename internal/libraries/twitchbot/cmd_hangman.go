package twitchbot

import (
	"fmt"
	"io"
	"strings"
)

type Hangman struct {
	Games map[string]Game // map[channel]Game
}

type Game struct {
	Word    string
	Guesses []string
}

const hangmanMaxWrong = 6
var hangmanFaces = [hangmanMaxWrong + 1]string{
	"😀", "😧", "😨", "😰", "😱", "😵", "💀", // index 6 = dead / game over
}

func (b *Bot) handleHangmanCommand(message *chatMessageEvent) {
	channel := message.BroadcasterUserLogin
//	channelId := message.BroadcasterUserId

	hangman := Hangman{
		Games: make(map[string]Game),
	}

	raw := strings.TrimSpace(message.Message.Text)
	args := ""
	if idx := strings.IndexAny(raw, " \t"); idx != -1 {
		args = strings.TrimSpace(raw[idx+1:])
	}
	fields := strings.Fields(args)

	if len(fields) == 0 {
		if game, ok := hangman.Games[channel]; ok {

			// resume existing game
			fmt.Printf("[HANGMAN] game found! %+v\n", game)

		} else {

			// new game

			newgame := Game{
				Word:    "",
				Guesses: []string{},
			}

			// get word
			resp, err := b.httpClient.Get("https://random-word-api.herokuapp.com/word")
			if err != nil {
				fmt.Printf("[HANGMAN] failed to fetch word! %s\n", err.Error())
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)

			fmt.Printf("[HANGMAN] word generated for new game: %s\n", string(body))

			newgame.Word = string(body)
		}
	} else {

		// handle guess
		switch strings.ToLower(fields[0]) {
		default:
			fmt.Println("hangman default switchcase")
		}
	}
}
