package overseer

import (
	"fmt"
	"strconv"
	"strings"
)

// itemVocab is the set of chat-facing item nouns the parser recognizes for v0.
// Plural and case variations are normalized before lookup (the trailing "s"
// is stripped before lookup, so all entries are singular even when the
// DFHack-side job_type is plural — e.g. `block` -> `ConstructBlocks`).
var itemVocab = map[string]struct{}{
	"table":      {},
	"bed":        {},
	"door":       {},
	"chair":      {},
	"throne":     {}, // synonym for chair (DF's internal name); both -> ConstructThrone
	"coffin":     {},
	"block":      {},
	"cabinet":    {},
	"chest":      {},
	"statue":     {},
	"floodgate":  {},
	"bucket":     {},
	"barrel":     {},
	"ash":        {},
	"charcoal":   {},
	"armorstand": {},
	"grate":      {},
	"hatchcover": {},
	"millstone":  {},
	"quern":      {},
	"slab":       {},
	"weaponrack": {},
	"altar":      {},
	"bookcase":   {},
	"pedestal":   {},
}

// itemMaterialNotApplicable lists items where the DFHack job_type doesn't
// take a material spec — the recipe has fixed inputs/outputs (e.g. MakeAsh
// burns wood and produces ash bars regardless of wood type; the DF Manager
// job has no material slot). For these items the parser allows the
// material to be omitted, and silently ignores any material the chatter
// supplies; the service-side skips populating the workorder JSON's
// material field.
var itemMaterialNotApplicable = map[string]struct{}{
	"ash":      {},
	"charcoal": {},
}

// itemMaterialAllowlist restricts which materials are accepted for specific
// items where DF has a strict requirement that would otherwise queue an
// unbuildable order. Items not present here accept any material in
// materialVocab (the loose default — DF rejects the impossibility itself).
var itemMaterialAllowlist = map[string]map[string]struct{}{
	"bed":    {"wood": {}},
	"barrel": {"wood": {}},
}

// placeableItemVocab is the subset of itemVocab that can be constructed as
// buildings via `!DF place`. Tools (bucket, barrel) and bulk products
// (block) are intentionally excluded — DF doesn't construct them as
// tile-occupying furniture, only as items.
var placeableItemVocab = map[string]struct{}{
	"table":     {},
	"bed":       {},
	"door":      {},
	"chair":     {},
	"coffin":    {},
	"cabinet":   {},
	"chest":     {},
	"statue":    {},
	"floodgate": {},
}

// nobleVocab is the set of chat-facing noble-position keywords `!DF appoint`
// accepts. Each maps to a single-slot fort position; the keyword→DFHack
// position code translation lives service-side in officeToPositionCode.
var nobleVocab = map[string]struct{}{
	"manager":    {},
	"bookkeeper": {},
	"broker":     {},
	"doctor":     {}, // chief medical dwarf
	"commander":  {}, // militia commander
}

// nobleDeferred maps recognized-but-not-yet-supported position keywords to a
// user-facing reason. Militia captain is squad-dependent (a captain leads a
// squad, which must exist first) — handling it properly needs squad
// management we haven't built, so it gets a clear "not supported yet" rather
// than an "unknown position" error.
var nobleDeferred = map[string]string{
	"captain": "captain needs a squad — not supported yet",
}

// materialVocab is the set of chat-facing materials accepted in v0.
// "metal" is a meta-token meaning "any available metal, executor picks".
var materialVocab = map[string]struct{}{
	"stone": {}, "wood": {}, "bone": {}, "leather": {}, "cloth": {}, "shell": {},
	"iron": {}, "copper": {}, "bronze": {}, "steel": {}, "silver": {}, "gold": {},
	"metal": {},
}

// materialAliases normalizes adjectival forms to the canonical material token.
var materialAliases = map[string]string{
	"wooden": "wood",
}

// fillerWords are chat-tolerance tokens stripped before grammar matching.
// `drink` and `from` are stripped specifically so the verbose brew phrasing
// (`brew drink from fruit`) works in addition to the canonical short form
// (`brew fruit`). They're benign for other verbs — no existing grammar
// slot needs to preserve them as data.
var fillerWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "some": {},
	"me": {}, "us": {}, "please": {},
	"drink": {}, "from": {},
}

