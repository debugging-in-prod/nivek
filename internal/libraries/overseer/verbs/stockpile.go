package verbs

import (
	"fmt"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// stockpileCategoryToPreset maps chat-facing stockpile category nouns to
// the DFHack stockpiles plugin's library preset path. Chat tokens are
// singular — the parser's trailing-s strip means plurals (animals,
// weapons, gems, etc.) also work. The presets ship with DFHack at
// hack/data/stockpiles/cat_*.dfstock; each is "accept this category,
// reject everything else", exactly what a single-category stockpile
// needs. Mapped via plugins.stockpiles.import_settings on a freshly
// constructed Stockpile building.
var stockpileCategoryToPreset = map[string]string{
	"all":       "library/all", // accepts every default top-level category
	"ammo":      "library/cat_ammo",
	"animal":    "library/cat_animals",
	"armor":     "library/cat_armor",
	"bar":       "library/cat_bars_blocks",
	"cloth":     "library/cat_cloth",
	"coin":      "library/cat_coins",
	"corpse":    "library/cat_corpses",
	"good":      "library/cat_finished_goods",
	"food":      "library/cat_food",
	"furniture": "library/cat_furniture",
	"gem":       "library/cat_gems",
	"leather":   "library/cat_leather",
	"refuse":    "library/cat_refuse",
	"sheet":     "library/cat_sheets",
	"stone":     "library/cat_stone",
	"weapon":    "library/cat_weapons",
	"wood":      "library/cat_wood",
}

// ParseStockpile handles `stockpile <category> <coords>` — same coord
// shape as the dig verbs, plus a category keyword (singular; plural-strip
// applies).
func ParseStockpile(rest []string) (wire.Action, error) {
	if len(rest) == 0 {
		return wire.Action{}, fmt.Errorf("stockpile needs <category> <coords>")
	}
	category := strings.TrimSuffix(rest[0], "s")
	if _, ok := stockpileCategoryToPreset[category]; !ok {
		return wire.Action{}, fmt.Errorf("unknown stockpile category: %q", rest[0])
	}
	region, err := parseRegionVerb("stockpile", rest[1:])
	if err != nil {
		return wire.Action{}, err
	}
	return wire.Action{
		Kind:   wire.ActionKindStockpile,
		Item:   category,
		Region: region,
	}, nil
}

// SubmitStockpile builds an abstract Stockpile building covering the
// region and applies one of DFHack's built-in cat_*.dfstock presets to
// restrict it to a single top-level category.
func SubmitStockpile(ex Executor, action wire.Action) error {
	if action.Region == nil {
		return fmt.Errorf("stockpile requires region")
	}
	preset, ok := stockpileCategoryToPreset[action.Item]
	if !ok {
		return fmt.Errorf("no DFHack preset for stockpile category: %s", action.Item)
	}
	r := action.Region
	if r.Min.Z != r.Max.Z {
		return fmt.Errorf("stockpile region must be on a single Z level (got Min.Z=%d Max.Z=%d)", r.Min.Z, r.Max.Z)
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
local sp = dfhack.buildings.constructBuilding{
    type = df.building_type.Stockpile,
    abstract = true,
    pos = {x=%d, y=%d, z=rawz},
    width = %d, height = %d,
}
if not sp then error("constructBuilding returned nil — bad spot or blocked") end
require("plugins.stockpiles").import_settings(%q, {id=sp.id})`,
		r.Min.Z, minX, minY, maxX-minX+1, maxY-minY+1, preset)
	return ex.RunLua(script)
}
