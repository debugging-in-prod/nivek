package overseer

import (
	"encoding/json"
	"time"
)

// Wire format types for the Overseer system: a Twitch-plays-Dwarf-Fortress pipeline.
//
// Messages flow between three tiers:
//   - Pi (chat parser): produces Command envelopes from parsed Twitch chat.
//   - Executor (dedicated laptop running DF + DFHack): consumes Commands, produces StateSnapshot envelopes.
//   - Vultr (public dashboard host): forwards StateSnapshots to browsers after stripping from_username.
//
// On internal links (Pi↔Executor, Pi↔Vultr) the full Envelope carries an HMAC.
// On the public Vultr→browser fanout the Envelope omits HMAC and Vultr strips from_username
// from any Queue / LastExecuted / RecentHistory entries before broadcasting.

// --- Command ---

// Command is a validated, parsed chat input the Pi sends to the executor.
type Command struct {
	ID         string        `json:"id"`
	ReceivedAt time.Time     `json:"received_at"`
	RawText    string        `json:"raw_text"`
	From       CommandSource `json:"from"`
	Action     Action        `json:"action"`
}

// CommandSource identifies who issued the command and where.
type CommandSource struct {
	Username string `json:"username"`
	Platform string `json:"platform"`
	Channel  string `json:"channel"`
}

const (
	PlatformTwitch = "twitch"
)

// Action is a tagged union over the verb-family being executed.
// v1 supports only ActionKindManufacture; future kinds (Place, Designate,
// Zone, Assign, Military, ...) extend this shape without disturbing manufacture.
// The flat-struct + Kind discriminator is intentional for v1 simplicity;
// refactor toward a sum-type pattern (interface + custom UnmarshalJSON) when
// a second kind lands.
type Action struct {
	Kind ActionKind `json:"kind"`

	// Fields below are populated based on Kind.
	// For Kind == ActionKindManufacture:
	Item     string  `json:"item,omitempty"`
	Material *string `json:"material,omitempty"`
	Quantity int     `json:"quantity,omitempty"`
}

type ActionKind string

const (
	ActionKindManufacture ActionKind = "manufacture"
	ActionKindPause       ActionKind = "pause"
	ActionKindUnpause     ActionKind = "unpause"
)

// --- StateSnapshot ---

// StateSnapshot is the full state of the executor at a moment in time.
// Produced by the executor, forwarded byte-for-byte by Pi, fanned out by Vultr
// (which first strips from_username from any nested ExecutedCmd / QueuedCmd entries
// before sending to public browsers).
type StateSnapshot struct {
	Seq           uint64         `json:"seq"`
	SnapshotAt    time.Time      `json:"snapshot_at"`
	Mode          GameMode       `json:"mode"`
	Vote          *VoteState     `json:"vote"`
	Queue         []QueuedCmd    `json:"queue"`
	LastExecuted  *ExecutedCmd   `json:"last_executed"`
	RecentHistory []ExecutedCmd  `json:"recent_history"`
	Fortress      *FortressStats `json:"fortress"`
	Links         LinkHealth     `json:"links"`
}

type GameMode string

const (
	GameModeDemocracy GameMode = "democracy"
	GameModeAnarchy   GameMode = "anarchy"
)

// VoteState describes the current vote (democracy mode). Nil in anarchy mode.
type VoteState struct {
	Open     bool        `json:"open"`
	OpensAt  time.Time   `json:"opens_at"`
	ClosesAt time.Time   `json:"closes_at"`
	Tally    []VoteTally `json:"tally"`
}

type VoteTally struct {
	RawText    string `json:"raw_text"`
	Action     Action `json:"action"`
	VoterCount int    `json:"voter_count"`
}

// QueuedCmd is a command waiting to execute (anarchy mode).
// FromUsername is stripped by Vultr before public fanout.
type QueuedCmd struct {
	CommandID    string    `json:"command_id"`
	RawText      string    `json:"raw_text"`
	Action       Action    `json:"action"`
	FromUsername string    `json:"from_username,omitempty"`
	EnqueuedAt   time.Time `json:"enqueued_at"`
}

// ExecutedCmd is a record of a command that the executor attempted to run.
// FromUsername is stripped by Vultr before public fanout.
type ExecutedCmd struct {
	CommandID    string     `json:"command_id"`
	RawText      string     `json:"raw_text"`
	Action       Action     `json:"action"`
	FromUsername string     `json:"from_username,omitempty"`
	ExecutedAt   time.Time  `json:"executed_at"`
	ChatToExecMs int64      `json:"chat_to_exec_ms"`
	Result       ExecResult `json:"result"`
	ErrorMessage string     `json:"error_message,omitempty"`
}

type ExecResult string

const (
	ExecResultOK    ExecResult = "ok"
	ExecResultError ExecResult = "error"
)

// FortressStats are read from DFHack. Nil when DFHack/DF are unavailable.
// Field set is intentionally small for v1; extend as the dashboard grows.
type FortressStats struct {
	Name       string `json:"name"`
	Year       int    `json:"year"`
	Season     string `json:"season"`
	Population int    `json:"population"`
	FPS        int    `json:"fps"`
	Wealth     int    `json:"wealth"`
}

// LinkHealth is the executor's view of each link in the pipeline.
// TwitchChat status reflects what the executor has been told by the Pi
// (populated via Pi-to-executor health messages; mechanism TBD).
type LinkHealth struct {
	TwitchChat   LinkStatus `json:"twitch_chat"`
	PiToExecutor LinkStatus `json:"pi_to_executor"`
	ExecutorToPi LinkStatus `json:"executor_to_pi"`
	DFHack       LinkStatus `json:"dfhack"`
}

type LinkStatus string

const (
	LinkStatusOK       LinkStatus = "ok"
	LinkStatusDegraded LinkStatus = "degraded"
	LinkStatusDown     LinkStatus = "down"
)

// --- Envelope ---

// Envelope wraps every message on every WebSocket in the system.
//
// On internal links (Pi↔Executor, Pi↔Vultr) HMAC is set and verified against a
// shared secret per link. Receiver rejects any envelope with seq <= last seen on
// this connection (replay protection on top of TCP-FIFO ordering).
//
// On the public Vultr→browser fanout, Vultr omits the HMAC field entirely.
type Envelope struct {
	Type   EnvelopeType    `json:"type"`
	Seq    uint64          `json:"seq"`
	SentAt time.Time       `json:"sent_at"`
	HMAC   string          `json:"hmac,omitempty"`
	Data   json.RawMessage `json:"data"`
}

type EnvelopeType string

const (
	EnvelopeTypeCommand       EnvelopeType = "command"
	EnvelopeTypeStateSnapshot EnvelopeType = "state_snapshot"
	EnvelopeTypeExecutedCmd   EnvelopeType = "executed_cmd"
)
