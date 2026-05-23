package twitchbot

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gempir/go-twitch-irc/v4"
	"github.com/google/uuid"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/autoshout"
	bread2 "github.com/tim-the-toolman-taylor/nivek/internal/libraries/bread"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/fishing"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/lurk"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer"
)

type Config struct {
	BotUsername      string
	BotOAuth         string
	Channels         []string // Changed from single Channel to multiple Channels
	StoragePath      string
	Timezone         string
	ExecutorWSURL    string // e.g. ws://192.168.1.X:8123/ws
	OverseerHmacKey  string // hex-encoded HMAC key, shared with the executor
}

type Bot struct {
	client         *twitch.Client
	config         Config
	counters       *CounterManager
	location       *time.Location
	nivek          nivek.NivekService
	autoShout      autoshout.NivekAutoShoutService
	bread          bread2.NivekBreadService
	lurkSvc        lurk.NivekLurkService
	overseerClient *overseer.Client
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

	// overseer (DF Twitch-plays) WebSocket client to the executor
	hmacKey, err := hex.DecodeString(config.OverseerHmacKey)
	if err != nil {
		return nil, fmt.Errorf("invalid OVERSEER_HMAC_KEY hex: %w", err)
	}
	overseerCli := overseer.NewClient(config.ExecutorWSURL, hmacKey)

	// Create Twitch IRC client
	client := twitch.NewClient(config.BotUsername, config.BotOAuth)
	client.IrcAddress = "irc.chat.twitch.tv:6697"
	client.TLS = true

	bot := &Bot{
		client:         client,
		config:         config,
		counters:       counters,
		location:       loc,
		nivek:          nivek,
		autoShout:      autoShout,
		bread:          bread,
		lurkSvc:        lurkSvc,
		overseerClient: overseerCli,
	}

	// Register message handler
	client.OnPrivateMessage(bot.handleMessage)

	client.OnNoticeMessage(func(m twitch.NoticeMessage) {
		log.Printf("[NOTICE] [%s] msg-id=%s: %s", m.Channel, m.MsgID, m.Message)
	})

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

	// Start IRC client with panic recovery and auto-reconnect
	go func() {
		for {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Recovered from Twitch IRC panic: %v", r)
					}
				}()
				if err := b.client.Connect(); err != nil {
					log.Printf("Error connecting to Twitch: %v", err)
				}
			}()

			select {
			case <-ctx.Done():
				return
			default:
				log.Println("Reconnecting to Twitch IRC in 5 seconds...")
				time.Sleep(5 * time.Second)
			}
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

	if strings.HasPrefix(msg, "!") {
		log.Printf("[CMD-RECV] [%s] %s: %q", channel, chattername, msg)
	}

	// if b.autoShout.OnMessage(channel, chattername) {
	// 	b.client.Say(channel, fmt.Sprintf("!so @%s", chattername))
	// }

	// !DF takes arguments — handle separately from the exact-match commands below
	if msg == "!df" || strings.HasPrefix(msg, "!df ") {
		args := strings.TrimSpace(strings.TrimPrefix(msg, "!df"))
		b.handleDFCommand(message.Message, args, chattername, channel)
		return
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
	log.Printf("[BREAD] [%s] %s: %d (Total: %d)", channel, username, count, totalCount)
}

func (b *Bot) handleFishCommand(username, channel string) {
	fishingService := fishing.NewService(b.nivek, channel)
	response := fishingService.GoFishing(username)

	b.client.Say(channel, response)
	log.Printf("[FISH] [%s] %s", channel, username)
}

func (b *Bot) handleDFCommand(rawText, args, username, channel string) {
	action, err := overseer.ParseCommand(args)
	if err != nil {
		log.Printf("[DF] [%s] %s: parse failed for %q: %v", channel, username, args, err)
		return // silent reject (locked design)
	}

	// help is a chat-response verb — no DFHack involvement, no executor
	// round-trip. Short-circuit here before the WS send.
	if action.Kind == overseer.ActionKindHelp {
		b.client.Say(channel, fmt.Sprintf(
			"@%s !DF: make [N] <material> <item> | pause | unpause | camera <x> <y> <z> | help",
			username,
		))
		log.Printf("[DF] [%s] %s: help requested", channel, username)
		return
	}

	cmd := overseer.Command{
		ID:         uuid.NewString(),
		ReceivedAt: time.Now().UTC(),
		RawText:    rawText,
		From: overseer.CommandSource{
			Username: username,
			Platform: overseer.PlatformTwitch,
			Channel:  channel,
		},
		Action: action,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	executed, err := b.overseerClient.Send(ctx, cmd)
	if err != nil {
		log.Printf("[DF] [%s] %s: executor send failed: %v", channel, username, err)
		b.client.Say(channel, fmt.Sprintf("@%s — couldn't reach DF: %s", username, err.Error()))
		return
	}

	if executed.Result == overseer.ExecResultError {
		log.Printf("[DF] [%s] %s: executor error: %s", channel, username, executed.ErrorMessage)
		b.client.Say(channel, fmt.Sprintf("@%s — couldn't queue: %s", username, executed.ErrorMessage))
		return
	}

	b.client.Say(channel, dfSuccessReply(username, action))
}

func dfSuccessReply(username string, action overseer.Action) string {
	switch action.Kind {
	case overseer.ActionKindManufacture:
		mat := ""
		if action.Material != nil {
			mat = *action.Material + " "
		}
		return fmt.Sprintf("@%s queued %d %s%s%s", username, action.Quantity, mat, action.Item, pluralize(action.Quantity))
	case overseer.ActionKindPause:
		return fmt.Sprintf("@%s paused DF", username)
	case overseer.ActionKindUnpause:
		return fmt.Sprintf("@%s unpaused DF", username)
	case overseer.ActionKindCamera:
		if action.Position != nil {
			return fmt.Sprintf("@%s moved camera to (%d, %d, %d)", username, action.Position.X, action.Position.Y, action.Position.Z)
		}
		return fmt.Sprintf("@%s moved camera", username)
	default:
		return fmt.Sprintf("@%s executed %s", username, action.Kind)
	}
}

func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
