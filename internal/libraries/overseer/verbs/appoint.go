package verbs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/overseer/wire"
)

// nobleVocab is the set of chat-facing noble-position keywords `!DF
// appoint` accepts. Each maps to a single-slot fort position; the
// keyword→DFHack position code translation lives in officeToPositionCode.
var nobleVocab = map[string]struct{}{
	"manager":    {},
	"bookkeeper": {},
	"broker":     {},
	"doctor":     {}, // chief medical dwarf
	"commander":  {}, // militia commander
}

// nobleDeferred maps recognized-but-not-yet-supported position keywords
// to a user-facing reason. Militia captain is squad-dependent (a captain
// leads a squad, which must exist first) — handling it properly needs
// squad management we haven't built, so it gets a clear "not supported
// yet" rather than an "unknown position" error.
var nobleDeferred = map[string]string{
	"captain": "captain needs a squad — not supported yet",
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

// appointLuaTemplate appoints a unit to a fort noble position. It
// mirrors DFHack's own make-monarch.lua, adapted from the civ entity
// (monarch) to the fortress entity (manager/bookkeeper/etc.): set the
// assignment's histfig/histfig2 to the target's historical figure, drop
// the previous holder's position entity-link, and add one for the new
// holder. Verified against the live save's structures
// (entity_id = fortress_entity.id, histfig2 mirrors histfig, link
// carries assignment_id/vector_idx/start_year).
//
// %d = unit.id, %q = position code. Both come from trusted sources
// (an int and a value from officeToPositionCode), so there's no
// lua-injection risk.
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

// ParseAppoint handles `appoint <position> <id>` — token order is
// flexible (the numeric token is the id, the other is the position).
// A leading `#` on the id is tolerated.
func ParseAppoint(tokens []string) (wire.Action, error) {
	if len(tokens) != 2 {
		return wire.Action{}, fmt.Errorf("appoint needs <position> <id>, e.g. 'appoint manager 8423'")
	}
	t0 := strings.TrimPrefix(tokens[0], "#")
	t1 := strings.TrimPrefix(tokens[1], "#")

	var posTok string
	var id int
	n0, e0 := strconv.Atoi(t0)
	n1, e1 := strconv.Atoi(t1)
	switch {
	case e0 != nil && e1 == nil:
		posTok, id = t0, n1
	case e0 == nil && e1 != nil:
		posTok, id = t1, n0
	default:
		return wire.Action{}, fmt.Errorf("appoint needs exactly one position keyword and one numeric id")
	}
	if id < 0 {
		return wire.Action{}, fmt.Errorf("invalid unit id: %d", id)
	}

	if reason, deferred := nobleDeferred[posTok]; deferred {
		// Chat-visible: a recognized position we just don't support yet.
		return wire.Action{}, &wire.RejectReason{Msg: reason}
	}
	if _, ok := nobleVocab[posTok]; !ok {
		return wire.Action{}, fmt.Errorf("unknown position: %q (try manager, bookkeeper, broker, doctor, commander)", posTok)
	}

	return wire.Action{
		Kind:   wire.ActionKindAppoint,
		Office: posTok,
		UnitID: id,
	}, nil
}

// SubmitAppoint runs the appointLuaTemplate with the target unit and
// the resolved DF position code.
func SubmitAppoint(ex Executor, action wire.Action) error {
	code, ok := officeToPositionCode[action.Office]
	if !ok {
		return fmt.Errorf("no DFHack position code for office: %s", action.Office)
	}
	return ex.RunLua(fmt.Sprintf(appointLuaTemplate, action.UnitID, code))
}
