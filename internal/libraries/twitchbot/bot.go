package twitchbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gempir/go-twitch-irc/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/autoshout"
	bread2 "github.com/tim-the-toolman-taylor/nivek/internal/libraries/bread"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/fishing"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/lurk"
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
	client    *twitch.Client
	config    Config
	counters  *CounterManager
	location  *time.Location
	nivek     nivek.NivekService
	autoShout autoshout.NivekAutoShoutService
	bread     bread2.NivekBreadService
	lurkSvc   lurk.NivekLurkService
}

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

	// auto shout service
	autoShout := autoshout.NewService(nivek)
	log.Println("[TwitchBot] created auto shout service")

	// bread service
	bread := bread2.NewService(nivek)

	// lurk service
	lurkSvc := lurk.NewService(nivek)

	// Create Twitch IRC client
	client := twitch.NewClient(config.BotUsername, config.BotOAuth)

	bot := &Bot{
		client:    client,
		config:    config,
		counters:  counters,
		location:  loc,
		nivek:     nivek,
		autoShout: autoShout,
		bread:     bread,
		lurkSvc:   lurkSvc,
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

	log.Printf("[%s] %s", chattername, msg)

	if b.autoShout.OnMessage(channel, chattername) {
		b.client.Say(channel, fmt.Sprintf("!so @%s", chattername))
	}

	// Check for commands
	switch msg {
	case "!bread":
		b.handleBreadCommand(chattername, channel)
	case "!fish":
		b.handleFishCommand(chattername, channel)
	case "!dad":
		b.client.Say(channel, "still out getting milk!")
	case "!lurk":
		b.handleLurkCommand(chattername, channel)
	}
}

func (b *Bot) handleLurkCommand(username, channel string) {
	if count := b.lurkSvc.OnMessage(channel, username); count > 0 {
		b.client.Say(channel, fmt.Sprintf(
			"thank you for the lurk! @%s You have lurked %d times",
			username,
			count,
		))
	}
}

func (b *Bot) handleBreadCommand(username, channel string) {
	count, err := b.bread.IncrementCount(channel, username)
	if err != nil {
		log.Printf("error incrementing bread count for channel [%s] chatter [%s]: %s", channel, username, err.Error())
		return
	}
	totalCount, err := b.bread.GetTotalBreadForChannel(channel)
	if err != nil {
		log.Printf("error getting total bread count for channel [%s] chatter [%s]: %s", channel, username, err.Error())
		return
	}

	response := fmt.Sprintf(
		"@%s has baked %d loaf%s of bread today! 🍞 This chat has baked %d loaf%s total in the last 24 hours.",
		username,
		count,
		pluralize(count),
		totalCount,
		pluralize(totalCount),
	)

	b.client.Say(channel, response)
	// log.Printf("[BREAD] [%s] %s: %d (Total: %d)", channel, username, count, totalCount)
}

func (b *Bot) handleFishCommand(username, channel string) {
	fishingService := fishing.NewService(b.nivek, channel)
	response := fishingService.GoFishing(username)

	b.client.Say(channel, response)
	// log.Printf("[FISH] [%s] %s", channel, username)
}

func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
