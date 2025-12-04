package twitchbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gempir/go-twitch-irc/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/autoshout"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/fishing"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

type Config struct {
	BotUsername string
	BotOAuth    string
	Channels    []string // Changed from single Channel to multiple Channels
	StoragePath string
	Timezone    string
}

type Bot struct {
	client            *twitch.Client
	config            Config
	counters          *CounterManager
	location          *time.Location
	nivek             nivek.NivekService
	autoShoutChatters map[string]map[string]interface{}
}

// @TODO::overhaul this substantially
// I think NewBot should work more like
// - twitch client
// - pull active users
// - create bots for each user based on their user tier
// - !bread and !piss are examples of free-tier commands while !fish and autoshout could be superuser
// imagining like
/**

this is just a basic bot
type Bot struct {
    client *twitch.Client // basic twitch connectivity
	config Config         // basic config -- currently this is where "chats to join" lives
	location *time.Location  // used for counters and time-based events
	nivek nivek.NivekService // db connections
}

then we could have an extended bot
type MidTierBot struct {
	Bot
	autoShout autoshout.NivekAutoShoutService
}

type PremiumBot struct {
	MidTierBot
	fishService fishing.NewNivekFishingService
}

so this premium bot still has core bot functionality, but extends that and offers extra functionality, like the
auto-shout system. With this architecture, I can limit new feature accessibility extensively

*/

func NewBot(nivek nivek.NivekService, config Config) (*Bot, error) {
	// Load timezone
	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone %s: %w", config.Timezone, err)
	}

	// Create counter manager
	counters, err := NewCounterManager(config.StoragePath, loc)
	if err != nil {
		return nil, fmt.Errorf("failed to create counter manager: %w", err)
	}

	// Fetch all auto-shout chatters
	autoShout := autoshout.NewService(nivek)
	shoutChatters, err := autoShout.GetAllAutoShoutChatters()
	if err != nil {
		log.Printf("Failed to get all auto shouts: %s", err.Error())
	}

	// Create Twitch IRC client
	client := twitch.NewClient(config.BotUsername, config.BotOAuth)

	bot := &Bot{
		client:            client,
		config:            config,
		counters:          counters,
		location:          loc,
		nivek:             nivek,
		autoShoutChatters: formatAutoShoutChatters(shoutChatters),
	}

	// Register message handler
	client.OnPrivateMessage(bot.handleMessage)

	// Log connection events
	client.OnConnect(func() {
		log.Printf("Connected to Twitch IRC as %s", config.BotUsername)
	})

	return bot, nil
}

func (b *Bot) Start(ctx context.Context) error {
	// Join all channels
	for _, channel := range b.config.Channels {
		b.client.Join(channel)
		log.Printf("Joining channel: %s", channel)
	}

	// Start reset timer
	go b.counters.StartResetTimer(ctx)

	// Start IRC client (blocking)
	go func() {
		if err := b.client.Connect(); err != nil {
			log.Printf("Error connecting to Twitch: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}

func (b *Bot) Stop() {
	log.Println("Disconnecting from Twitch...")
	b.client.Disconnect()

	// Save counters one last time
	if err := b.counters.Save(); err != nil {
		log.Printf("Error saving counters on shutdown: %v", err)
	}
}

func (b *Bot) handleMessage(message twitch.PrivateMessage) {
	// Normalize message
	msg := strings.TrimSpace(strings.ToLower(message.Message))
	chattername := message.User.Name
	channel := message.Channel

	if _, channelExists := b.autoShoutChatters[channel]; channelExists {
		if _, chatterExists := b.autoShoutChatters[channel][chattername]; chatterExists {
			b.client.Say(channel, fmt.Sprintf("!so %s", chattername))
			delete(b.autoShoutChatters[channel], chattername)
		}
	}

	// Check for commands
	switch msg {
	case "!bread":
		b.handleBreadCommand(chattername, channel)
	case "!piss":
		b.handlePissCommand(chattername, channel)
	case "!fish":
		b.handleFishCommand(chattername, channel)
	case "!dad":
		b.client.Say(channel, "still out getting milk!")
	}
}

// @TODO::autoshout still needs bot integration. The frontend/backend of the website part are functional
// but the bot also needs to do this per-streamer, and with the current setup it would pull every row from this table
// Ideally it will only pull for whoever is live, and even then maybe use a shorter-term data store to indicate shoutout
// status rather than keeping it in application code as map[string]bool. This is likely the least efficient use of dat

func (b *Bot) handleBreadCommand(username, channel string) {
	userCount := b.counters.IncrementBread(username)
	totalCount := b.counters.GetTotalBread()

	response := fmt.Sprintf(
		"@%s has baked %d loaf%s of bread today! 🍞 This chat has baked %d loaf%s total in the last 24 hours.",
		username,
		userCount,
		pluralize(userCount),
		totalCount,
		pluralize(totalCount),
	)

	b.client.Say(channel, response)
	log.Printf("[BREAD] [%s] %s: %d (Total: %d)", channel, username, userCount, totalCount)
}

func (b *Bot) handlePissCommand(username, channel string) {
	userCount := b.counters.IncrementPiss(username)
	totalCount := b.counters.GetTotalPiss()

	response := fmt.Sprintf(
		"@%s has pissed %d time%s today! 💦 This chat has pissed %d time%s total in the last 24 hours.",
		username,
		userCount,
		pluralize(userCount),
		totalCount,
		pluralize(totalCount),
	)

	b.client.Say(channel, response)
	log.Printf("[PISS] [%s] %s: %d (Total: %d)", channel, username, userCount, totalCount)
}

func (b *Bot) handleFishCommand(username, channel string) {
	fishingService := fishing.NewService(b.nivek, channel)
	response := fishingService.GoFishing(username)

	b.client.Say(channel, response)
	log.Printf("[FISH] [%s] %s", channel, username)
}

func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func formatAutoShoutChatters(shoutChatters []autoshout.ShoutChatter) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})

	for _, chatter := range shoutChatters {
		if _, exists := result[chatter.ChannelName]; !exists {
			result[chatter.ChannelName] = make(map[string]interface{})
		}

		result[chatter.ChannelName][chatter.ChatterName] = nil
	}

	return result
}
