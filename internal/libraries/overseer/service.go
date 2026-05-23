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
	"table":     "ConstructTable",
	"bed":       "ConstructBed",
	"door":      "ConstructDoor",
	"chair":     "ConstructThrone",
	"coffin":    "ConstructCoffin",
	"block":     "ConstructBlocks",
	"cabinet":   "ConstructCabinet",
	"chest":     "ConstructChest",
	"statue":    "ConstructStatue",
	"floodgate": "ConstructFloodgate",
	"bucket":    "MakeBucket",
	"barrel":    "MakeBarrel",
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
type workorderRequest struct {
	Job              string   `json:"job"`
	AmountTotal      int      `json:"amount_total"`
	Material         string   `json:"material,omitempty"`
	MaterialCategory []string `json:"material_category,omitempty"`
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

func (s *nivekOverseerServiceImpl) submitManufacture(action Action) error {
	if action.Material == nil {
		return fmt.Errorf("manufacture requires material")
	}
	jobType, ok := itemToJobType[action.Item]
	if !ok {
		return fmt.Errorf("no DFHack job_type mapping for item: %s", action.Item)
	}
	spec, ok := materialToWorkorderSpec[*action.Material]
	if !ok {
		return fmt.Errorf("no DFHack material mapping for material: %s", *action.Material)
	}

	qty := action.Quantity
	if qty <= 0 {
		qty = 1
	}

	payload, err := json.Marshal(workorderRequest{
		Job:              jobType,
		AmountTotal:      qty,
		Material:         spec.Material,
		MaterialCategory: spec.MaterialCategory,
	})
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
