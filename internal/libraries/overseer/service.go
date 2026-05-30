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
// Each entry's job_type was verified against `df.job_type` on a live fort
// (the names in this table all return a non-nil enum value when probed via
// dfhack-run lua). Items that use a generic `MakeTool` / `MakeWeapon` /
// `MakeShield` / `MakeTrapComponent` job with an item_subtype live in
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
// manufactures via a generic job dispatched by subtype — tools (MakeTool),
// weapons (MakeWeapon), shields (MakeShield), trap components
// (MakeTrapComponent). The job picks the right recipe by reading the
// item_subtype off the workorder.
type subtypeJobSpec struct {
	Job         string // df.job_type enum name
	ItemSubtype string // raws.itemdefs.<bucket>.id (ITEM_TOOL_*, ITEM_WEAPON_*, ...)
}

// itemToSubtypeJob covers every chat item whose DF manufacture path is
// "generic job + subtype" rather than a dedicated Construct*/Make* job_type.
// Subtypes were enumerated live from df.global.world.raws.itemdefs.*; jobs
// were verified non-nil against df.job_type.
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

// placeSpec describes how a chat item maps to a DFHack building for
// dfhack.buildings.constructBuilding. BuildingType is a df.building_type
// enum name. WorkshopSubtype is a df.workshop_type enum name, set only
// when BuildingType is "Workshop". FurnaceSubtype is a df.furnace_type
// enum name, set only when BuildingType is "Furnace". At most one
// subtype field is populated per entry.
type placeSpec struct {
	BuildingType    string
	WorkshopSubtype string
	FurnaceSubtype  string
}

