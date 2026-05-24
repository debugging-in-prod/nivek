-- dump-map-snapshot.lua
--
-- Dumps the current fortress's tile and furniture data as JSON, matching
-- the overseer.MapSnapshot shape expected by nivek.life/api/df/snapshot.
-- Output goes to stdout.
--
-- Usage:
--   dfhack-run dump-map-snapshot
--
-- Phase 2 PoC: dumps current Z ± Z_RADIUS levels (5 levels by default),
-- full X/Y extent of the map. Material on furniture is hardcoded to
-- "stone" — proper material lookup is a follow-up.
--
-- Wire contract this output must satisfy is defined in:
--   internal/libraries/overseer/const.go    (Go struct)
--   nivek-vue/src/types/df.ts               (TS interface)

local json = require('json')

-- ============ CONFIG ============
local Z_RADIUS = 25  -- dumps current_z ± this many levels (51 total)
-- ================================

-- TileType values mirror overseer.TileType in const.go. Keep in sync.
local T_UNKNOWN, T_WALL, T_FLOOR, T_RAMP, T_STAIR, T_WATER, T_MAGMA, T_TREE = 0, 1, 2, 3, 4, 5, 6, 7

-- DF building_type → chat-facing item vocab. Keep in sync with
-- itemVocab + itemToJobType in
-- internal/libraries/overseer/parse.go and service.go.
local BUILDING_TYPE_TO_CHAT = {
    [df.building_type.Table]     = 'table',
    [df.building_type.Bed]       = 'bed',
    [df.building_type.Chair]     = 'chair',
    [df.building_type.Door]      = 'door',
    [df.building_type.Coffin]    = 'coffin',
    [df.building_type.Cabinet]   = 'cabinet',
    [df.building_type.Box]       = 'chest',
    [df.building_type.Statue]    = 'statue',
    [df.building_type.Floodgate] = 'floodgate',
}

-- Force a lua table to encode as a JSON array rather than an object.
-- Without this, empty tables ({}) serialize as "{}" instead of "[]" with
-- most lua JSON libraries, which breaks the wire contract.
local function as_array(t)
    return setmetatable(t, { __jsontype = 'array' })
end

local function tile_at(block, dx, dy)
    local desig = block.designation[dx][dy]
    -- Liquid overrides shape for display
    if desig.flow_size > 0 then
        if desig.liquid_type == df.tile_liquid.Magma then return T_MAGMA end
        return T_WATER
    end
    local tt = block.tiletype[dx][dy]
    local shape = df.tiletype.attrs[tt].shape
    if shape == df.tiletype_shape.WALL then return T_WALL end
    if shape == df.tiletype_shape.FLOOR then return T_FLOOR end
    if shape == df.tiletype_shape.RAMP then return T_RAMP end
    if shape == df.tiletype_shape.STAIR_UP
        or shape == df.tiletype_shape.STAIR_DOWN
        or shape == df.tiletype_shape.STAIR_UPDOWN then return T_STAIR end
    return T_UNKNOWN
end

local map = df.global.world.map
local center_z = df.global.window_z
local z_min = math.max(0, center_z - Z_RADIUS)
local z_max = math.min(map.z_count - 1, center_z + Z_RADIUS)
local width = map.x_count
local height = map.y_count

-- Build per-Z level data
local levels = {}
for z = z_min, z_max do
    local tiles = {}
    for i = 1, width * height do tiles[i] = T_UNKNOWN end

    -- Iterate by 16x16 blocks. dfhack.maps.getTileBlock is the safe
    -- per-position accessor; iterating df.global.world.map.map_blocks
    -- directly crashed DF on a real save (gotcha from spatial PoC).
    for by = 0, height - 1, 16 do
        for bx = 0, width - 1, 16 do
            local block = dfhack.maps.getTileBlock({ x = bx, y = by, z = z })
            if block then
                for dy = 0, 15 do
                    for dx = 0, 15 do
                        local x, y = bx + dx, by + dy
                        if x < width and y < height then
                            tiles[y * width + x + 1] = tile_at(block, dx, dy)
                        end
                    end
                end
            end
        end
    end

    table.insert(levels, {
        z = z,
        tiles = as_array(tiles),
        furniture = as_array({}),  -- populated below
    })
end

-- Furniture pass: one walk through all buildings, dispatch to the right Z
for _, b in ipairs(df.global.world.buildings.all) do
    local type_name = BUILDING_TYPE_TO_CHAT[b:getType()]
    if type_name and b.z >= z_min and b.z <= z_max then
        for _, lvl in ipairs(levels) do
            if lvl.z == b.z then
                table.insert(lvl.furniture, {
                    type = type_name,
                    material = 'stone', -- PoC: real material lookup TBD
                    x = b.x1,
                    y = b.y1,
                })
                break
            end
        end
    end
end

-- Citizens pass: list active fortress citizens with their name, profession,
-- age, current job, stress category, and position. Bounded by total fort
-- pop (usually 50-200) so payload growth is modest.
local citizens = {}
for _, u in ipairs(df.global.world.units.active) do
    if dfhack.units.isCitizen(u) then
        -- getReadableName returns "Name 'Nickname', Profession" — strip the
        -- trailing profession since we send it as a separate field, and
        -- df2utf-normalize so special characters render as valid UTF-8.
        local name = dfhack.df2utf(dfhack.units.getReadableName(u)) or '(unnamed)'
        name = (name:gsub(', [^,]+$', ''))

        -- getProfessionName returns the human-readable form (e.g. "Miner")
        -- vs the raw enum string ("MINER"). Falls back to enum if unset.
        local prof_name = dfhack.units.getProfessionName(u)
        if not prof_name or prof_name == '' then
            prof_name = df.profession[u.profession] or 'Unknown'
        end
        local age = math.floor(dfhack.units.getAge(u, true) or 0)

        local job_name = ''
        if u.job and u.job.current_job then
            local ok, jn = pcall(dfhack.job.getName, u.job.current_job)
            if ok and jn then job_name = dfhack.df2utf(jn) end
        end

        local stress = dfhack.units.getStressCategory(u) or 3  -- 3 = "Fine"/neutral

        table.insert(citizens, {
            id = u.id,
            name = name,
            profession = prof_name,
            age = age,
            job = job_name,
            stress = stress,
            position = { x = u.pos.x, y = u.pos.y, z = u.pos.z },
        })
    end
end

-- The first map hotkey (F1, in-game bound to e.g. "Wagon arrival location")
-- gives the dashboard a sensible starting view. hotkeys is a fixed-size
-- array; an unassigned slot reads as (0,0,0), which we treat as "no focus".
local focus = nil
local hk = df.global.plotinfo.main.hotkeys
if hk and #hk > 0 then
    local f1 = hk[0]
    if not (f1.x == 0 and f1.y == 0 and f1.z == 0) then
        focus = { x = f1.x, y = f1.y, z = f1.z }
    end
end

local snapshot = {
    captured_at = os.date('!%Y-%m-%dT%H:%M:%SZ'),
    origin = { x = 0, y = 0, z = z_min },
    width = width,
    height = height,
    levels = as_array(levels),
    citizens = as_array(citizens),
    focus = focus,
}

print(json.encode(snapshot))
