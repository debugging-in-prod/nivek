package overseer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// Client is a Pi-side WebSocket client to the executor.
// Single in-flight command at a time (mutex-serialized send-and-wait).
// Reconnects lazily on the next Send after a connection failure.
type Client struct {
	url     string
	hmacKey []byte

	mu         sync.Mutex
	conn       *websocket.Conn
	sentSeq    uint64
	lastAckSeq uint64
}

func NewClient(url string, hmacKey []byte) *Client {
	return &Client{url: url, hmacKey: hmacKey}
}

// Send marshals cmd, sends it over the (lazily-connected) WebSocket, and
// blocks until an ExecutedCmd ack arrives. On any transport or verification
// failure the connection is dropped. Transport-level failures (write/read
// failing on a connection the client thought was healthy) are retried once
// on a fresh connection — handles the stale-connection case after an
// executor restart or LAN hiccup without surfacing it to the chatter.
//
// At-most-twice on the wire: in the rare case the server processed the
// command and died before acking, the retry will queue a duplicate. Stale
// connections (the common case) drop the bytes before the server sees them,
// so no duplication.
func (c *Client) Send(ctx context.Context, cmd wire.Command) (*wire.ExecutedCmd, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	executed, retryable, err := c.sendOnceLocked(ctx, cmd)
	if err == nil || !retryable {
		return executed, err
	}
	// dropConnLocked already fired in sendOnceLocked; ensureConnectedLocked
	// inside the retry call will dial a fresh connection.
	executed, _, err = c.sendOnceLocked(ctx, cmd)
	return executed, err
}

// sendOnceLocked is the single-attempt body of Send. The bool return is
// true when the failure was transport-level (write/read on what should have
// been a healthy connection) and a retry on a fresh connection is appropriate.
func (c *Client) sendOnceLocked(ctx context.Context, cmd wire.Command) (*wire.ExecutedCmd, bool, error) {
	if err := c.ensureConnectedLocked(ctx); err != nil {
		return nil, false, fmt.Errorf("connect: %w", err)
	}

	c.sentSeq++
	seq := c.sentSeq

	buf, err := MarshalEnvelope(wire.EnvelopeTypeCommand, seq, cmd, c.hmacKey)
	if err != nil {
		return nil, false, fmt.Errorf("marshal command: %w", err)
	}

	if err := c.conn.Write(ctx, websocket.MessageText, buf); err != nil {
		c.dropConnLocked()
		return nil, true, fmt.Errorf("write: %w", err)
	}

	_, ackBuf, err := c.conn.Read(ctx)
	if err != nil {
		c.dropConnLocked()
		return nil, true, fmt.Errorf("read ack: %w", err)
	}

	env, err := UnmarshalEnvelope(ackBuf, c.hmacKey)
	if err != nil {
		c.dropConnLocked()
		return nil, false, fmt.Errorf("verify ack: %w", err)
	}
	if env.Type != wire.EnvelopeTypeExecutedCmd {
		return nil, false, fmt.Errorf("unexpected ack envelope type: %s", env.Type)
	}
	if env.Seq <= c.lastAckSeq {
		return nil, false, errors.New("replay in ack stream")
	}
	c.lastAckSeq = env.Seq

	var executed wire.ExecutedCmd
	if err := json.Unmarshal(env.Data, &executed); err != nil {
		return nil, false, fmt.Errorf("decode ack: %w", err)
	}
	return &executed, false, nil
}

// Close terminates the connection (idempotent).
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close(websocket.StatusNormalClosure, "")
	c.conn = nil
	return err
}

func (c *Client) ensureConnectedLocked(ctx context.Context) error {
	if c.conn != nil {
		return nil
	}
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, c.url, nil)
	if err != nil {
		return err
	}
	c.conn = conn
	c.sentSeq = 0
	c.lastAckSeq = 0
	return nil
}

func (c *Client) dropConnLocked() {
	if c.conn != nil {
		c.conn.Close(websocket.StatusInternalError, "")
		c.conn = nil
	}
}
