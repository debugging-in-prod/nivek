package endpoints

import ( 
  "bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
  "encoding/json"
  "io"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
  "github.com/tim-the-toolman-taylor/nivek/cmd/core-api/coreconfig"
)

type User struct {
	id        int
	username  string
	email     string
	createdAt string
}

// Notification request headers
const (
  TWITCH_MESSAGE_ID        = "Twitch-Eventsub-Message-Id"
  TWITCH_MESSAGE_TIMESTAMP = "Twitch-Eventsub-Message-Timestamp"
  TWITCH_MESSAGE_SIGNATURE = "Twitch-Eventsub-Message-Signature"
  MESSAGE_TYPE             = "Twitch-Eventsub-Message-Type"
)

// Notification message types
const (
  MESSAGE_TYPE_VERIFICATION = "webhook_callback_verification"
  MESSAGE_TYPE_NOTIFICATION = "notification"
  MESSAGE_TYPE_REVOCATION   = "revocation"
)

const HMAC_PREFIX = "sha256="

type EventSubSubscriptionResponse struct {
  Challenge string                  `json:"challenge,omitempty"`
  Subscription SubscriptionResponse `json:"subscription"`
  // Event is present on notification message - keep raw until needed
  Event json.RawMessage             `json:"event,omitempty"`
  Transport map[string]string       `json:"transport"`
  CreatedAt string                  `json:"created_at"`
}

type SubscriptionResponse struct {
  Id string                   `json:"id"`
  Status string               `json:"status"`
  Type string                 `json:"type"`
  Cost int                    `json:"cost"`
  Version string              `json:"version"`
  Condition map[string]string `json:"condition"`
}

func NewIndexEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.String(http.StatusOK, "Hello world")
	}
}

func NewTwitchEventSubEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
  return func(c echo.Context) error {
    cfg, _ := nivek.CustomConfig().(coreconfig.CoreApiConfig)
    secret := cfg.TwitchEventSubSecret

    body, err := io.ReadAll(c.Request().Body)
    if err != nil {
      nivek.Logger().Errorf("failed to read eventsub body: %v", err)
      return c.NoContent(http.StatusBadRequest)
    }
    // Restore body in case anything else reads it later.
    c.Request().Body = io.NopCloser(bytes.NewReader(body)) 

    message := getHmacMessage(c.Request(), string(body))
    computedHmac := fmt.Sprintf("%s%s", HMAC_PREFIX, getHmac(secret, message))
    if !verifyMessage(computedHmac, c.Request().Header.Get(TWITCH_MESSAGE_SIGNATURE)) {
      return c.NoContent(http.StatusForbidden)
    }

    var notification EventSubSubscriptionResponse
    if err := c.Bind(&notification); err != nil {
      nivek.Logger().Errorf("failed to parse webhook subscription response: %s", err.Error())
      return c.NoContent(http.StatusBadRequest)
    }

    if MESSAGE_TYPE_NOTIFICATION == c.Request().Header.Get(MESSAGE_TYPE) {
      // this is where stream.online and stream.offline are going to be handled
      // I could put this on the bot directly, but having it here makes more sense as incoming requests are already routed to this API service
      // so if I want to keep this setup, then this API is going to need to be able to manage the bot connecting and disconnecting to streams


      // @TODO::do something with notifications
      nivek.Logger().Debugf("Event Type: %s", notification.Subscription.Type)
      nivek.Logger().Debugf("notification: %+v", notification)
      return c.NoContent(http.StatusNoContent)
    }

    if MESSAGE_TYPE_VERIFICATION == c.Request().Header.Get(MESSAGE_TYPE) {
      return c.String(http.StatusOK, notification.Challenge)
    }

    if MESSAGE_TYPE_REVOCATION == c.Request().Header.Get(MESSAGE_TYPE) {
      nivek.Logger().Debugf("%s notifications revoked!", notification.Subscription.Type)
      nivek.Logger().Debugf("reason: %s", notification.Subscription.Status)
      nivek.Logger().Debugf("condition: %s", notification.Subscription.Condition)
      return c.NoContent(http.StatusNoContent)
    }

    return c.NoContent(http.StatusBadRequest)
  }
}

func getHmacMessage(request *http.Request, body string) string {
  return fmt.Sprintf("%s%s%s",
    request.Header.Get(strings.ToLower(TWITCH_MESSAGE_ID)),
    request.Header.Get(strings.ToLower(TWITCH_MESSAGE_TIMESTAMP)),
    body,
  )
}

func getHmac(secret, message string) string {
  mac := hmac.New(sha256.New, []byte(secret))
  mac.Write([]byte(message))
  return hex.EncodeToString(mac.Sum(nil))
}

func verifyMessage(computedHmac, verifySignature string) bool {
  return hmac.Equal([]byte(computedHmac), []byte(verifySignature))
}

