package overseer

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// NivekOverseerService submits parsed actions to DFHack.
// Lives executor-side (machine running DF + DFHack), not Pi-side.
type NivekOverseerService interface {
	Submit(action Action) error
}

type nivekOverseerServiceImpl struct {
	dfhackRunPath string
}

func NewService(dfhackRunPath string) NivekOverseerService {
	return &nivekOverseerServiceImpl{dfhackRunPath: dfhackRunPath}
}

// itemToJobType maps chat-facing item nouns to DFHack `job_type` enum names
// used by the `workorder` script. Chat tokens are singular; DFHack's enum
// name is whatever DFHack uses internally (chair -> ConstructThrone,
// chest -> ConstructBox, block -> ConstructBlocks, etc.).
//
// Confirmed via `orders export`: table, bed, door, chair, coffin, blocks.
// Extrapolated (very likely correct): cabinet, chest, statue, floodgate.
// If any extrapolated mapping errors out in practice, fix the value here.
var itemToJobType = map[string]string{
	"table":      "ConstructTable",
	"bed":        "ConstructBed",
	"door":       "ConstructDoor",
	"chair":      "ConstructThrone",
	"throne":     "ConstructThrone",
	"coffin":     "ConstructCoffin",
	"block":      "ConstructBlocks",
	"cabinet":    "ConstructCabinet",
	"chest":      "ConstructChest",
	"statue":     "ConstructStatue",
	"floodgate":  "ConstructFloodgate",
	"bucket":     "MakeBucket",
	"barrel":     "MakeBarrel",
	"ash":        "MakeAsh",
	"charcoal":   "MakeCharcoal",
	"armorstand": "ConstructArmorStand",
	"grate":      "ConstructGrate",
	"hatchcover": "ConstructHatchCover",
	"millstone":  "ConstructMillstone",
	"quern":      "ConstructQuern",
	"slab":       "ConstructSlab",
	"weaponrack": "ConstructWeaponRack",
}

// itemToToolSubtype covers chat items that DF builds as *tools* via the
// MakeTool job_type with an item_subtype, rather than a dedicated
// Construct*/Make* job_type. These are stoneworker products that became
// tools in DF Premium. Verified via `df.global.world.raws.itemdefs.tools`.
var itemToToolSubtype = map[string]string{
	"altar":    "ITEM_TOOL_ALTAR",
	"bookcase": "ITEM_TOOL_BOOKCASE",
	"pedestal": "ITEM_TOOL_PEDESTAL",
}

// placeableItemToBuildingType maps chat-facing item nouns to DFHack
// df.building_type enum names used by dfhack.buildings.constructBuilding.
// Distinct from itemToJobType because DF's job_type enum (used for
// manufacture orders) and building_type enum (used for placement) have
// different names for the same things — e.g. chat "chair" is
// ConstructThrone in job_type but Chair in building_type; chat "chest"
// is ConstructChest in job_type but Box in building_type. Confirmed via
// `df.building_type[name]` lookups.
var placeableItemToBuildingType = map[string]string{
	"table":     "Table",
	"bed":       "Bed",
	"door":      "Door",
	"chair":     "Chair",
	"coffin":    "Coffin",
	"cabinet":   "Cabinet",
	"chest":     "Box",
	"statue":    "Statue",
	"floodgate": "Floodgate",
}

