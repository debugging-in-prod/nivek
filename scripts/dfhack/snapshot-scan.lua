-- snapshot-scan.lua
--
-- Persistent, incremental replacement for dump-map-snapshot.lua. Instead of
-- walking the whole map synchronously (which froze DF for ~5s every cycle),
-- this scans a time-boxed slice of map blocks per frame, accumulates a full
-- snapshot, and atomically publishes it to a JSON file that df-snapshot-pusher
-- reads and forwards to nivek.life. No single frame does enough work to hitch.
--
-- Usage:
--   snapshot-scan start    -- begin the background scan loop (idempotent)
--   snapshot-scan stop      -- cancel it
--   snapshot-scan status    -- report whether it's running + output path
--
-- Autostart: add `snapshot-scan start` to
--   <DF>/dfhack-config/init/onMapLoad.init
-- so it begins on every fort load. repeat-util cancels itself on world
-- unload, so leaving a fort stops the scan automatically.
--
-- Output file: <DF>/nivek-snapshot.json  (point DASHBOARD_SNAPSHOT_FILE at it).
-- Written via a temp file + atomic rename, so the pusher never reads a
-- partial pass.
--
-- Wire contract this output must satisfy is defined in:
--   internal/libraries/overseer/const.go    (Go struct)
--   nivek-vue/src/types/df.ts               (TS interface)

local repeatUtil = require('repeat-util')
local json = require('json')

-- ============ CONFIG ============
local BUDGET_MS = 2          -- max real-time ms of scanning work per frame
local PASS_GAP_MS = 30000    -- idle gap after a completed pass before the next
local SCHED_NAME = 'nivek-snapshot-scan'
local OUT_PATH = dfhack.getDFPath() .. '/nivek-snapshot.json'
local TMP_PATH = OUT_PATH .. '.tmp'
-- ================================

-- TileType values mirror overseer.TileType in const.go. Keep in sync.
local T_UNKNOWN, T_WALL, T_FLOOR, T_RAMP, T_STAIR, T_WATER, T_MAGMA, T_TREE = 0, 1, 2, 3, 4, 5, 6, 7

-- DF building_type → chat-facing item vocab. Keep in sync with
-- placeableItemToBuilding in internal/libraries/overseer/service.go.
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

-- Force a lua table to encode as a JSON array rather than an object, so an
-- empty list serializes as "[]" not "{}".
local function as_array(t)
    return setmetatable(t, { __jsontype = 'array' })
end

local function tile_at(block, dx, dy)
    local desig = block.designation[dx][dy]
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

