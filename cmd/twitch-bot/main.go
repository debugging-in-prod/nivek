package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/twitchbot"
)

func main() {
	// Load configuration from environment
	channelsStr := getEnv("TWITCH_CHANNELS", getEnv("TWITCH_CHANNEL", ""))

	// Parse comma-separated channel list
	var channels []string
	if channelsStr != "" {
		channels = strings.Split(channelsStr, ",")
		// Trim whitespace from each channel
		for i, ch := range channels {
			channels[i] = strings.TrimSpace(ch)
		}
	}

	config := twitchbot.Config{
		BotUsername: getEnv("TWITCH_BOT_USERNAME", ""),
		BotOAuth:    getEnv("TWITCH_BOT_OAUTH", ""),
		Channels:    channels,
		StoragePath: getEnv("TWITCH_STORAGE_PATH", "./data/twitch-counters.json"),
		Timezone:    getEnv("TWITCH_TIMEZONE", "America/New_York"),
	}

	// Validate required config
	if config.BotUsername == "" || config.BotOAuth == "" || len(config.Channels) == 0 {
		log.Fatal("Missing required environment variables: TWITCH_BOT_USERNAME, TWITCH_BOT_OAUTH, TWITCH_CHANNELS (or TWITCH_CHANNEL)")
	}

	// Create bot instance
	bot, err := twitchbot.NewBot(config)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start bot in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- bot.Start(ctx)
	}()

	// Wait for shutdown signal or error
	select {
	case <-sigChan:
		log.Println("Received shutdown signal, gracefully stopping bot...")
		cancel()
		bot.Stop()
	case err := <-errChan:
		if err != nil {
			log.Fatalf("Bot error: %v", err)
		}
	}

	log.Println("Bot stopped successfully")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
