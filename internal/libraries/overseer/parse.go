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
	"table":         {},
	"bed":           {},
	"door":          {},
	"chair":         {},
	"throne":        {}, // synonym for chair (DF's internal name); both -> ConstructThrone
	"coffin":        {},
	"block":         {},
	"cabinet":       {},
	"chest":         {},
	"statue":        {},
	"floodgate":     {},
	"bucket":        {},
	"barrel":        {},
	"ash":           {},
	"charcoal":      {},
	"armorstand":    {},
	"grate":         {},
	"hatchcover":    {},
	"millstone":     {},
	"quern":         {},
	"slab":          {},
	"weaponrack":    {},
	"altar":         {},
	"bookcase":      {},
	"pedestal":      {},
	"cage":          {},
	"animaltrap":    {},
	"bin":           {},
	"crutch":        {},
	"splint":        {},
	"pipesection":   {},
	"minecart":      {},
	"stepladder":    {},
	"wheelbarrow":   {},
	"shield":        {},
	"buckler":       {},
	"trainingaxe":   {},
	"trainingspear": {},
	"trainingsword": {},
	"corkscrew":     {},
	"menacingspike": {},
	"spikedball":    {},
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

// (Placeable items live in placeableItemToBuilding in service.go — same
// package — which is the single source of truth for both parse-time
// validation and the building_type/subtype used at construction.)

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
//   - `camera <x> <y> <z>` — recenter DF camera on the given tile. Z is
//     the in-game ELEVATION the dashboard displays (executor converts
//     internally). Coord tolerance: bare `137 115 150`, comma `137,115,150`,
//     and dashboard-paste `(137, 115, 150)` all parse the same
//   - `help` — bot posts the command list in chat (short-circuited in
//     handleDFCommand; no executor round-trip)
//   - `place <item> <x> <y> <z>` — queue a build job at the given tile for
//     a furniture/workshop item. Z is elevation, same coord-format
//     tolerance as `camera`. Only items in placeableItemToBuilding are
//     accepted (most manufacturable items; not block/bucket/barrel/ash/
//     charcoal, which aren't buildings)
//   - `brew [qty] <source>` — queue a brew-drink workorder. Source is
//     `fruit` (BREW_DRINK_FROM_PLANT_GROWTH) or `plant`
//     (BREW_DRINK_FROM_PLANT). Verbose chatter-friendly forms like
//     `brew drink from fruit` parse the same because `drink` and `from`
//     are filler-stripped
//   - `mine <x,y,z>` or `mine <x1,y1,z> <x2,y2[,z]>` — designate a dig area
//     on a single Z level. Z is elevation. With one coord, designates a
//     single tile; with two, the rectangle spanning the corners. The
//     second coord's Z is optional and always ignored, so a dashboard
//     paste like `(97,87,35) (97,88,35)` is accepted but the second 35
//     has no effect — multi-Z mining stays impossible to express. Area
//     is capped at 100 tiles per command to keep individual jobs bounded
//   - `channel <x,y,z>` / `channel <x1,y1,z> <x2,y2[,z]>` — same shape and
//     constraints as `mine`, but applies the channel dig designation
//     (carves the tile down a level, leaving a ramp below)
//   - `digramp <x,y,z>` / `digramp <x1,y1,z> <x2,y2[,z]>` — same shape and
//     constraints as `mine`, but applies the ramp dig designation (carves
//     out the tile as an upward ramp, exposing the floor above)
//   - `cuttree <x,y,z>` / `cuttree <x1,y1,z> <x2,y2[,z]>` — same shape and
//     constraints as `mine`, but designates only tree-shape tiles (trunk,
//     branches, twigs, saplings) inside the region for chopping. Wall
//     tiles are skipped, so `cuttree` over a region that mixes trees and
//     walls won't accidentally dig the walls. Errors if no trees were
//     found in the region
//   - `stockpile <category> <x,y,z>` / `stockpile <category> <x1,y1,z>
//     <x2,y2[,z]>` — builds an abstract stockpile covering the region and
//     restricts it to a single top-level category. Categories: ammo,
//     animal, armor, bar, cloth, coin, corpse, good, food, furniture,
//     gem, leather, refuse, sheet, stone, weapon, wood, plus `all` for
//     a stockpile that accepts every default top-level category. Same
//     coord tolerances and 100-tile cap as the dig verbs
//   - `craft <workshop_id> [qty] <material> <item>` — queue a job
//     directly into a specific workshop, bypassing the fortress manager
//     queue. Useful pre-manager (workorders need a manager to process).
//     Workshop id matches the `#N` shown on the dashboard footprint
//     label; a leading `#` is tolerated. Materials supported in v1: wood,
//     stone, bone, leather, cloth, shell, metal — specific metal types
//     (iron, copper, etc.) require raws lookup, deferred. The item must
//     not be a fixed-recipe one (ash/charcoal — use `make` instead)
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
	case "channel":
		return parseChannel(rest)
	case "digramp":
		return parseDigRamp(rest)
	case "cuttree":
		return parseCutTree(rest)
	case "stockpile":
		return parseStockpile(rest)
	case "craft":
		return parseCraft(rest)
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

