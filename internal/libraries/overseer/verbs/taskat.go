package verbs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// ParseTaskat handles `taskat #<workshop_id> [qty] <material> <item>`.
// The leading `#` on the workshop id is REQUIRED so the bare-number
// disambiguates from `make`'s qty slot — `taskat 2 wood table` would
// otherwise read like "2 wood tables at... somewhere?".
func ParseTaskat(tokens []string) (wire.Action, error) {
	if len(tokens) < 3 {
		return wire.Action{}, fmt.Errorf("taskat needs #<workshop_id> [qty] <material> <item>")
	}
	if !strings.HasPrefix(tokens[0], "#") {
		return wire.Action{}, fmt.Errorf("workshop id must be prefixed with # (e.g. taskat #2 wood table) — read the id off the dashboard label")
	}
	wsTok := strings.TrimPrefix(tokens[0], "#")
	wsID, err := strconv.Atoi(wsTok)
	if err != nil || wsID < 0 {
		return wire.Action{}, fmt.Errorf("invalid workshop id: %q", tokens[0])
	}
	tokens = tokens[1:]

	qty := 1
	if n, parseErr := strconv.Atoi(tokens[0]); parseErr == nil {
		if n <= 0 {
			return wire.Action{}, fmt.Errorf("quantity must be positive")
		}
		qty = n
		tokens = tokens[1:]
	}
	if len(tokens) < 2 {
		return wire.Action{}, fmt.Errorf("taskat needs material and item after workshop id")
	}

	itemToken := strings.TrimSuffix(tokens[len(tokens)-1], "s")
	if _, ok := itemVocab[itemToken]; !ok {
		return wire.Action{}, fmt.Errorf("unknown item: %q", itemToken)
	}
	if _, na := itemMaterialNotApplicable[itemToken]; na {
		return wire.Action{}, fmt.Errorf("item %q has a fixed recipe — use !DF make, not taskat", itemToken)
	}

	pre := tokens[:len(tokens)-1]
	if len(pre) != 1 {
		return wire.Action{}, fmt.Errorf("taskat needs exactly one material token, got %q", strings.Join(pre, " "))
	}
	matToken := normalizeMaterial(pre[0])
	if _, ok := materialVocab[matToken]; !ok {
		return wire.Action{}, fmt.Errorf("unknown material: %q", pre[0])
	}

	return wire.Action{
		Kind:       wire.ActionKindTaskat,
		Item:       itemToken,
		Material:   &matToken,
		Quantity:   qty,
		WorkshopID: wsID,
	}, nil
}

// taskatMaterialLua returns the lua snippet that configures the given
// chat material on a freshly-created df.job. v1 supports the material
// categories DF exposes as bool flags on job.material_category (wood,
// bone, leather, cloth, shell, metal) plus generic "stone" via the
// INORGANIC mat_type. Specific metals (iron/copper/etc) need raws
// lookups and aren't implemented yet — return "" so SubmitTaskat
// surfaces a chat-visible error rather than queuing an unworkable job.
func taskatMaterialLua(material, jobVar string) string {
	switch material {
	case "wood", "bone", "leather", "cloth", "shell", "metal":
		return jobVar + ".material_category." + material + " = true"
	case "stone":
		// mat_type 0 = INORGANIC, mat_index -1 = any non-economic stone.
		return jobVar + ".mat_type = 0; " + jobVar + ".mat_index = -1"
	}
	return ""
}

// SubmitTaskat queues N copies of a single-item job directly into the
// target workshop's job vector, then dfhack.job.linkIntoWorld each one.
// Bypasses the manager queue entirely — the dwarves see the tasks
// immediately. Designed for pre-manager bootstrap; once a manager
// exists, !DF make is the higher-volume path.
func SubmitTaskat(ex Executor, action wire.Action) error {
	if action.WorkshopID == 0 {
		return fmt.Errorf("taskat requires workshop_id")
	}
	if action.Material == nil {
		return fmt.Errorf("taskat requires material")
	}

	var jobType, itemSubtype string
	if spec, hasSubtype := itemToSubtypeJob[action.Item]; hasSubtype {
		jobType = spec.Job
		itemSubtype = spec.ItemSubtype
	} else if jt, ok := itemToJobType[action.Item]; ok {
		jobType = jt
	} else {
		return fmt.Errorf("no DFHack job_type mapping for item: %s", action.Item)
	}
	if itemSubtype != "" {
		return fmt.Errorf("item %q uses an item_subtype path not yet supported in taskat", action.Item)
	}

	materialLua := taskatMaterialLua(*action.Material, "j")
	if materialLua == "" {
		return fmt.Errorf("material %q not supported in taskat v1 (try wood, stone, bone, leather, cloth, shell, metal)", *action.Material)
	}

	qty := action.Quantity
	if qty < 1 {
		qty = 1
	}

	script := fmt.Sprintf(`
local bld = df.building.find(%d)
if not bld then error("no building with id %d") end
if bld:getType() ~= df.building_type.Workshop then error("building %d is not a workshop") end
for i = 1, %d do
    local j = df.job:new()
    j.job_type = df.job_type.%s
    j.pos.x = bld.centerx
    j.pos.y = bld.centery
    j.pos.z = bld.z
    j.mat_type = -1
    j.mat_index = -1
    %s
    local bref = df.general_ref_building_holderst:new()
    bref.building_id = bld.id
    j.general_refs:insert("#", bref)
    bld.jobs:insert("#", j)
    dfhack.job.linkIntoWorld(j, true)
end`, action.WorkshopID, action.WorkshopID, action.WorkshopID, qty, jobType, materialLua)
	return ex.RunLua(script)
}
