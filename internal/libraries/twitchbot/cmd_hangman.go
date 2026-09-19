package twitchbot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Hangman struct {
	Games map[string]*Game // map[channel]Game
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
	channelId := message.BroadcasterUserId

	hangman := Hangman{
		Games: make(map[string]*Game),
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
			b.say(channelId, printstate(game))
		} else {

			// new game
			newgame := startNewGame(b.httpClient)
			if newgame == nil {
				b.say(channelId, "whoops! Failed to start new game")
			} else {
				b.say(channelId, "New game started!")
				b.say(channelId, printstate(newgame))
				hangman.Games[channel] = newgame
			}
		}
	} else {

		// handle guess
		switch strings.ToLower(fields[0]) {
		default:
			fmt.Println("hangman default switchcase")
		}
	}
}

func printstate(game *Game) string {
	guessed := make(map[rune]bool, len(game.Guesses))
	for _, g := range game.Guesses {
		for _, r := range strings.ToLower(g) {
			guessed[r] = true
		}
	}

	slots := make([]string, 0, len(game.Word))
	for _, r := range game.Word {
		if guessed[r] {
			slots = append(slots, string(r))
		} else {
			slots = append(slots, "_")
		}
	}

	var misses []string
	for _, g := range game.Guesses {
		lg := strings.ToLower(g)
		if len(lg) == 1 && !strings.ContainsAny(game.Word, lg) {
			misses = append(misses, lg)
		}
	}
	wrong := min(len(game.Guesses), hangmanMaxWrong)

	line := fmt.Sprintf("%s  %s", hangmanFaces[wrong], strings.Join(slots, " "))
	if len(misses) > 0 {
		line += "   ❌ " + strings.Join(misses, " ")
	}
	line += fmt.Sprintf(" (%d/%d)", wrong, hangmanMaxWrong)

	return line
}

func startNewGame(httpClient *http.Client) *Game {
	newgame := &Game{
		Word:    "",
		Guesses: []string{},
	}

	// get word
	resp, err := httpClient.Get("https://random-word-api.herokuapp.com/word")
	if err != nil {
		fmt.Printf("[HANGMAN] failed to fetch word! %s\n", err.Error())
		return nil
	}
	defer resp.Body.Close()

	var words []string
	if err := json.NewDecoder(resp.Body).Decode(&words); err != nil {
		fmt.Printf("[HANGMAN] failed to decode new-word response: %s\n", err.Error())
		return nil
	}

	if len(words) == 0 {
		fmt.Println("[HANGMAN] word API returned an empty array")
		return nil
	}

	fmt.Printf("[HANGMAN] word generated for new game: %s\n", words[0])

	newgame.Word = words[0]
	return newgame
}
