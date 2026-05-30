package verbs

import (
	"fmt"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// ParseCutTree handles `cuttree <coords>` — same region-verb shape as
// mine/channel/digramp.
func ParseCutTree(rest []string) (wire.Action, error) {
	region, err := parseRegionVerb("cuttree", rest)
	if err != nil {
		return wire.Action{}, err
	}
	return wire.Action{Kind: wire.ActionKindCutTree, Region: region}, nil
}

// SubmitCutTree designates trees in the region for chopping. DF v50
// represents trees as plant entities in df.global.world.plants.tree_dry
// / tree_wet — the underlying tile at the trunk position is usually
// FLOOR (or whatever ground the tree sits on), NOT a tree-shape tile, so
// the only reliable way to find trees is iterating the plant vectors and
// setting the dig designation at each trunk position whose pos falls
// inside the region. Errors with "no trees in region" if zero matched.
func SubmitCutTree(ex Executor, action wire.Action) error {
	if action.Region == nil {
		return fmt.Errorf("cuttree requires region")
	}
	r := action.Region
	if r.Min.Z != r.Max.Z {
		return fmt.Errorf("cuttree region must be on a single Z level (got Min.Z=%d Max.Z=%d)", r.Min.Z, r.Max.Z)
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
local minX, maxX, minY, maxY = %d, %d, %d, %d
local count = 0
local function markIfInRegion(t)
    local p = t.pos
    if p.z == rawz and p.x >= minX and p.x <= maxX and p.y >= minY and p.y <= maxY then
        local block = dfhack.maps.getTileBlock(p)
        if block then
            block.designation[p.x%%16][p.y%%16].dig = df.tile_dig_designation.Default
            block.occupancy[p.x%%16][p.y%%16].dig_marked = false
            block.flags.designated = true
            count = count + 1
        end
    end
end
for _, t in ipairs(df.global.world.plants.tree_dry) do markIfInRegion(t) end
for _, t in ipairs(df.global.world.plants.tree_wet) do markIfInRegion(t) end
if count == 0 then error("no trees in region") end`, r.Min.Z, minX, maxX, minY, maxY)
	return ex.RunLua(script)
}
