package verbs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// itemToJobType maps chat-facing item nouns to DFHack `job_type` enum
// names used by the `workorder` script. Chat tokens are singular; DFHack's
// enum name is whatever DFHack uses internally (chair -> ConstructThrone,
// chest -> ConstructBox, block -> ConstructBlocks, etc.).
//
// Each entry's job_type was verified against `df.job_type` on a live fort
// (the names in this table all return a non-nil enum value when probed
// via dfhack-run lua). Items that use a generic `MakeTool` / `MakeWeapon`
// / `MakeShield` / `MakeTrapComponent` job with an item_subtype live in
// itemToSubtypeJob instead — that table covers anything DF dispatches by
// subtype rather than by a dedicated Construct*/Make* job.
var itemToJobType = map[string]string{
	"table":       "ConstructTable",
	"bed":         "ConstructBed",
	"door":        "ConstructDoor",
	"chair":       "ConstructThrone",
	"throne":      "ConstructThrone",
	"coffin":      "ConstructCoffin",
	"block":       "ConstructBlocks",
	"cabinet":     "ConstructCabinet",
	"chest":       "ConstructChest",
	"statue":      "ConstructStatue",
	"floodgate":   "ConstructFloodgate",
	"bucket":      "MakeBucket",
	"barrel":      "MakeBarrel",
	"ash":         "MakeAsh",
	"charcoal":    "MakeCharcoal",
	"armorstand":  "ConstructArmorStand",
	"grate":       "ConstructGrate",
	"hatchcover":  "ConstructHatchCover",
	"millstone":   "ConstructMillstone",
	"quern":       "ConstructQuern",
	"slab":        "ConstructSlab",
	"weaponrack":  "ConstructWeaponRack",
	"cage":        "MakeCage",
	"animaltrap":  "MakeAnimalTrap",
	"bin":         "ConstructBin",
	"crutch":      "ConstructCrutch",
	"splint":      "ConstructSplint",
	"pipesection": "MakePipeSection",
}

// subtypeJobSpec is the {job_type, item_subtype} pair for items DF
// manufactures via a generic job dispatched by subtype — tools
// (MakeTool), weapons (MakeWeapon), shields (MakeShield), trap
// components (MakeTrapComponent). The job picks the right recipe by
// reading the item_subtype off the workorder.
type subtypeJobSpec struct {
	Job         string // df.job_type enum name
	ItemSubtype string // raws.itemdefs.<bucket>.id (ITEM_TOOL_*, ITEM_WEAPON_*, ...)
}

// itemToSubtypeJob covers every chat item whose DF manufacture path is
// "generic job + subtype" rather than a dedicated Construct*/Make*
// job_type. Subtypes were enumerated live from
// df.global.world.raws.itemdefs.*; jobs were verified non-nil against
// df.job_type.
var itemToSubtypeJob = map[string]subtypeJobSpec{
	"altar":         {Job: "MakeTool", ItemSubtype: "ITEM_TOOL_ALTAR"},
	"bookcase":      {Job: "MakeTool", ItemSubtype: "ITEM_TOOL_BOOKCASE"},
	"pedestal":      {Job: "MakeTool", ItemSubtype: "ITEM_TOOL_PEDESTAL"},
	"minecart":      {Job: "MakeTool", ItemSubtype: "ITEM_TOOL_MINECART"},
	"stepladder":    {Job: "MakeTool", ItemSubtype: "ITEM_TOOL_STEPLADDER"},
	"wheelbarrow":   {Job: "MakeTool", ItemSubtype: "ITEM_TOOL_WHEELBARROW"},
	"shield":        {Job: "MakeShield", ItemSubtype: "ITEM_SHIELD_SHIELD"},
	"buckler":       {Job: "MakeShield", ItemSubtype: "ITEM_SHIELD_BUCKLER"},
	"trainingaxe":   {Job: "MakeWeapon", ItemSubtype: "ITEM_WEAPON_AXE_TRAINING"},
	"trainingspear": {Job: "MakeWeapon", ItemSubtype: "ITEM_WEAPON_SPEAR_TRAINING"},
	"trainingsword": {Job: "MakeWeapon", ItemSubtype: "ITEM_WEAPON_SWORD_SHORT_TRAINING"},
	"corkscrew":     {Job: "MakeTrapComponent", ItemSubtype: "ITEM_TRAPCOMP_ENORMOUSCORKSCREW"},
	"menacingspike": {Job: "MakeTrapComponent", ItemSubtype: "ITEM_TRAPCOMP_MENACINGSPIKE"},
	"spikedball":    {Job: "MakeTrapComponent", ItemSubtype: "ITEM_TRAPCOMP_SPIKEDBALL"},
}