-- Furniture is cheap (a few hundred buildings at most) so it's gathered once
-- at the start of a pass and indexed by Z for per-level emission.
local function gather_furniture_by_z(z_min, z_max)
    local by_z = {}
    for _, b in ipairs(df.global.world.buildings.all) do
        local name = BUILDING_TYPE_TO_CHAT[b:getType()]
        if name and b.z >= z_min and b.z <= z_max then
            local list = by_z[b.z]
            if not list then list = {}; by_z[b.z] = list end
            list[#list + 1] = { type = name, material = 'stone', x = b.x1, y = b.y1 }
        end
    end
    return by_z
end

-- Citizens are bounded by fort population (usually 50-200), so the whole list
-- is gathered in one slice at finalize time.
local function gather_citizens()
    local citizens = {}
    for _, u in ipairs(df.global.world.units.active) do
        if dfhack.units.isCitizen(u) then
            local name = dfhack.df2utf(dfhack.units.getReadableName(u)) or '(unnamed)'
            name = (name:gsub(', [^,]+$', ''))
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
            local stress = dfhack.units.getStressCategory(u) or 3
            citizens[#citizens + 1] = {
                id = u.id,
                name = name,
                profession = prof_name,
                age = age,
                job = job_name,
                stress = stress,
                position = { x = u.pos.x, y = u.pos.y, z = u.pos.z },
            }
        end
    end
    return citizens
end

-- The first map hotkey (F1) gives the dashboard a sensible starting view.
-- An unassigned slot reads as (0,0,0), which we treat as "no focus".
local function gather_focus()
    local hk = df.global.plotinfo.main.hotkeys
    if hk and #hk > 0 then
        local f1 = hk[0]
        if not (f1.x == 0 and f1.y == 0 and f1.z == 0) then
            return { x = f1.x, y = f1.y, z = f1.z }
        end
    end
    return nil
end

-- ===== incremental pass state (persists across scheduled ticks via the
-- closure repeat-util holds) =====
local pass = nil       -- in-progress pass table, or nil when idle
local last_end_ms = 0  -- getTickCount() when the last pass finished

-- begin_pass opens the temp file, writes the header, and sets up block
-- iteration. Returns the pass table, or nil if no map is loaded / open fails.
local function begin_pass()
    if not dfhack.isMapLoaded() then return nil end

    local map = df.global.world.map
    -- Scan the full map height so deep/tall forts are captured completely,
    -- independent of where the camera sits. Forts can span ~150 of ~190
    -- z-levels, so a camera-centered band would clip the surface or the depths.
    local z_min = 0
    local z_max = map.z_count - 1
    local width = map.x_count
    local height = map.y_count
    -- DF's in-game "elevation" readout = absolute world z relative to sea level (100).
    -- Snapshot z values are embark-local (0..z_count-1); the dashboard adds this
    -- offset to display elevation so chatters see the same number the player does.
    local z_offset = map.region_z - 100

    local f, err = io.open(TMP_PATH, 'w')
    if not f then
        dfhack.printerr('snapshot-scan: cannot open ' .. TMP_PATH .. ': ' .. tostring(err))
        return nil
    end

    f:write(string.format('{"captured_at":%s,"origin":%s,"width":%d,"height":%d,"z_offset":%d,"levels":[',
        json.encode(os.date('!%Y-%m-%dT%H:%M:%SZ')),
        json.encode({ x = 0, y = 0, z = z_min }),
        width, height, z_offset))

    -- Block origins for one Z level (same grid for every level).
    local blocks = {}
    for by = 0, height - 1, 16 do
        for bx = 0, width - 1, 16 do
            blocks[#blocks + 1] = { bx, by }
        end
    end

    -- Reused tile buffer for the current level; reset to Unknown per level.
    local tiles = {}
    for i = 1, width * height do tiles[i] = T_UNKNOWN end

    return {
        f = f,
        z_min = z_min, z_max = z_max, width = width, height = height,
        blocks = blocks,
        furniture_by_z = gather_furniture_by_z(z_min, z_max),
        cur_z = z_min,
        cur_block = 1,
        tiles = tiles,
        wrote_level = false,
    }
end

local function scan_block(p)
    local b = p.blocks[p.cur_block]
    local bx, by = b[1], b[2]
    local block = dfhack.maps.getTileBlock({ x = bx, y = by, z = p.cur_z })
    if block then
        local width, height, tiles = p.width, p.height, p.tiles
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

-- finish_level emits the current level's JSON and advances to the next Z,
-- resetting the tile buffer. The big tiles array is joined with table.concat
-- (fast, once per level) rather than a giant json.encode at end-of-pass.
local function finish_level(p)
    local f = p.f
    if p.wrote_level then f:write(',') end
    f:write(string.format('{"z":%d,"tiles":[', p.cur_z))
    f:write(table.concat(p.tiles, ','))
    f:write('],"furniture":')
    f:write(json.encode(as_array(p.furniture_by_z[p.cur_z] or {})))
    f:write('}')
    p.wrote_level = true

    p.cur_z = p.cur_z + 1
    p.cur_block = 1
    if p.cur_z <= p.z_max then
        local tiles = p.tiles
        for i = 1, p.width * p.height do tiles[i] = T_UNKNOWN end
    end
end

local function finalize(p)
    local f = p.f
    f:write('],"citizens":')
    f:write(json.encode(as_array(gather_citizens())))
    local focus = gather_focus()
    if focus then
        f:write(',"focus":')
        f:write(json.encode(focus))
    end
    f:write('}')
    f:close()
    -- rename(2) atomically replaces OUT_PATH on the same filesystem, so the
    -- pusher always reads a complete pass.
    local ok, err = os.rename(TMP_PATH, OUT_PATH)
    if not ok then
        dfhack.printerr('snapshot-scan: rename failed: ' .. tostring(err))
    end
end

-- tick runs every frame: a time-boxed slice of scanning while a pass is
-- active, otherwise idling until the inter-pass gap elapses.
local function tick()
    if pass and not dfhack.isMapLoaded() then
        -- Fort unloaded mid-pass; drop the partial cleanly.
        pcall(function() pass.f:close() end)
        pass = nil
        return
    end

    local now = dfhack.getTickCount()
    if not pass then
        if last_end_ms == 0 or (now - last_end_ms) >= PASS_GAP_MS then
            pass = begin_pass()
            if not pass then last_end_ms = now end  -- back off before retry
        end
        return
    end

    local deadline = now + BUDGET_MS
    while dfhack.getTickCount() < deadline do
        if pass.cur_block <= #pass.blocks then
            scan_block(pass)
            pass.cur_block = pass.cur_block + 1
        else
            finish_level(pass)
            if pass.cur_z > pass.z_max then
                finalize(pass)
                last_end_ms = dfhack.getTickCount()
                pass = nil
                return
            end
        end
    end
end

local cmd = ({...})[1] or 'start'
if cmd == 'start' then
    pass = nil
    last_end_ms = 0  -- 0 => first pass starts immediately
    repeatUtil.scheduleEvery(SCHED_NAME, 1, 'frames', tick)
    print('snapshot-scan: started, writing ' .. OUT_PATH)
elseif cmd == 'stop' then
    repeatUtil.cancel(SCHED_NAME)
    print('snapshot-scan: stopped')
elseif cmd == 'status' then
    print('snapshot-scan: ' .. (repeatUtil.isScheduled(SCHED_NAME) and 'running' or 'stopped'))
    print('  output: ' .. OUT_PATH)
else
    print('usage: snapshot-scan [start|stop|status]')
end
