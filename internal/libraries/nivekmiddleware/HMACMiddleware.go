package nivekmiddleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// HMACMaxClockSkewSeconds is the replay window. A captured request can only be
// re-sent within this many seconds of its original timestamp. 60s is comfortably
// wider than realistic clock drift between the Pi and Vultr (both NTP-synced)
// while staying narrow enough that replay is a non-issue for write traffic.
const HMACMaxClockSkewSeconds = 60

// NewHMACMiddleware returns an Echo middleware that authenticates the caller
// by verifying a per-request HMAC-SHA256 signature using a key loaded from the
// given env var (hex-encoded, decoded to raw bytes).
//
// Canonical string the client must sign (newline-separated, no trailing \n):
//
//	<METHOD>\n<PATH>\n<RAW_QUERY>\n<TIMESTAMP>\n<BODY>
//
// Headers the client must send:
//   - X-Nivek-Timestamp: unix seconds, as decimal
//   - X-Nivek-HMAC:      hex of HMAC-SHA256(key, canonical)
//
// Failure modes return distinct status codes so the caller's logs identify
// the misconfiguration:
//
//	503 — server-side key env var unset or invalid hex (deploy issue)
//	400 — body unreadable, headers missing/malformed, or timestamp not numeric
//	401 — timestamp outside the skew window, or HMAC doesn't match
//
// The body is fully buffered so the downstream handler can re-read it.
func NewHMACMiddleware(keyEnvVar string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			keyHex := os.Getenv(keyEnvVar)
			if keyHex == "" {
				return c.JSON(http.StatusServiceUnavailable, map[string]string{
					"error": fmt.Sprintf("hmac auth not configured (%s missing)", keyEnvVar),
				})
			}
			key, err := hex.DecodeString(keyHex)
			if err != nil {
				return c.JSON(http.StatusServiceUnavailable, map[string]string{
					"error": fmt.Sprintf("%s is not valid hex", keyEnvVar),
				})
			}

			tsHeader := c.Request().Header.Get("X-Nivek-Timestamp")
			if tsHeader == "" {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "missing X-Nivek-Timestamp header",
				})
			}
			ts, err := strconv.ParseInt(tsHeader, 10, 64)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "X-Nivek-Timestamp is not a unix-seconds integer",
				})
			}
			skew := time.Now().Unix() - ts
			if skew < 0 {
				skew = -skew
			}
			if skew > HMACMaxClockSkewSeconds {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "request timestamp outside acceptable skew window",
				})
			}

			claimedHex := c.Request().Header.Get("X-Nivek-HMAC")
			if claimedHex == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "missing X-Nivek-HMAC header",
				})
			}
			claimed, err := hex.DecodeString(claimedHex)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "X-Nivek-HMAC is not valid hex",
				})
			}

			body, err := io.ReadAll(c.Request().Body)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "read body",
				})
			}
			// Restore the body so the downstream handler can read it again.
			c.Request().Body = io.NopCloser(bytes.NewReader(body))

			canonical := fmt.Sprintf(
				"%s\n%s\n%s\n%d\n%s",
				c.Request().Method,
				c.Request().URL.Path,
				c.Request().URL.RawQuery,
				ts,
				body,
			)

			mac := hmac.New(sha256.New, key)
			mac.Write([]byte(canonical))
			if !hmac.Equal(claimed, mac.Sum(nil)) {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "hmac mismatch",
				})
			}

			return next(c)
		}
	}
}
