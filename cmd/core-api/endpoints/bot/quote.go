package bot

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/quote"
)

// quoteResponse is the wire shape of a single quote for the bot.
type quoteResponse struct {
	Found bool         `json:"found"`
	Quote *quote.Quote `json:"quote,omitempty"`
}

// NewGetQuoteEndpoint returns one of a channel's quotes: quote #number when
// the number query param is given, otherwise a random one. Responds
// {"found": false} when there is no such quote.
func NewGetQuoteEndpoint(nivekSvc nivek.NivekService) echo.HandlerFunc {
	return getQuoteHandler(quote.NewService(nivekSvc))
}

func getQuoteHandler(svc quote.NivekQuoteService) echo.HandlerFunc {
	return func(c echo.Context) error {
		channelID := c.QueryParam("channel_twitch_id")
		if channelID == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "channel_twitch_id query param required"})
		}

		var (
			q     quote.Quote
			found bool
			err   error
		)
		if raw := c.QueryParam("number"); raw != "" {
			n, convErr := strconv.Atoi(raw)
			if convErr != nil || n <= 0 {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "number must be a positive integer"})
			}
			q, found, err = svc.Get(channelID, n)
		} else {
			q, found, err = svc.Random(channelID)
		}
		if err != nil {
			log.Printf("[QUOTE] get for %s failed: %v", channelID, err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "quote lookup failed"})
		}
		if !found {
			return c.JSON(http.StatusOK, quoteResponse{Found: false})
		}
		return c.JSON(http.StatusOK, quoteResponse{Found: true, Quote: &q})
	}
}

// NewPostQuoteAddEndpoint stores a new quote and returns it with its
// per-channel number. The mod/broadcaster check lives in the bot handler.
func NewPostQuoteAddEndpoint(nivekSvc nivek.NivekService) echo.HandlerFunc {
	return postQuoteAddHandler(quote.NewService(nivekSvc))
}

func postQuoteAddHandler(svc quote.NivekQuoteService) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			ChannelTwitchId string  `json:"channel_twitch_id"`
			Text            string  `json:"text"`
			AddedBy         string  `json:"added_by"`
			Game            *string `json:"game"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "decode body"})
		}
		if req.ChannelTwitchId == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "channel_twitch_id required"})
		}
		if _, ok := quote.ValidText(req.Text); !ok {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "text must be 1-300 characters"})
		}
		q, err := svc.Add(req.ChannelTwitchId, req.Text, req.AddedBy, req.Game)
		if err != nil {
			if errors.Is(err, quote.ErrInvalid) {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid quote"})
			}
			log.Printf("[QUOTE] add for %s failed: %v", req.ChannelTwitchId, err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "quote add failed"})
		}
		return c.JSON(http.StatusOK, quoteResponse{Found: true, Quote: &q})
	}
}

// NewPostQuoteDeleteEndpoint removes a channel's quote by number. Responds
// {"found": false} when it didn't exist.
func NewPostQuoteDeleteEndpoint(nivekSvc nivek.NivekService) echo.HandlerFunc {
	return postQuoteDeleteHandler(quote.NewService(nivekSvc))
}

func postQuoteDeleteHandler(svc quote.NivekQuoteService) echo.HandlerFunc {
	return func(c echo.Context) error {
		var req struct {
			ChannelTwitchId string `json:"channel_twitch_id"`
			Number          int    `json:"number"`
		}
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "decode body"})
		}
		if req.ChannelTwitchId == "" || req.Number <= 0 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "channel_twitch_id and a positive number required"})
		}
		found, err := svc.Delete(req.ChannelTwitchId, req.Number)
		if err != nil {
			log.Printf("[QUOTE] delete #%d for %s failed: %v", req.Number, req.ChannelTwitchId, err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "quote delete failed"})
		}
		return c.JSON(http.StatusOK, map[string]bool{"found": found})
	}
}