// placeableItemToBuilding maps chat-facing item nouns to the building they
// construct via `!DF place`. The building_type names differ from the
// job_type names used for manufacture (e.g. chat "chair" is ConstructThrone
// as a job_type but Chair as a building_type; "chest" is Box). Every entry
// here was verified live with constructBuilding on a running fort.
//
// Excluded from placement (manufacturable, but not buildings): block (a
// construction material), bucket/barrel/bin (containers), ash/charcoal
// (bars), crutch/splint (medical items), minecart/stepladder/wheelbarrow/
// pipesection (tools / siege parts), shield/buckler (combat gear),
// trainingaxe/spear/sword (weapons), corkscrew/menacingspike/spikedball
// (trap components — built into a Trap building, not standalone).
var placeableItemToBuilding = map[string]placeSpec{
	"table":      {BuildingType: "Table"},
	"bed":        {BuildingType: "Bed"},
	"door":       {BuildingType: "Door"},
	"chair":      {BuildingType: "Chair"},
	"throne":     {BuildingType: "Chair"}, // synonym for chair
	"coffin":     {BuildingType: "Coffin"},
	"cabinet":    {BuildingType: "Cabinet"},
	"chest":      {BuildingType: "Box"},
	"statue":     {BuildingType: "Statue"},
	"floodgate":  {BuildingType: "Floodgate"},
	"armorstand": {BuildingType: "Armorstand"},
	"weaponrack": {BuildingType: "Weaponrack"},
	"hatchcover": {BuildingType: "Hatch"},
	"grate":      {BuildingType: "GrateFloor"}, // floor grate; the common case
	"slab":       {BuildingType: "Slab"},
	"bookcase":   {BuildingType: "Bookcase"},
	"pedestal":   {BuildingType: "DisplayFurniture"},
	"altar":      {BuildingType: "OfferingPlace"},
	"cage":       {BuildingType: "Cage"},
	"animaltrap": {BuildingType: "AnimalTrap"},
	"quern":      {BuildingType: "Workshop", WorkshopSubtype: "Quern"},
	"millstone":  {BuildingType: "Workshop", WorkshopSubtype: "Millstone"},

	// Workshops (df.workshop_type). Names follow DF's UI; chat tokens are
	// singular (the parser's trailing-s strip means `carpenters` also works).
	// Skipped: `Tool` (catch-all, not used in vanilla) and `Custom` (mod
	// hook, no fixed enum). Magma variants below — they only succeed once
	// the fort has dug to magma; until then constructBuilding returns nil
	// and chat gets a "no matching item available" error.
	"carpenter":     {BuildingType: "Workshop", WorkshopSubtype: "Carpenters"},
	"farmer":        {BuildingType: "Workshop", WorkshopSubtype: "Farmers"},
	"mason":         {BuildingType: "Workshop", WorkshopSubtype: "Masons"},
	"craftsdwarf":   {BuildingType: "Workshop", WorkshopSubtype: "Craftsdwarfs"},
	"jeweler":       {BuildingType: "Workshop", WorkshopSubtype: "Jewelers"},
	"forge":         {BuildingType: "Workshop", WorkshopSubtype: "MetalsmithsForge"},
	"magmaforge":    {BuildingType: "Workshop", WorkshopSubtype: "MagmaForge"},
	"bowyer":        {BuildingType: "Workshop", WorkshopSubtype: "Bowyers"},
	"mechanic":      {BuildingType: "Workshop", WorkshopSubtype: "Mechanics"},
	"siegeworkshop": {BuildingType: "Workshop", WorkshopSubtype: "Siege"},
	"butcher":       {BuildingType: "Workshop", WorkshopSubtype: "Butchers"},
	"leatherwork":   {BuildingType: "Workshop", WorkshopSubtype: "Leatherworks"},
	"tanner":        {BuildingType: "Workshop", WorkshopSubtype: "Tanners"},
	"clothier":      {BuildingType: "Workshop", WorkshopSubtype: "Clothiers"},
	"fishery":       {BuildingType: "Workshop", WorkshopSubtype: "Fishery"},
	"still":         {BuildingType: "Workshop", WorkshopSubtype: "Still"},
	"loom":          {BuildingType: "Workshop", WorkshopSubtype: "Loom"},
	"kennel":        {BuildingType: "Workshop", WorkshopSubtype: "Kennels"},
	"ashery":        {BuildingType: "Workshop", WorkshopSubtype: "Ashery"},
	"kitchen":       {BuildingType: "Workshop", WorkshopSubtype: "Kitchen"},
	"dyer":          {BuildingType: "Workshop", WorkshopSubtype: "Dyers"},
	"soapmaker":     {BuildingType: "Workshop", WorkshopSubtype: "SoapMaker"},
	"screwpress":    {BuildingType: "Workshop", WorkshopSubtype: "ScrewPress"},

	// Furnaces (df.furnace_type). Magma variants need magma access, same
	// caveat as magmaforge above.
	"woodfurnace":       {BuildingType: "Furnace", FurnaceSubtype: "WoodFurnace"},
	"smelter":           {BuildingType: "Furnace", FurnaceSubtype: "Smelter"},
	"glassfurnace":      {BuildingType: "Furnace", FurnaceSubtype: "GlassFurnace"},
	"kiln":              {BuildingType: "Furnace", FurnaceSubtype: "Kiln"},
	"magmasmelter":      {BuildingType: "Furnace", FurnaceSubtype: "MagmaSmelter"},
	"magmaglassfurnace": {BuildingType: "Furnace", FurnaceSubtype: "MagmaGlassFurnace"},
	"magmakiln":         {BuildingType: "Furnace", FurnaceSubtype: "MagmaKiln"},
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

// officeToPositionCode maps chat-facing noble keywords to DFHack
// entity_position `code` strings. Confirmed live via the fort entity's
// positions.own table: MANAGER, BOOKKEEPER, BROKER, CHIEF_MEDICAL_DWARF,
// MILITIA_COMMANDER. Militia captain (MILITIA_CAPTAIN) is intentionally
// absent — it's squad-dependent and rejected at the parser.
var officeToPositionCode = map[string]string{
	"manager":    "MANAGER",
	"bookkeeper": "BOOKKEEPER",
	"broker":     "BROKER",
	"doctor":     "CHIEF_MEDICAL_DWARF",
	"commander":  "MILITIA_COMMANDER",
}

// appointLuaTemplate appoints a unit to a fort noble position. It mirrors
// DFHack's own make-monarch.lua, adapted from the civ entity (monarch) to
// the fortress entity (manager/bookkeeper/etc.): set the assignment's
// histfig/histfig2 to the target's historical figure, drop the previous
// holder's position entity-link, and add one for the new holder. Verified
// against the live save's structures (entity_id = fortress_entity.id,
// histfig2 mirrors histfig, link carries assignment_id/vector_idx/start_year).
//
// %d = unit.id, %q = position code. Both come from trusted sources (an int
// and a value from officeToPositionCode), so there's no lua-injection risk.
const appointLuaTemplate = `
local UNIT_ID = %d
local CODE = %q
local unit = df.unit.find(UNIT_ID)
if not unit then error("no unit with id "..UNIT_ID) end
if not dfhack.units.isCitizen(unit) then error("unit "..UNIT_ID.." is not a citizen of this fort") end
local figid = unit.hist_figure_id
if figid < 0 then error("unit "..UNIT_ID.." has no historical figure") end
local newfig = df.historical_figure.find(figid)
if not newfig then error("historical figure "..figid.." not found") end

local ent = df.global.plotinfo.main.fortress_entity
if not ent then error("no fortress entity") end

local posid
for _,p in ipairs(ent.positions.own) do
    if p.code == CODE then posid = p.id break end
end
if not posid then error("position "..CODE.." not defined for this fort") end

local done = false
for aidx,a in ipairs(ent.positions.assignments) do
    if a.position_id == posid then
        if a.histfig == newfig.id then done = true break end
        local oldid = a.histfig
        a.histfig = newfig.id
        a.histfig2 = newfig.id
        if oldid >= 0 then
            local oldfig = df.historical_figure.find(oldid)
            if oldfig then
                for k,v in pairs(oldfig.entity_links) do
                    if df.histfig_entity_link_positionst:is_instance(v)
                       and v.assignment_id == a.id and v.entity_id == ent.id then
                        oldfig.entity_links:erase(k)
                        break
                    end
                end
            end
        end
        local has = false
        for _,v in pairs(newfig.entity_links) do
            if df.histfig_entity_link_positionst:is_instance(v)
               and v.assignment_id == a.id and v.entity_id == ent.id then
                has = true break
            end
        end
        if not has then
            newfig.entity_links:insert("#", {new=df.histfig_entity_link_positionst,
                entity_id=ent.id, link_strength=100, assignment_id=a.id,
                assignment_vector_idx=aidx, start_year=df.global.cur_year})
        end
        done = true
        break
    end
end
if not done then error("no assignment slot for position "..CODE) end
print("appointed unit "..UNIT_ID.." as "..CODE)
`

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
		return s.submitDigDesignation(action, "Default")
	case ActionKindChannel:
		return s.submitDigDesignation(action, "Channel")
	case ActionKindDigRamp:
		return s.submitDigDesignation(action, "Ramp")
	case ActionKindAppoint:
		return s.submitAppoint(action)
	default:
		return fmt.Errorf("unsupported action kind: %s", action.Kind)
	}
}

