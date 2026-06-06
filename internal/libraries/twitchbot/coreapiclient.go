package twitchbot

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// CoreAPIClient is the bot's only path to persistent state. Every call signs
// the request body + method + path + timestamp with HMAC-SHA256 and includes:
//
//	X-Nivek-Timestamp: unix seconds
//	X-Nivek-HMAC:      hex(SHA256(key, <METHOD>\n<PATH>\n<QUERY>\n<TS>\n<BODY>))
//
// Matches the canonical-string format enforced by
// nivekmiddleware.NewHMACMiddleware on core-api.
type CoreAPIClient struct {
	baseURL    string
	hmacKey    []byte
	httpClient *http.Client
}

func NewCoreAPIClient(baseURL, hmacKeyHex string) (*CoreAPIClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("CORE_API_URL is empty")
	}
	key, err := hex.DecodeString(hmacKeyHex)
	if err != nil {
		return nil, fmt.Errorf("BOT_API_HMAC_KEY is not valid hex: %w", err)
	}
	if len(key) < 16 {
		// Server enforces 32-byte minimum via deploy convention; 16 here is a
		// sanity floor that catches obvious typos like a 2-char key.
		return nil, fmt.Errorf("BOT_API_HMAC_KEY too short (%d bytes)", len(key))
	}
	return &CoreAPIClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		hmacKey:    key,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// do executes a signed request and decodes the JSON response into `out`.
// Path is the route under /api (e.g. "/bot/channels"); query is the raw
// query string without leading "?". Body may be nil for GETs.
func (c *CoreAPIClient) do(method, path, rawQuery string, body []byte, out any) error {
	full := c.baseURL + "/api" + path
	if rawQuery != "" {
		full += "?" + rawQuery
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	canonical := fmt.Sprintf("%s\n/api%s\n%s\n%s\n%s", method, path, rawQuery, ts, body)
	mac := hmac.New(sha256.New, c.hmacKey)
	mac.Write([]byte(canonical))
	sig := hex.EncodeToString(mac.Sum(nil))

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, full, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("X-Nivek-Timestamp", ts)
	req.Header.Set("X-Nivek-HMAC", sig)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("core-api %s %s: status %d: %s", method, path, resp.StatusCode, respBody)
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (c *CoreAPIClient) GetChannels() ([]string, error) {
	var resp struct {
		Channels []string `json:"channels"`
	}
	if err := c.do(http.MethodGet, "/bot/channels", "", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Channels, nil
}

func (c *CoreAPIClient) IncrementBread(channel, chatter string) (int, error) {
	body, _ := json.Marshal(map[string]string{"channel": channel, "chatter": chatter})
	var resp struct {
		Count int `json:"count"`
	}
	if err := c.do(http.MethodPost, "/bot/bread/increment", "", body, &resp); err != nil {
		return 0, err
	}
	return resp.Count, nil
}

func (c *CoreAPIClient) GetBreadTotal(channel string) (int, error) {
	q := url.Values{}
	q.Set("channel", channel)
	var resp struct {
		Total int `json:"total"`
	}
	if err := c.do(http.MethodGet, "/bot/bread/total", q.Encode(), nil, &resp); err != nil {
		return 0, err
	}
	return resp.Total, nil
}

func (c *CoreAPIClient) LurkOnMessage(channel, chatter string) int {
	body, _ := json.Marshal(map[string]string{"channel": channel, "chatter": chatter})
	var resp struct {
		Count int `json:"count"`
	}
	if err := c.do(http.MethodPost, "/bot/lurk/message", "", body, &resp); err != nil {
		// Mirror lurk.OnMessage's swallow-and-return-0 behavior so the
		// caller's `count > 0` gate keeps working untouched.
		return 0
	}
	return resp.Count
}

func (c *CoreAPIClient) GoFishing(channel, chatter string) (string, error) {
	body, _ := json.Marshal(map[string]string{"channel": channel, "chatter": chatter})
	var resp struct {
		Message string `json:"message"`
	}
	if err := c.do(http.MethodPost, "/bot/fish/go", "", body, &resp); err != nil {
		return "", err
	}
	return resp.Message, nil
}