// materialToWorkorderSpec maps chat-facing material tokens to the JSON shape
// DFHack's `workorder` command expects. Two shapes coexist:
//
//   - `material: "INORGANIC[:SUBTYPE]"` for stone (any non-economic rock) and
//     specific metals (iron, copper, ...). Confirmed via `orders export`.
//   - `material_category: ["<name>"]` for DF's high-level material categories
//     (wood, bone, leather, ...). Confirmed for wood; extrapolated for the
//     rest by category-name analogy.
//
// If an extrapolated mapping ever produces "unknown material" in DF, fix the
// value here.
var materialToWorkorderSpec = map[string]workorderMaterialSpec{
	"stone":   {Material: "INORGANIC"},
	"wood":    {MaterialCategory: []string{"wood"}},
	"iron":    {Material: "INORGANIC:IRON"},
	"copper":  {Material: "INORGANIC:COPPER"},
	"bronze":  {Material: "INORGANIC:BRONZE"},
	"steel":   {Material: "INORGANIC:STEEL"},
	"silver":  {Material: "INORGANIC:SILVER"},
	"gold":    {Material: "INORGANIC:GOLD"},
	"bone":    {MaterialCategory: []string{"bone"}},
	"leather": {MaterialCategory: []string{"leather"}},
	"cloth":   {MaterialCategory: []string{"cloth"}},
	"shell":   {MaterialCategory: []string{"shell"}},
	"metal":   {MaterialCategory: []string{"metal"}},
}

type workorderMaterialSpec struct {
	Material         string
	MaterialCategory []string
}

// workorderRequest is the JSON payload `workorder <json>` accepts.
// Material xor MaterialCategory — populate whichever the spec uses.
// ItemSubtype is set only for MakeTool jobs (altar/bookcase/pedestal).
type workorderRequest struct {
	Job              string   `json:"job"`
	AmountTotal      int      `json:"amount_total"`
	Material         string   `json:"material,omitempty"`
	MaterialCategory []string `json:"material_category,omitempty"`
	ItemSubtype      string   `json:"item_subtype,omitempty"`
}

// brewSourceToReaction maps chat-facing brew sources to DFHack reaction
// IDs used by the workorder script's CustomReaction job_type. Confirmed
// via `orders export` — these are the two reaction strings DF Premium
// uses for the default brewing templates.
var brewSourceToReaction = map[string]string{
	"fruit": "BREW_DRINK_FROM_PLANT_GROWTH",
	"plant": "BREW_DRINK_FROM_PLANT",
}

// customReactionRequest is the workorder JSON payload for CustomReaction
// jobs (brewing, etc.) — these don't use job_type names like MakeTable
// but instead specify "job":"CustomReaction" + "reaction":<reaction_id>.
type customReactionRequest struct {
	Job         string `json:"job"`
	Reaction    string `json:"reaction"`
	AmountTotal int    `json:"amount_total"`
}

func (s *nivekOverseerServiceImpl) Submit(action Action) error {
	switch action.Kind {
	case ActionKindManufacture:
		return s.submitManufacture(action)
	case ActionKindPause:
		return s.runLua("df.global.pause_state=true")
	case ActionKindUnpause:
		return s.runLua("df.global.pause_state=false")
	case ActionKindCamera:
		return s.submitCamera(action)
	case ActionKindPlace:
		return s.submitPlace(action)
	case ActionKindBrew:
		return s.submitBrew(action)
	case ActionKindMine:
		return s.submitMine(action)
	default:
		return fmt.Errorf("unsupported action kind: %s", action.Kind)
	}
}

func (s *nivekOverseerServiceImpl) submitCamera(action Action) error {
	if action.Position == nil {
		return fmt.Errorf("camera requires position")
	}
	script := fmt.Sprintf("dfhack.gui.revealInDwarfmodeMap({x=%d,y=%d,z=%d}, true)",
		action.Position.X, action.Position.Y, action.Position.Z)
	return s.runLua(script)
}

func (s *nivekOverseerServiceImpl) submitMine(action Action) error {
	if action.Region == nil {
		return fmt.Errorf("mine requires region")
	}
	r := action.Region
	if r.Min.Z != r.Max.Z {
		return fmt.Errorf("mine region must be on a single Z level (got Min.Z=%d Max.Z=%d)", r.Min.Z, r.Max.Z)
	}
	minX, maxX := r.Min.X, r.Max.X
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	minY, maxY := r.Min.Y, r.Max.Y
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	// Iterate each tile in the rectangle, set the dig designation. Pattern
	// matches the spatial PoC's single-tile lua, just looped. Block lookup
	// per-tile rather than batched — at <=25 tiles, RPC overhead is fine.
	script := fmt.Sprintf(`
for x = %d, %d do
    for y = %d, %d do
        local block = dfhack.maps.getTileBlock({x=x, y=y, z=%d})
        if block then
            block.designation[x%%16][y%%16].dig = df.tile_dig_designation.Default
            block.occupancy[x%%16][y%%16].dig_marked = false
            block.flags.designated = true
        end
    end
end`, minX, maxX, minY, maxY, r.Min.Z)
	return s.runLua(script)
}