func (s *nivekOverseerServiceImpl) submitCamera(action Action) error {
	if action.Position == nil {
		return fmt.Errorf("camera requires position")
	}
	// Position.Z is elevation (dashboard-native); DFHack APIs want raw
	// embark-local z. Convert in the lua so we don't need a separate
	// region_z fetch. See [Position] in const.go.
	script := fmt.Sprintf(`local rawz = %d - (df.global.world.map.region_z - 100); dfhack.gui.revealInDwarfmodeMap({x=%d,y=%d,z=rawz}, true)`,
		action.Position.Z, action.Position.X, action.Position.Y)
	return s.runLua(script)
}

// submitDigDesignation issues a rectangular dig designation across the
// region in action.Region. designation is the df.tile_dig_designation enum
// name to apply — "Default" (mine), "Channel" (channel), or "Ramp"
// (digramp). The shape — single-Z, raw-z conversion, per-tile block lookup
// — is identical across all three verbs; only the designation differs.
func (s *nivekOverseerServiceImpl) submitDigDesignation(action Action, designation string) error {
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
	// Iterate each tile in the rectangle, set the dig designation. Block
	// lookup per-tile rather than batched — at <=25 tiles, RPC overhead is
	// fine. r.Min.Z is elevation (dashboard-native); convert to raw z
	// inside the lua, same as submitCamera/submitPlace.
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
	spec, ok := placeableItemToBuilding[action.Item]
	if !ok {
		return fmt.Errorf("no DFHack building mapping for item: %s", action.Item)
	}
	// constructBuilding returns the building handle on success, nil on
	// failure (bad tile, occupied, no item available, etc.). Surface the
	// nil case as an error so chat sees it. Workshops and furnaces
	// additionally need a subtype from df.workshop_type / df.furnace_type;
	// the placeSpec carries at most one of the two.
	subtype := ""
	switch {
	case spec.WorkshopSubtype != "":
		subtype = fmt.Sprintf(", subtype=df.workshop_type.%s", spec.WorkshopSubtype)
	case spec.FurnaceSubtype != "":
		subtype = fmt.Sprintf(", subtype=df.furnace_type.%s", spec.FurnaceSubtype)
	}
	// Position.Z is elevation; convert to raw z in the lua (see submitCamera).
	script := fmt.Sprintf(
		`local rawz = %d - (df.global.world.map.region_z - 100); local bld = dfhack.buildings.constructBuilding{type=df.building_type.%s%s, pos={x=%d,y=%d,z=rawz}}; if not bld then error('constructBuilding returned nil — bad spot, blocked, or no matching item available') end`,
		action.Position.Z, spec.BuildingType, subtype, action.Position.X, action.Position.Y,
	)
	return s.runLua(script)
}

func (s *nivekOverseerServiceImpl) submitAppoint(action Action) error {
	code, ok := officeToPositionCode[action.Office]
	if !ok {
		return fmt.Errorf("no DFHack position code for office: %s", action.Office)
	}
	return s.runLua(fmt.Sprintf(appointLuaTemplate, action.UnitID, code))
}

func (s *nivekOverseerServiceImpl) submitManufacture(action Action) error {
	qty := action.Quantity
	if qty <= 0 {
		qty = 1
	}

	req := workorderRequest{AmountTotal: qty}

	// Items dispatched by subtype (tools, weapons, shields, trap components)
	// hit a generic job_type with item_subtype set; everything else has a
	// dedicated Construct*/Make* job_type and no subtype.
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