// materialToWorkorderSpec maps chat-facing material tokens to the JSON
// shape DFHack's `workorder` command expects. Two shapes coexist:
//
//   - `material: "INORGANIC[:SUBTYPE]"` for stone (any non-economic rock)
//     and specific metals (iron, copper, ...). Confirmed via
//     `orders export`.
//   - `material_category: ["<name>"]` for DF's high-level material
//     categories (wood, bone, leather, ...). Confirmed for wood;
//     extrapolated for the rest by category-name analogy.
//
// If an extrapolated mapping ever produces "unknown material" in DF,
// fix the value here.
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

// ParseManufacture handles `make [qty] <material> <item>`.
func ParseManufacture(tokens []string) (wire.Action, error) {
	if len(tokens) == 0 {
		return wire.Action{}, fmt.Errorf("missing item")
	}

	qty := 1
	if n, parseErr := strconv.Atoi(tokens[0]); parseErr == nil {
		if n <= 0 {
			return wire.Action{}, fmt.Errorf("quantity must be positive")
		}
		qty = n
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return wire.Action{}, fmt.Errorf("missing item")
	}

	itemToken := strings.TrimSuffix(tokens[len(tokens)-1], "s")
	if _, ok := itemVocab[itemToken]; !ok {
		return wire.Action{}, fmt.Errorf("unknown item: %q", itemToken)
	}

	_, materialNotApplicable := itemMaterialNotApplicable[itemToken]

	var material *string
	pre := tokens[:len(tokens)-1]
	switch len(pre) {
	case 0:
		// DF Manager requires a material for most items — orders without
		// one queue as "unknown material" and can never execute. Items in
		// itemMaterialNotApplicable are the exception (fixed-recipe jobs).
		if !materialNotApplicable {
			return wire.Action{}, fmt.Errorf("missing material")
		}
	case 1:
		if materialNotApplicable {
			// Material isn't used for this item; silently ignore whatever
			// the chatter put there rather than rejecting valid commands.
			break
		}
		matToken := normalizeMaterial(pre[0])
		if _, ok := materialVocab[matToken]; !ok {
			return wire.Action{}, fmt.Errorf("unknown material: %q", pre[0])
		}
		material = &matToken
	default:
		return wire.Action{}, fmt.Errorf("extra tokens: %q", strings.Join(pre, " "))
	}

	if !materialNotApplicable {
		if allowed, restricted := itemMaterialAllowlist[itemToken]; restricted {
			if _, ok := allowed[*material]; !ok {
				return wire.Action{}, fmt.Errorf("material %q not allowed for item %q", *material, itemToken)
			}
		}
	}

	return wire.Action{
		Kind:     wire.ActionKindManufacture,
		Item:     itemToken,
		Material: material,
		Quantity: qty,
	}, nil
}

// SubmitManufacture sends a `workorder` JSON payload to DFHack. The
// order goes into the fortress-wide manager queue and waits for the
// manager to validate it.
func SubmitManufacture(ex Executor, action wire.Action) error {
	qty := action.Quantity
	if qty <= 0 {
		qty = 1
	}

	req := workorderRequest{AmountTotal: qty}

	// Items dispatched by subtype (tools, weapons, shields, trap
	// components) hit a generic job_type with item_subtype set;
	// everything else has a dedicated Construct*/Make* job_type and
	// no subtype.
	if spec, hasSubtype := itemToSubtypeJob[action.Item]; hasSubtype {
		req.Job = spec.Job
		req.ItemSubtype = spec.ItemSubtype
	} else {
		jobType, ok := itemToJobType[action.Item]
		if !ok {
			return fmt.Errorf("no DFHack job_type mapping for item: %s", action.Item)
		}
		req.Job = jobType
	}

	// Material is nil for fixed-recipe jobs (MakeAsh, MakeCharcoal — these
	// don't take a material slot, the recipe inputs are fixed). For
	// everything else the parser guarantees Material is set; populate
	// the workorder material/material_category fields from the
	// translation table so DFHack queues the right material variant.
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

	return ex.RunDFHack("workorder", string(payload))
}