// RejectReason is a parse error whose message is safe and intended to be
// shown to the chatter. Ordinary parse errors are silently dropped (locked
// design — keeps chat clean of typo/garbage feedback); a RejectReason is the
// deliberate exception for a recognized-but-unsupported command, where a
// short "why" is more helpful than silence. Currently only `appoint captain`.
type RejectReason struct{ Msg string }

func (e *RejectReason) Error() string { return e.Msg }

// ParseCommand parses the arguments of a `!DF` chat command into an Action.
// The caller is expected to have stripped the `!df` prefix before passing.
//
// Verbs (v0):
//   - `make [qty] <material> <item>` — manufacture (material is required;
//     DF Manager refuses to execute orders without one)
//   - `pause` — pause DF
//   - `unpause` — unpause DF
//   - `camera <x> <y> <z>` — recenter DF camera on the given tile (coords
//     accept space- and/or comma-separated forms: `137 115 150`,
//     `137,115,150`, `137, 115, 150` all parse the same)
//   - `help` — bot posts the command list in chat (short-circuited in
//     handleDFCommand; no executor round-trip)
//   - `place <item> <x> <y> <z>` — queue a build job at the given tile
//     for an already-manufactured furniture item (commas optional, same
//     coord-format tolerance as `camera`). Only items in
//     placeableItemVocab are accepted
//   - `brew [qty] <source>` — queue a brew-drink workorder. Source is
//     `fruit` (BREW_DRINK_FROM_PLANT_GROWTH) or `plant`
//     (BREW_DRINK_FROM_PLANT). Verbose chatter-friendly forms like
//     `brew drink from fruit` parse the same because `drink` and `from`
//     are filler-stripped
//   - `mine <x1,y1,z> <x2,y2>` — designate a rectangular dig area on a
//     single Z level. Z is only specified in the first coord; the second
//     coord's Z is implicit (prevents multi-Z mining commands by design).
//     Area is capped at 25 tiles per command to keep individual jobs
//     bounded
//   - `appoint <position> <id>` — assign a dwarf (by its stable unit.id,
//     shown on the /df/citizens page) to a fort noble position. Positions:
//     manager, bookkeeper, broker, doctor, commander. `captain` is
//     recognized but deferred (needs squad management). Token order is
//     flexible and a leading `#` on the id is tolerated
//
// Tolerances (apply to all verbs): case-insensitive, whitespace-collapsing,
// filler-word stripping (a, an, the, some, me, us, please). Manufacture
// additionally allows plural-stripping on the item and the wooden->wood
// adjectival alias on the material.
func ParseCommand(args string) (Action, error) {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(args)))
	tokens = stripFillerWords(tokens)
	if len(tokens) == 0 {
		return Action{}, fmt.Errorf("empty command")
	}
	verb := tokens[0]
	rest := tokens[1:]
	switch verb {
	case "make":
		return parseManufacture(rest)
	case "pause":
		return parsePause(rest)
	case "unpause":
		return parseUnpause(rest)
	case "camera":
		return parseCamera(rest)
	case "help":
		return parseHelp(rest)
	case "place":
		return parsePlace(rest)
	case "brew":
		return parseBrew(rest)
	case "mine":
		return parseMine(rest)
	case "appoint":
		return parseAppoint(rest)
	default:
		return Action{}, fmt.Errorf("unknown verb: %q", verb)
	}
}

