package overseer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
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
// failure the connection is dropped and the next Send will reconnect.
func (c *Client) Send(ctx context.Context, cmd Command) (*ExecutedCmd, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureConnectedLocked(ctx); err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	c.sentSeq++
	seq := c.sentSeq

	buf, err := MarshalEnvelope(EnvelopeTypeCommand, seq, cmd, c.hmacKey)
	if err != nil {
		return nil, fmt.Errorf("marshal command: %w", err)
	}

	if err := c.conn.Write(ctx, websocket.MessageText, buf); err != nil {
		c.dropConnLocked()
		return nil, fmt.Errorf("write: %w", err)
	}

	_, ackBuf, err := c.conn.Read(ctx)
	if err != nil {
		c.dropConnLocked()
		return nil, fmt.Errorf("read ack: %w", err)
	}

	env, err := UnmarshalEnvelope(ackBuf, c.hmacKey)
	if err != nil {
		c.dropConnLocked()
		return nil, fmt.Errorf("verify ack: %w", err)
	}
	if env.Type != EnvelopeTypeExecutedCmd {
		return nil, fmt.Errorf("unexpected ack envelope type: %s", env.Type)
	}
	if env.Seq <= c.lastAckSeq {
		return nil, errors.New("replay in ack stream")
	}
	c.lastAckSeq = env.Seq

	var executed ExecutedCmd
	if err := json.Unmarshal(env.Data, &executed); err != nil {
		return nil, fmt.Errorf("decode ack: %w", err)
	}
	return &executed, nil
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