// extractCoordInts pulls all integer tokens out of a coord-list. Tolerates
// the formats chatters actually type or copy off the dashboard:
//   - bare:               `137 115 150`
//   - commas:             `137,115,150` / `137, 115, 150`
//   - dashboard parens:   `(137, 115, 150)` / `(137,115,150) (142,118,150)`
//
// Strips `(`, `)`, and `,`, then splits on whitespace and Atoi's each token.
// Returns the integers in the order the chatter wrote them; callers decide
// how many they expect and what each slot means.
func extractCoordInts(rest []string) ([]int, error) {
	joined := strings.Join(rest, " ")
	for _, ch := range []string{"(", ")", ","} {
		joined = strings.ReplaceAll(joined, ch, " ")
	}
	parts := strings.Fields(joined)
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("invalid coordinate %q", p)
		}
		out[i] = n
	}
	return out, nil
}

func parseCamera(rest []string) (Action, error) {
	coords, err := extractCoordInts(rest)
	if err != nil {
		return Action{}, err
	}
	if len(coords) != 3 {
		return Action{}, fmt.Errorf("camera needs 3 coordinates, got %d", len(coords))
	}
	return Action{
		Kind:     ActionKindCamera,
		Position: &Position{X: coords[0], Y: coords[1], Z: coords[2]},
	}, nil
}

// regionVerbMaxArea is the per-command cap on the rectangular tile area
// for region-based dig verbs (mine, channel, digramp). Keeps individual
// commands bounded.
const regionVerbMaxArea = 100

// parseRegionVerb parses the coord-list shared by all rectangular dig verbs.
// Accepts:
//   - 3 ints — single tile (x,y,z); 1×1 region
//   - 5 ints — legacy two-corner form: x1,y1,z + x2,y2
//   - 6 ints — dashboard copy-paste: (x1,y1,z) + (x2,y2,z); the second z is
//     silently discarded to preserve the single-Z invariant
//
// Anything else rejects. verb is the chat-facing verb name, used only in
// error messages.
func parseRegionVerb(verb string, rest []string) (*Region, error) {
	coords, err := extractCoordInts(rest)
	if err != nil {
		return nil, err
	}
	var x1, y1, z, x2, y2 int
	switch len(coords) {
	case 3:
		// Single-tile form: second corner equals the first.
		x1, y1, z = coords[0], coords[1], coords[2]
		x2, y2 = x1, y1
	case 5, 6:
		x1, y1, z, x2, y2 = coords[0], coords[1], coords[2], coords[3], coords[4]
		// coords[5], if present, is the second coord's z — intentionally
		// ignored so multi-Z designations stay impossible to express.
	default:
		return nil, fmt.Errorf("%s needs (x,y,z) or (x1,y1,z) (x2,y2[,z]) — 3, 5, or 6 numbers total, got %d", verb, len(coords))
	}

	dx := abs(x2-x1) + 1
	dy := abs(y2-y1) + 1
	area := dx * dy
	if area > regionVerbMaxArea {
		return nil, fmt.Errorf("%s area %dx%d=%d tiles exceeds %d-tile cap", verb, dx, dy, area, regionVerbMaxArea)
	}

	return &Region{
		Min: Position{X: x1, Y: y1, Z: z},
		Max: Position{X: x2, Y: y2, Z: z}, // Z inherits from first coord
	}, nil
}

