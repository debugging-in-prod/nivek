// df-snapshot-pusher runs on the DFHost, invokes the dump-map-snapshot
// DFHack script on a slow timer, strips DFHack's color-reset codes from
// stdout, HMAC-signs the JSON body, and POSTs to nivek.life's dashboard
// ingest endpoint. All flow is outbound from DFHost — no inbound socket,
// no persistent authenticated channel.
//
// Configuration via .env (or process env):
//
//	DFHACK_RUN_PATH             absolute path to dfhack-run (shared with
//	                            df-executor; same value)
//	DASHBOARD_PUSH_URL          full URL of the receiver endpoint
//	                            (e.g. https://nivek.life/api/df/snapshot)
//	DASHBOARD_PUSH_HMAC_KEY     hex-encoded shared secret; must match
//	                            the same env var on the receiver side
//	DASHBOARD_PUSH_INTERVAL_SEC (optional, default 300) seconds between
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
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer"
)

const (
	defaultIntervalSec = 300
	minIntervalSec     = 10
	dfhackRunTimeout   = 60 * time.Second
	httpPushTimeout    = 30 * time.Second
)

// ansiRE matches ANSI CSI escape sequences. dfhack-run wraps script
// stdout in color-reset codes (\x1b[0m before AND after the script's
// output); the receiver expects plain JSON, so strip them client-side.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

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
	dfhackRunPath string
	pushURL       string
	hmacKey       []byte
	interval      time.Duration
	httpClient    *http.Client
}

func loadConfigOrExit() *config {
	c := &config{
		dfhackRunPath: requireEnv("DFHACK_RUN_PATH"),
		pushURL:       requireEnv("DASHBOARD_PUSH_URL"),
		httpClient:    &http.Client{Timeout: httpPushTimeout},
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
func parseSnapshot(body []byte) (*overseer.MapSnapshot, error) {
	var s overseer.MapSnapshot
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// isEmptySnapshot reports whether a snapshot carries no fortress data at all:
// no citizens, no furniture, and every tile Unknown across every level. This
// is the signature of a dump taken with no fort loaded (DF at the menu). Any
// single piece of real content makes it non-empty.
func isEmptySnapshot(s *overseer.MapSnapshot) bool {
	if len(s.Citizens) > 0 {
		return false
	}
	for i := range s.Levels {
		if len(s.Levels[i].Furniture) > 0 {
			return false
		}
		for _, t := range s.Levels[i].Tiles {
			if t != overseer.TileUnknown {
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

// pushOnce runs the dfhack-run dump-map-snapshot script, strips ANSI
// codes, signs the body, gzip-compresses it, and POSTs to the receiver.
// Errors are logged but don't take down the daemon — next tick will
// try again.
//
// HMAC is computed over the UNCOMPRESSED JSON so the signature binds
// to the content rather than the transport encoding (and stays valid
// if the compression algorithm/level changes someday). Receiver must
// decompress first, then verify.
func (c *config) pushOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), dfhackRunTimeout)
	defer cancel()

	raw, err := exec.CommandContext(ctx, c.dfhackRunPath, "dump-map-snapshot").CombinedOutput()
	if err != nil {
		log.Printf("dfhack-run failed: %v (first 200 bytes of output: %.200q)", err, raw)
		return
	}

	body := bytes.TrimSpace(ansiRE.ReplaceAll(raw, nil))
	if len(body) == 0 {
		log.Printf("dfhack-run produced empty output, skipping push")
		return
	}

	// Guard against pushing an empty capture. If DF is at the menu / no fort
	// is loaded when the dump fires (e.g. the game was quit before the pusher
	// was stopped), the snapshot has zero citizens and every tile is Unknown.
	// The receiver keeps only a single snapshot with no expiry, so pushing
	// this would clobber the last good one and blank the dashboard. Skipping
	// it leaves the last real snapshot in place. Parse failures fall through
	// to the push (the receiver validates) so a parsing hiccup never
	// suppresses genuine data.
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