func parseManufacture(tokens []string) (Action, error) {
	if len(tokens) == 0 {
		return Action{}, fmt.Errorf("missing item")
	}

	qty := 1
	if n, parseErr := strconv.Atoi(tokens[0]); parseErr == nil {
		if n <= 0 {
			return Action{}, fmt.Errorf("quantity must be positive")
		}
		qty = n
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return Action{}, fmt.Errorf("missing item")
	}

	itemToken := strings.TrimSuffix(tokens[len(tokens)-1], "s")
	if _, ok := itemVocab[itemToken]; !ok {
		return Action{}, fmt.Errorf("unknown item: %q", itemToken)
	}

	_, materialNotApplicable := itemMaterialNotApplicable[itemToken]

	var material *string
	pre := tokens[:len(tokens)-1]
	switch len(pre) {
	case 0:
		// DF Manager requires a material for most items — orders without one
		// queue as "unknown material" and can never execute. Items in
		// itemMaterialNotApplicable are the exception (fixed-recipe jobs).
		if !materialNotApplicable {
			return Action{}, fmt.Errorf("missing material")
		}
	case 1:
		if materialNotApplicable {
			// Material isn't used for this item; silently ignore whatever
			// the chatter put there rather than rejecting valid commands.
			break
		}
		matToken := normalizeMaterial(pre[0])
		if _, ok := materialVocab[matToken]; !ok {
			return Action{}, fmt.Errorf("unknown material: %q", pre[0])
		}
		material = &matToken
	default:
		return Action{}, fmt.Errorf("extra tokens: %q", strings.Join(pre, " "))
	}

	if !materialNotApplicable {
		if allowed, restricted := itemMaterialAllowlist[itemToken]; restricted {
			if _, ok := allowed[*material]; !ok {
				return Action{}, fmt.Errorf("material %q not allowed for item %q", *material, itemToken)
			}
		}
	}

	return Action{
		Kind:     ActionKindManufacture,
		Item:     itemToken,
		Material: material,
		Quantity: qty,
	}, nil
}

func parsePause(rest []string) (Action, error) {
	if len(rest) > 0 {
		return Action{}, fmt.Errorf("extra tokens: %q", strings.Join(rest, " "))
	}
	return Action{Kind: ActionKindPause}, nil
}

func parseUnpause(rest []string) (Action, error) {
	if len(rest) > 0 {
		return Action{}, fmt.Errorf("extra tokens: %q", strings.Join(rest, " "))
	}
	return Action{Kind: ActionKindUnpause}, nil
}

func parseHelp(rest []string) (Action, error) {
	if len(rest) > 0 {
		return Action{}, fmt.Errorf("extra tokens: %q", strings.Join(rest, " "))
	}
	return Action{Kind: ActionKindHelp}, nil
}

func parseCamera(rest []string) (Action, error) {
	// Accept commas as separators in addition to whitespace:
	// `137,115,150`, `137, 115, 150`, `137 115 150` all parse the same.
	joined := strings.ReplaceAll(strings.Join(rest, " "), ",", " ")
	parts := strings.Fields(joined)
	if len(parts) != 3 {
		return Action{}, fmt.Errorf("camera needs 3 coordinates, got %d", len(parts))
	}
	coords := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return Action{}, fmt.Errorf("invalid coordinate %q", p)
		}
		coords[i] = n
	}
	return Action{
		Kind:     ActionKindCamera,
		Position: &Position{X: coords[0], Y: coords[1], Z: coords[2]},
	}, nil
}

// mineMaxArea is the per-command cap on the rectangular dig area
// (tiles). Keeps individual commands bounded.
const mineMaxArea = 25

func parseMine(rest []string) (Action, error) {
	// Liberal coord parsing: commas → spaces, then we expect exactly 5 ints
	// (x1, y1, z) + (x2, y2). The single-Z constraint is encoded by NOT
	// asking for a second Z — second coord inherits the first's Z.
	joined := strings.ReplaceAll(strings.Join(rest, " "), ",", " ")
	parts := strings.Fields(joined)
	if len(parts) != 5 {
		return Action{}, fmt.Errorf("mine needs <x1,y1,z> <x2,y2> — 5 numbers total, got %d", len(parts))
	}
	coords := make([]int, 5)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return Action{}, fmt.Errorf("invalid coordinate %q", p)
		}
		coords[i] = n
	}
	x1, y1, z, x2, y2 := coords[0], coords[1], coords[2], coords[3], coords[4]

	dx := abs(x2-x1) + 1
	dy := abs(y2-y1) + 1
	area := dx * dy
	if area > mineMaxArea {
		return Action{}, fmt.Errorf("mine area %dx%d=%d tiles exceeds %d-tile cap", dx, dy, area, mineMaxArea)
	}

	return Action{
		Kind: ActionKindMine,
		Region: &Region{
			Min: Position{X: x1, Y: y1, Z: z},
			Max: Position{X: x2, Y: y2, Z: z}, // Z inherits from first coord
		},
	}, nil
}