func parseMine(rest []string) (Action, error) {
	region, err := parseRegionVerb("mine", rest)
	if err != nil {
		return Action{}, err
	}
	return Action{Kind: ActionKindMine, Region: region}, nil
}

func parseChannel(rest []string) (Action, error) {
	region, err := parseRegionVerb("channel", rest)
	if err != nil {
		return Action{}, err
	}
	return Action{Kind: ActionKindChannel, Region: region}, nil
}

func parseDigRamp(rest []string) (Action, error) {
	region, err := parseRegionVerb("digramp", rest)
	if err != nil {
		return Action{}, err
	}
	return Action{Kind: ActionKindDigRamp, Region: region}, nil
}

func parseCutTree(rest []string) (Action, error) {
	region, err := parseRegionVerb("cuttree", rest)
	if err != nil {
		return Action{}, err
	}
	return Action{Kind: ActionKindCutTree, Region: region}, nil
}

// parseCraft handles `craft <workshop_id> [qty] <material> <item>` — queue
// a job directly into a specific workshop's task list, bypassing the
// manager queue. Used to bootstrap a fort before a manager exists.
// Workshop id matches the #N on the dashboard footprint label. A leading
// `#` on the id is tolerated. Material/item vocab and the optional qty
// follow the same shape as `make`.
func parseCraft(tokens []string) (Action, error) {
	if len(tokens) < 3 {
		return Action{}, fmt.Errorf("craft needs <workshop_id> [qty] <material> <item>")
	}
	wsTok := strings.TrimPrefix(tokens[0], "#")
	wsID, err := strconv.Atoi(wsTok)
	if err != nil || wsID < 0 {
		return Action{}, fmt.Errorf("invalid workshop id: %q", tokens[0])
	}
	tokens = tokens[1:]

	qty := 1
	if n, parseErr := strconv.Atoi(tokens[0]); parseErr == nil {
		if n <= 0 {
			return Action{}, fmt.Errorf("quantity must be positive")
		}
		qty = n
		tokens = tokens[1:]
	}
	if len(tokens) < 2 {
		return Action{}, fmt.Errorf("craft needs material and item after workshop id")
	}

	itemToken := strings.TrimSuffix(tokens[len(tokens)-1], "s")
	if _, ok := itemVocab[itemToken]; !ok {
		return Action{}, fmt.Errorf("unknown item: %q", itemToken)
	}
	if _, na := itemMaterialNotApplicable[itemToken]; na {
		return Action{}, fmt.Errorf("item %q has a fixed recipe — use !DF make, not craft", itemToken)
	}

	pre := tokens[:len(tokens)-1]
	if len(pre) != 1 {
		return Action{}, fmt.Errorf("craft needs exactly one material token, got %q", strings.Join(pre, " "))
	}
	matToken := normalizeMaterial(pre[0])
	if _, ok := materialVocab[matToken]; !ok {
		return Action{}, fmt.Errorf("unknown material: %q", pre[0])
	}

	return Action{
		Kind:       ActionKindCraft,
		Item:       itemToken,
		Material:   &matToken,
		Quantity:   qty,
		WorkshopID: wsID,
	}, nil
}

func parseStockpile(rest []string) (Action, error) {
	if len(rest) == 0 {
		return Action{}, fmt.Errorf("stockpile needs <category> <coords>")
	}
	category := strings.TrimSuffix(rest[0], "s")
	if _, ok := stockpileCategoryToPreset[category]; !ok {
		return Action{}, fmt.Errorf("unknown stockpile category: %q", rest[0])
	}
	region, err := parseRegionVerb("stockpile", rest[1:])
	if err != nil {
		return Action{}, err
	}
	return Action{
		Kind:   ActionKindStockpile,
		Item:   category,
		Region: region,
	}, nil
}

// abs returns |n|. Used by parseRegionVerb to compute rectangle dimensions
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
	if len(rest) == 0 {
		return Action{}, fmt.Errorf("place needs <item> <x> <y> <z>")
	}
	item := strings.TrimSuffix(rest[0], "s")
	if _, ok := placeableItemToBuilding[item]; !ok {
		return Action{}, fmt.Errorf("not placeable: %q", item)
	}
	coords, err := extractCoordInts(rest[1:])
	if err != nil {
		return Action{}, err
	}
	if len(coords) != 3 {
		return Action{}, fmt.Errorf("place needs 3 coordinates after item, got %d", len(coords))
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
