package verbs

import (
	"fmt"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

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

// placeableItemToBuilding maps chat-facing item nouns to the building
// they construct via `!DF place`. The building_type names differ from
// the job_type names used for manufacture (e.g. chat "chair" is
// ConstructThrone as a job_type but Chair as a building_type; "chest"
// is Box). Every entry here was verified live with constructBuilding on
// a running fort.
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
	// SoapMaker and ScrewPress were included originally but aren't present
	// in this DF version's df.workshop_type enum (it ends at Millstone);
	// `!DF place soapmaker ...` would fail at constructBuilding with a nil
	// subtype. Re-add when targeting a DF build that ships them.

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

// ParsePlace handles `place <item> <coords>` — a single-tile placement.
// No material is taken; the placed building enters DFHack's buildingplan
// queue and claims any matching material when one becomes available.
func ParsePlace(rest []string) (wire.Action, error) {
	if len(rest) == 0 {
		return wire.Action{}, fmt.Errorf("place needs <item> <x> <y> <z>")
	}
	item := strings.TrimSuffix(rest[0], "s")
	if _, ok := placeableItemToBuilding[item]; !ok {
		return wire.Action{}, fmt.Errorf("not placeable: %q", item)
	}
	coords, err := extractCoordInts(rest[1:])
	if err != nil {
		return wire.Action{}, err
	}
	if len(coords) != 3 {
		return wire.Action{}, fmt.Errorf("place needs 3 coordinates after item, got %d", len(coords))
	}
	return wire.Action{
		Kind:     wire.ActionKindPlace,
		Item:     item,
		Position: &wire.Position{X: coords[0], Y: coords[1], Z: coords[2]},
	}, nil
}

// SubmitPlace routes through DFHack's buildingplan plugin: defaults
// filters via getFiltersByType, constructs the building, and registers
// it with buildingplan. The building enters "planned" state; when
// matching materials become available (now or later), buildingplan
// auto-claims them and the build starts. Eliminates the previous
// failure mode where placing a table with no wood in stock returned nil.
func SubmitPlace(ex Executor, action wire.Action) error {
	if action.Position == nil {
		return fmt.Errorf("place requires position")
	}
	spec, ok := placeableItemToBuilding[action.Item]
	if !ok {
		return fmt.Errorf("no DFHack building mapping for item: %s", action.Item)
	}
	subtypeLua := "local subtype = -1"
	switch {
	case spec.WorkshopSubtype != "":
		subtypeLua = fmt.Sprintf("local subtype = df.workshop_type.%s", spec.WorkshopSubtype)
	case spec.FurnaceSubtype != "":
		subtypeLua = fmt.Sprintf("local subtype = df.furnace_type.%s", spec.FurnaceSubtype)
	}
	// Position.Z is elevation; convert to raw z (see SubmitCamera).
	script := fmt.Sprintf(`
local rawz = %d - (df.global.world.map.region_z - 100)
local btype = df.building_type.%s
%s
local bp = require('plugins.buildingplan')
local filters = dfhack.buildings.getFiltersByType({}, btype, subtype, -1)
local bld, err = dfhack.buildings.constructBuilding{
    type = btype, subtype = subtype,
    pos = {x = %d, y = %d, z = rawz},
    filters = filters,
}
if err then error(tostring(err)) end
if not bld then error('constructBuilding returned nil — bad spot or blocked') end
bp.addPlannedBuilding(bld)
bp.scheduleCycle()`, action.Position.Z, spec.BuildingType, subtypeLua, action.Position.X, action.Position.Y)
	return ex.RunLua(script)
}
