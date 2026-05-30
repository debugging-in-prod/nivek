package verbs

import (
	"fmt"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// zoneTypeToCivzoneEnum maps chat-facing zone-type keywords to DFHack
// df.civzone_type enum names. v1 covers office, bedroom, dormitory —
// the room-designation set originally requested. Easy to extend later
// with dininghall, meetinghall, barracks, tomb, etc. (the full list is
// in df.civzone_type).
var zoneTypeToCivzoneEnum = map[string]string{
	"office":    "Office",
	"bedroom":   "Bedroom",
	"dormitory": "Dormitory",
}

// ParseZone handles `zone <type> <coords>` — designates a rectangular
// fortress room. v1 types: office, bedroom, dormitory; same coord
// tolerances and 100-tile cap as the dig verbs.
func ParseZone(rest []string) (wire.Action, error) {
	if len(rest) == 0 {
		return wire.Action{}, fmt.Errorf("zone needs <type> <coords>")
	}
	zoneType := strings.TrimSuffix(rest[0], "s")
	if _, ok := zoneTypeToCivzoneEnum[zoneType]; !ok {
		return wire.Action{}, fmt.Errorf("unknown zone type: %q (try office, bedroom, dormitory)", rest[0])
	}
	region, err := parseRegionVerb("zone", rest[1:])
	if err != nil {
		return wire.Action{}, err
	}
	return wire.Action{
		Kind:   wire.ActionKindZone,
		Item:   zoneType,
		Region: region,
	}, nil
}

// SubmitZone builds an abstract Civzone covering the region and sets
// its type (df.civzone_type) to the requested room kind. The created
// zone is unowned — for offices, a noble's `assigned_unit_id` would be
// set in a separate appoint/assign flow.
func SubmitZone(ex Executor, action wire.Action) error {
	if action.Region == nil {
		return fmt.Errorf("zone requires region")
	}
	zoneEnum, ok := zoneTypeToCivzoneEnum[action.Item]
	if !ok {
		return fmt.Errorf("no DFHack civzone_type mapping for zone: %s", action.Item)
	}
	r := action.Region
	if r.Min.Z != r.Max.Z {
		return fmt.Errorf("zone region must be on a single Z level (got Min.Z=%d Max.Z=%d)", r.Min.Z, r.Max.Z)
	}
	minX, maxX := r.Min.X, r.Max.X
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minY, maxY := r.Min.Y, r.Max.Y
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	script := fmt.Sprintf(`
local rawz = %d - (df.global.world.map.region_z - 100)
local z = dfhack.buildings.constructBuilding{
    type = df.building_type.Civzone,
    abstract = true,
    pos = {x=%d, y=%d, z=rawz},
    width = %d, height = %d,
}
if not z then error("constructBuilding returned nil — bad spot or blocked") end
z.type = df.civzone_type.%s`,
		r.Min.Z, minX, minY, maxX-minX+1, maxY-minY+1, zoneEnum)
	return ex.RunLua(script)
}