// abs returns |n|. Used by parseMine to compute rectangle dimensions
// regardless of which corner the chatter listed first — `mine 0,0,0 5,5`
// and `mine 5,5,0 0,0` describe the same region, so `Max - Min` can be
// negative. Go's math.Abs is float64-only, so a 5-line int helper is
// cheaper than the conversion dance.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func parseBrew(tokens []string) (Action, error) {
	if len(tokens) == 0 {
		return Action{}, fmt.Errorf("brew needs a source (fruit or plant)")
	}
	qty := 1
	if n, parseErr := strconv.Atoi(tokens[0]); parseErr == nil {
		if n <= 0 {
			return Action{}, fmt.Errorf("quantity must be positive")
		}
		qty = n
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return Action{}, fmt.Errorf("brew needs a source (fruit or plant)")
	}
	// Source is the last token (plural-stripped); anything before it is
	// unexpected and gets rejected so typos don't quietly succeed.
	source := strings.TrimSuffix(tokens[len(tokens)-1], "s")
	if _, ok := brewSourceToReaction[source]; !ok {
		return Action{}, fmt.Errorf("unknown brew source: %q (expected fruit or plant)", source)
	}
	if len(tokens) > 1 {
		return Action{}, fmt.Errorf("extra tokens: %q", strings.Join(tokens[:len(tokens)-1], " "))
	}
	return Action{
		Kind:     ActionKindBrew,
		Item:     source,
		Quantity: qty,
	}, nil
}

func parsePlace(rest []string) (Action, error) {
	// Tolerate comma separators between coords, same as camera.
	joined := strings.ReplaceAll(strings.Join(rest, " "), ",", " ")
	parts := strings.Fields(joined)
	if len(parts) != 4 {
		return Action{}, fmt.Errorf("place needs <item> <x> <y> <z>, got %d args", len(parts))
	}
	item := strings.TrimSuffix(parts[0], "s")
	if _, ok := placeableItemVocab[item]; !ok {
		return Action{}, fmt.Errorf("not placeable: %q", item)
	}
	coords := make([]int, 3)
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(parts[i+1])
		if err != nil {
			return Action{}, fmt.Errorf("invalid coordinate %q", parts[i+1])
		}
		coords[i] = n
	}
	return Action{
		Kind:     ActionKindPlace,
		Item:     item,
		Position: &Position{X: coords[0], Y: coords[1], Z: coords[2]},
	}, nil
}

// parseAppoint handles `appoint <position> <id>` — assign a dwarf (by its
// stable DFHack unit.id) to a fort noble position. Tolerances:
//   - either token order: `appoint manager 8423` and `appoint 8423 manager`
//     both work (the numeric token is the id, the other is the position).
//   - a leading `#` on the id is stripped, so copy-pasting the `#8423` shown
//     on the citizens dashboard works verbatim.
func parseAppoint(tokens []string) (Action, error) {
	if len(tokens) != 2 {
		return Action{}, fmt.Errorf("appoint needs <position> <id>, e.g. 'appoint manager 8423'")
	}
	// Strip the dashboard's display `#` prefix off whichever token carries it.
	t0 := strings.TrimPrefix(tokens[0], "#")
	t1 := strings.TrimPrefix(tokens[1], "#")

	var posTok string
	var id int
	n0, e0 := strconv.Atoi(t0)
	n1, e1 := strconv.Atoi(t1)
	switch {
	case e0 != nil && e1 == nil:
		posTok, id = t0, n1
	case e0 == nil && e1 != nil:
		posTok, id = t1, n0
	default:
		return Action{}, fmt.Errorf("appoint needs exactly one position keyword and one numeric id")
	}
	if id < 0 {
		return Action{}, fmt.Errorf("invalid unit id: %d", id)
	}

	if reason, deferred := nobleDeferred[posTok]; deferred {
		// Chat-visible: a recognized position we just don't support yet.
		return Action{}, &RejectReason{Msg: reason}
	}
	if _, ok := nobleVocab[posTok]; !ok {
		return Action{}, fmt.Errorf("unknown position: %q (try manager, bookkeeper, broker, doctor, commander)", posTok)
	}

	return Action{
		Kind:   ActionKindAppoint,
		Office: posTok,
		UnitID: id,
	}, nil
}

func stripFillerWords(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if _, ok := fillerWords[t]; ok {
			continue
		}
		out = append(out, t)
	}
	return out
}

func normalizeMaterial(token string) string {
	if alias, ok := materialAliases[token]; ok {
		return alias
	}
	return token
}
