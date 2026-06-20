// df-snapshot-pusher runs on the DFHost. It reads the snapshot JSON file
// published by the snapshot-scan DFHack script on a slow timer, HMAC-signs
// the body, gzip-compresses it, and POSTs to peanutbudderbot.com's dashboard ingest
// endpoint. All flow is outbound from DFHost — no inbound socket, no
// persistent authenticated channel.
//
// The snapshot is produced incrementally in-game by snapshot-scan.lua (a
// time-boxed scan that never freezes DF) and published via atomic rename, so
// the file the pusher reads is always a complete pass.
//
// Configuration via .env (or process env):
//
//	DASHBOARD_SNAPSHOT_FILE     absolute path to the snapshot JSON written by
//	                            snapshot-scan.lua (e.g. <DF>/nivek-snapshot.json)
//	DASHBOARD_PUSH_URL          full URL of the receiver endpoint
//	                            (e.g. https://peanutbudderbot.com/api/df/snapshot)
//	DASHBOARD_PUSH_HMAC_KEY     hex-encoded shared secret; must match
//	                            the same env var on the receiver side
//	DASHBOARD_PUSH_INTERVAL_SEC (optional, default 15) seconds between
//	                            pushes; minimum 10 to prevent foot-shooting
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

const (
	defaultIntervalSec = 15
	minIntervalSec     = 10
	httpPushTimeout    = 30 * time.Second
)

func main() {
	_ = godotenv.Load()

	cfg := loadConfigOrExit()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("df-snapshot-pusher: pushing every %s to %s", cfg.interval, cfg.pushURL)

	// Fire one push immediately so failures surface in logs without
	// waiting an interval cycle on first boot.
	cfg.pushOnce()

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			log.Println("df-snapshot-pusher: shutting down")
			return
		case <-ticker.C:
			cfg.pushOnce()
		}
	}
}

type config struct {
	snapshotFile string
	pushURL      string
	hmacKey      []byte
	interval     time.Duration
	httpClient   *http.Client
}

func loadConfigOrExit() *config {
	c := &config{
		snapshotFile: requireEnv("DASHBOARD_SNAPSHOT_FILE"),
		pushURL:      requireEnv("DASHBOARD_PUSH_URL"),
		httpClient:   &http.Client{Timeout: httpPushTimeout},
	}
	keyHex := requireEnv("DASHBOARD_PUSH_HMAC_KEY")
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Fatalf("DASHBOARD_PUSH_HMAC_KEY is not valid hex: %v", err)
	}
	c.hmacKey = key

	intervalSec := defaultIntervalSec
	if s := os.Getenv("DASHBOARD_PUSH_INTERVAL_SEC"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			log.Fatalf("DASHBOARD_PUSH_INTERVAL_SEC must be an integer, got %q", s)
		}
		if n < minIntervalSec {
			log.Fatalf("DASHBOARD_PUSH_INTERVAL_SEC must be >= %d, got %d", minIntervalSec, n)
		}
		intervalSec = n
	}
	c.interval = time.Duration(intervalSec) * time.Second
	return c
}

// parseSnapshot decodes the dump JSON into a MapSnapshot for emptiness
// checking. The pusher otherwise forwards bytes verbatim; this is the only
// place it inspects the payload.
func parseSnapshot(body []byte) (*wire.MapSnapshot, error) {
	var s wire.MapSnapshot
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// isEmptySnapshot reports whether a snapshot carries no fortress data at all:
// no citizens, no furniture, and every tile Unknown across every level. This
// is the signature of a dump taken with no fort loaded (DF at the menu). Any
// single piece of real content makes it non-empty.
func isEmptySnapshot(s *wire.MapSnapshot) bool {
	if len(s.Citizens) > 0 {
		return false
	}
	for i := range s.Levels {
		if len(s.Levels[i].Furniture) > 0 {
			return false
		}
		for _, t := range s.Levels[i].Tiles {
			if t != wire.TileUnknown {
				return false
			}
		}
	}
	return true
}

func requireEnv(name string) string {
	v := os.Getenv(name)
	if v == "" {
		log.Fatalf("%s env var is required", name)
	}
	return v
}

// pushOnce reads the snapshot file published by snapshot-scan.lua, signs the
// body, gzip-compresses it, and POSTs to the receiver. Errors are logged but
// don't take down the daemon — next tick will try again.
//
// HMAC is computed over the UNCOMPRESSED JSON so the signature binds
// to the content rather than the transport encoding (and stays valid
// if the compression algorithm/level changes someday). Receiver must
// decompress first, then verify.
func (c *config) pushOnce() {
	raw, err := os.ReadFile(c.snapshotFile)
	if err != nil {
		log.Printf("read snapshot file %s: %v", c.snapshotFile, err)
		return
	}

	body := bytes.TrimSpace(raw)
	if len(body) == 0 {
		log.Printf("snapshot file is empty, skipping push")
		return
	}

	// Guard against pushing an empty capture (zero citizens, all tiles
	// Unknown) — the signature of a snapshot taken with no fort loaded. The
	// snapshot-scan script only runs while a fort is loaded, so this is now
	// belt-and-suspenders, but the receiver keeps a single snapshot with no
	// expiry, so a stray empty file would clobber the last good one and blank
	// the dashboard. Parse failures fall through to the push (the receiver
	// validates) so a parsing hiccup never suppresses genuine data.
	if snap, perr := parseSnapshot(body); perr != nil {
		log.Printf("warning: could not parse snapshot to check for emptiness, pushing anyway: %v", perr)
	} else if isEmptySnapshot(snap) {
		log.Printf("snapshot is empty (no fort loaded: 0 citizens, all tiles Unknown), skipping push")
		return
	}

	mac := hmac.New(sha256.New, c.hmacKey)
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(body); err != nil {
		log.Printf("gzip write: %v", err)
		return
	}
	if err := gz.Close(); err != nil {
		log.Printf("gzip close: %v", err)
		return
	}

	// Capture compressed size *before* sending — http.Client.Do drains the
	// buffer as it reads the body, so a post-send compressed.Len() returns 0.
	compressedSize := compressed.Len()

	ctx, cancel := context.WithTimeout(context.Background(), httpPushTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.pushURL, &compressed)
	if err != nil {
		log.Printf("build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("X-Nivek-HMAC", sig)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("POST failed: %v", err)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		log.Printf("POST returned HTTP %d: %.200q", resp.StatusCode, respBody)
		return
	}
	log.Printf("pushed %d bytes raw, %d bytes compressed (%.1fx ratio), HTTP %d",
		len(body), compressedSize, float64(len(body))/float64(compressedSize), resp.StatusCode)
}
