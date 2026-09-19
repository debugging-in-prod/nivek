package hangman

type Hangman struct {
	Games map[string]*Game // map[channel]Game
}

type Game struct {
	Word     string
	Guesses  []string
	GameOver bool
}

const HangmanMaxWrong = 6

var HangmanFaces = [HangmanMaxWrong + 1]string{
	"😀", "😧", "😨", "😰", "😱", "😵", "💀", // index 6 = dead / game over
}
