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

	// For Kind == ActionKindCamera (and future spatial actions):
	Position *Position `json:"position,omitempty"`

	// For Kind == ActionKindMine / ActionKindChannel / ActionKindDigRamp /
	// ActionKindCutTree (and future range-based spatial actions):
	Region *Region `json:"region,omitempty"`

	// For Kind == ActionKindAppoint:
	Office string `json:"office,omitempty"`  // noble-position keyword (manager, bookkeeper, broker, doctor, commander)
	UnitID int    `json:"unit_id,omitempty"` // target dwarf's DFHack unit.id (stable, rename-proof)

	// For Kind == ActionKindCraft:
	WorkshopID int `json:"workshop_id,omitempty"` // target workshop's DFHack building.id, surfaced as #id on footprints
}

// Position is a tile coordinate as chatters see and type them. X and Y are
// embark-local; Z is the in-game ELEVATION the dashboard displays — NOT the
// raw embark-local z. The executor converts to raw z at the DFHack boundary
// (raw_z = Z - (map.region_z - 100)), so wire consumers above the executor
// stay in dashboard-native coords and copy-paste works without translation.
type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
}

// Region is a 2D rectangular area on a single Z level. Min and Max are
// inclusive corners; Min.Z and Max.Z MUST be equal (single-Z constraint
// enforced at parse time). Reusable across any verb that operates on a
// range of tiles — `mine` is the first user; channeling, stockpile-zone
// placement, and dig-down patterns will reuse the same shape later.
type Region struct {
	Min Position `json:"min"`
	Max Position `json:"max"`
}

// --- MapSnapshot ---

// MapSnapshot is a frozen view of a range of Z levels — a vertical slab of
// the fortress at a moment in time. Produced by the executor (eventually
// via a DFHack lua dump; currently fixture-fed), forwarded through the Pi
// byte-for-byte, fanned out by Vultr to dashboard viewers.
//
// All Z levels share the same X/Y extent (Width × Height) anchored at
// Origin.X / Origin.Y; only the Z layers themselves vary. The dashboard
// loads all levels in one fetch and switches between them client-side
// (no per-Z network call) — keeps Z navigation snappy at the cost of
// fatter snapshot payloads. Acceptable at v1 scale; revisit if real
// DFHack data with 100+ Z levels makes the payload painful.
type MapSnapshot struct {
	CapturedAt time.Time `json:"captured_at"`
	Origin     Position  `json:"origin"`             // X, Y are valid for all levels; Z = lowest level's Z
	Width      int       `json:"width"`              // number of tiles along X (same for every level)
	Height     int       `json:"height"`             // number of tiles along Y (same for every level)
	Levels     []ZLevel  `json:"levels"`             // sorted ascending by Z, contiguous (no gaps)
	ZOffset    int       `json:"z_offset"`           // add to raw z to get in-game elevation: elev = z + ZOffset. Computed from world map.region_z - 100 (DF's sea-level reference).
	Citizens   []Citizen `json:"citizens,omitempty"` // active citizen units in the fortress
	Focus      *Position `json:"focus,omitempty"`    // F1 map-hotkey location; dashboard centers its initial view here. nil when unset.
}

// Citizen is a fortress dwarf (or other citizen race) the dashboard
// surfaces in its sidebar. Set is intentionally small — name/profession/
// job + position cover the "who's doing what and where" question; stress
// gives a visible mood signal. Skills, attributes, relationships, health
// detail, equipment etc. are deliberately not included to keep the
// snapshot payload bounded.
type Citizen struct {
	ID         int      `json:"id"` // DFHack unit.id — stable, unique, rename-proof; how chat targets a dwarf
	Name       string   `json:"name"`
	Profession string   `json:"profession"`    // e.g. "Miner", "Carpenter", "Recruit", "Child"
	Age        int      `json:"age"`           // integer years
	Job        string   `json:"job,omitempty"` // current task; "" when idle
	Stress     int      `json:"stress"`        // dfhack stress category: 0=most stressed (miserable) .. 6=least stressed (ecstatic)
	Position   Position `json:"position"`      // current tile coord
}

// ZLevel is one floor of the fortress at a specific Z coordinate.
type ZLevel struct {
	Z          int              `json:"z"`
	Tiles      []TileType       `json:"tiles"`                // row-major: index = y*Width + x
	Furniture  []FurniturePlace `json:"furniture"`            // placed objects on this Z, world coords (single-tile)
	Footprints []Footprint      `json:"footprints,omitempty"` // multi-tile buildings (workshops, furnaces, stockpiles)
}

// Footprint is a multi-tile rectangular building drawn as a tinted region
// on the dashboard. Distinct from FurniturePlace, which is single-tile
// glyph overlays. Kind discriminates the visual style and label color;
// Subtype is the chat-facing name for the specific workshop/furnace
// (matching the !DF place vocab) or category for stockpiles. Empty
// Subtype is allowed — the renderer falls back to Kind as the label.
type Footprint struct {
	ID      int    `json:"id"`      // DFHack building.id — stable handle chat uses to target this workshop / stockpile
	Kind    string `json:"kind"`    // "workshop", "furnace", "stockpile"
	Subtype string `json:"subtype"` // workshop/furnace chat name (e.g. "carpenter", "smelter"), or "" when unknown
	X1      int    `json:"x1"`
	Y1      int    `json:"y1"`
	X2      int    `json:"x2"`
	Y2      int    `json:"y2"`
}

// TileType is the v0 set of tile shapes the dashboard renders. Intentionally
// small — extend as DFHack reveals what's worth distinguishing visually.
//
// Underlying type is `int` (NOT `uint8`) on purpose: Go's encoding/json
// treats `[]uint8` (and any named type with that underlying) as a byte
// slice and base64-encodes it on the wire. That breaks the dashboard
// renderer, which expects an array of numbers. Using `int` ensures
// `[]TileType` JSON-encodes as a normal array.
type TileType int

const (
	TileUnknown TileType = 0
	TileWall    TileType = 1
	TileFloor   TileType = 2
	TileRamp    TileType = 3
	TileStair   TileType = 4
	TileWater   TileType = 5
	TileMagma   TileType = 6
	TileTree    TileType = 7
)

// FurniturePlace is a single piece of furniture on the snapshot's Z level.
// Coordinates are world coords (not offsets into the Tiles array).
type FurniturePlace struct {
	Type     string `json:"type"`     // chat vocab: "table", "bed", "door", ...
	Material string `json:"material"` // chat vocab: "stone", "wood", ...
	X        int    `json:"x"`
	Y        int    `json:"y"`
}

type ActionKind string

const (
	ActionKindManufacture ActionKind = "manufacture"
	ActionKindPause       ActionKind = "pause"
	ActionKindUnpause     ActionKind = "unpause"
	ActionKindCamera      ActionKind = "camera"
	ActionKindHelp        ActionKind = "help"
	ActionKindPlace       ActionKind = "place"
	ActionKindBrew        ActionKind = "brew"
	ActionKindMine        ActionKind = "mine"
	ActionKindChannel     ActionKind = "channel"
	ActionKindDigRamp     ActionKind = "digramp"
	ActionKindCutTree     ActionKind = "cuttree"
	ActionKindStockpile   ActionKind = "stockpile"
	ActionKindCraft       ActionKind = "craft"
	ActionKindAppoint     ActionKind = "appoint"
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