func (s *nivekOverseerServiceImpl) submitBrew(action Action) error {
	reaction, ok := brewSourceToReaction[action.Item]
	if !ok {
		return fmt.Errorf("no DFHack reaction mapping for brew source: %s", action.Item)
	}
	qty := action.Quantity
	if qty <= 0 {
		qty = 1
	}
	payload, err := json.Marshal(customReactionRequest{
		Job:         "CustomReaction",
		Reaction:    reaction,
		AmountTotal: qty,
	})
	if err != nil {
		return fmt.Errorf("marshal brew workorder json: %w", err)
	}
	out, err := exec.Command(s.dfhackRunPath, "workorder", string(payload)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dfhack-run failed: %w: %s", err, string(out))
	}
	return nil
}

func (s *nivekOverseerServiceImpl) submitPlace(action Action) error {
	if action.Position == nil {
		return fmt.Errorf("place requires position")
	}
	dfType, ok := placeableItemToBuildingType[action.Item]
	if !ok {
		return fmt.Errorf("no DFHack building_type mapping for item: %s", action.Item)
	}
	// constructBuilding returns the building handle on success, nil on
	// failure (bad tile, occupied, no item available, etc.). Surface the
	// nil case as an error so chat sees it.
	script := fmt.Sprintf(
		`local bld = dfhack.buildings.constructBuilding{type=df.building_type.%s, pos={x=%d,y=%d,z=%d}}; if not bld then error('constructBuilding returned nil — bad spot, blocked, or no matching item available') end`,
		dfType, action.Position.X, action.Position.Y, action.Position.Z,
	)
	return s.runLua(script)
}

func (s *nivekOverseerServiceImpl) submitManufacture(action Action) error {
	qty := action.Quantity
	if qty <= 0 {
		qty = 1
	}

	req := workorderRequest{AmountTotal: qty}

	// Tools (altar/bookcase/pedestal) use the MakeTool job_type with an
	// item_subtype; everything else has a dedicated Construct*/Make* job.
	if subtype, isTool := itemToToolSubtype[action.Item]; isTool {
		req.Job = "MakeTool"
		req.ItemSubtype = subtype
	} else {
		jobType, ok := itemToJobType[action.Item]
		if !ok {
			return fmt.Errorf("no DFHack job_type mapping for item: %s", action.Item)
		}
		req.Job = jobType
	}

	// Material is nil for fixed-recipe jobs (MakeAsh, MakeCharcoal — these
	// don't take a material slot, the recipe inputs are fixed). For
	// everything else the parser guarantees Material is set; populate the
	// workorder material/material_category fields from the translation
	// table so DFHack queues the right material variant.
	if action.Material != nil {
		spec, ok := materialToWorkorderSpec[*action.Material]
		if !ok {
			return fmt.Errorf("no DFHack material mapping for material: %s", *action.Material)
		}
		req.Material = spec.Material
		req.MaterialCategory = spec.MaterialCategory
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal workorder json: %w", err)
	}

	out, err := exec.Command(s.dfhackRunPath, "workorder", string(payload)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dfhack-run failed: %w: %s", err, string(out))
	}
	return nil
}

func (s *nivekOverseerServiceImpl) runLua(script string) error {
	out, err := exec.Command(s.dfhackRunPath, "lua", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("dfhack-run lua failed: %w: %s", err, string(out))
	}
	return nil
}
