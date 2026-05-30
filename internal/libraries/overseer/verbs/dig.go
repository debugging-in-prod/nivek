package verbs

import (
	"fmt"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// Mine / Channel / DigRamp share the same region-verb parser shape and
// the same per-tile dig-designation lua loop; only the
// df.tile_dig_designation enum value applied differs. The parsers stamp
// the appropriate ActionKind; submitDigDesignation emits the shared lua.

func ParseMine(rest []string) (wire.Action, error) {
	region, err := parseRegionVerb("mine", rest)
	if err != nil {
		return wire.Action{}, err
	}
	return wire.Action{Kind: wire.ActionKindMine, Region: region}, nil
}

func ParseChannel(rest []string) (wire.Action, error) {
	region, err := parseRegionVerb("channel", rest)
	if err != nil {
		return wire.Action{}, err
	}
	return wire.Action{Kind: wire.ActionKindChannel, Region: region}, nil
}

func ParseDigRamp(rest []string) (wire.Action, error) {
	region, err := parseRegionVerb("digramp", rest)
	if err != nil {
		return wire.Action{}, err
	}
	return wire.Action{Kind: wire.ActionKindDigRamp, Region: region}, nil
}

func SubmitMine(ex Executor, action wire.Action) error {
	return submitDigDesignation(ex, action, "Default")
}

func SubmitChannel(ex Executor, action wire.Action) error {
	return submitDigDesignation(ex, action, "Channel")
}

func SubmitDigRamp(ex Executor, action wire.Action) error {
	return submitDigDesignation(ex, action, "Ramp")
}

// submitDigDesignation issues a rectangular dig designation across the
// region in action.Region. designation is the df.tile_dig_designation
// enum name to apply — "Default" (mine), "Channel" (channel), or "Ramp"
// (digramp). The shape — single-Z, raw-z conversion, per-tile block
// lookup — is identical across all three verbs; only the designation
// differs.
func submitDigDesignation(ex Executor, action wire.Action, designation string) error {
	if action.Region == nil {
		return fmt.Errorf("%s requires region", action.Kind)
	}
	r := action.Region
	if r.Min.Z != r.Max.Z {
		return fmt.Errorf("%s region must be on a single Z level (got Min.Z=%d Max.Z=%d)", action.Kind, r.Min.Z, r.Max.Z)
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
for x = %d, %d do
    for y = %d, %d do
        local block = dfhack.maps.getTileBlock({x=x, y=y, z=rawz})
        if block then
            block.designation[x%%16][y%%16].dig = df.tile_dig_designation.%s
            block.occupancy[x%%16][y%%16].dig_marked = false
            block.flags.designated = true
        end
    end
end`, r.Min.Z, minX, maxX, minY, maxY, designation)
	return ex.RunLua(script)
}
