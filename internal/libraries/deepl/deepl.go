// Package deepl is a minimal client for the DeepL translation API. It targets
// the Free API plan (api-free.deepl.com; keys end in ":fx"). Only the single
// Translate call the bot needs is implemented.
package deepl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// FreeAPIURL is the DeepL Free plan translate endpoint. The paid plan uses
// api.deepl.com instead; a Free key (suffix ":fx") only works against this host.
const FreeAPIURL = "https://api-free.deepl.com/v2/translate"

const requestTimeout = 8 * time.Second

// Translate sends text to DeepL and returns the translation into targetLang
// (e.g. "JA", "EN-US"). sourceLang may be "" to let DeepL auto-detect it. The
// passed http.Client is used as-is except for a per-request timeout via context.
func Translate(client *http.Client, apiKey, text, targetLang, sourceLang string) (string, error) {
	return translateAt(client, FreeAPIURL, apiKey, text, targetLang, sourceLang)
}

// translateAt is the implementation, parameterized on the endpoint so tests can
// point it at a mock server.
func translateAt(client *http.Client, endpoint, apiKey, text, targetLang, sourceLang string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("missing DeepL API key")
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("nothing to translate")
	}

	form := url.Values{}
	form.Set("text", text)
	form.Set("target_lang", targetLang)
	if sourceLang != "" {
		form.Set("source_lang", sourceLang)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "DeepL-Auth-Key "+apiKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode/100 != 2 {
		// Surface the common, actionable DeepL failures by name.
		switch resp.StatusCode {
		case http.StatusForbidden: // 403
			return "", fmt.Errorf("deepl auth failed (bad/!fx key): %s", strings.TrimSpace(string(body)))
		case http.StatusTooManyRequests: // 429
			return "", fmt.Errorf("deepl rate limited")
		case 456: // DeepL-specific: quota exceeded
			return "", fmt.Errorf("deepl quota exceeded for this billing period")
		default:
			return "", fmt.Errorf("deepl returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
	}

	var decoded struct {
		Translations []struct {
			DetectedSourceLanguage string `json:"detected_source_language"`
			Text                   string `json:"text"`
		} `json:"translations"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(decoded.Translations) == 0 {
		return "", fmt.Errorf("deepl returned no translations")
	}
	return decoded.Translations[0].Text, nil
}
