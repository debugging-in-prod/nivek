// MapSnapshot shape matches `overseer.MapSnapshot` in the Go backend
// (internal/libraries/overseer/const.go). Keep field names and types in
// sync — this is the wire contract the dashboard renders from.

export interface Position {
    x: number
    y: number
    z: number
}

export enum TileType {
    Unknown = 0,
    Wall    = 1,
    Floor   = 2,
    Ramp    = 3,
    Stair   = 4,
    Water   = 5,
    Magma   = 6,
    Tree    = 7,
}

export interface FurniturePlace {
    type:     string  // "table", "bed", "door", ...
    material: string  // "stone", "wood", ...
    x: number
    y: number
}

// Footprint is a multi-tile rectangular building (workshop, furnace,
// stockpile) drawn under the furniture glyph layer as a tinted region
// with a centered label. Distinct from FurniturePlace (single-tile).
export interface Footprint {
    id:      number  // DFHack building.id — stable handle chat uses to target this workshop
    kind:    string  // "workshop" | "furnace" | "stockpile"
    subtype: string  // chat-facing workshop/furnace name, or "" for stockpiles
    x1: number
    y1: number
    x2: number
    y2: number
}

export interface ZLevel {
    z: number
    tiles: number[]              // row-major TileType values; length = width*height
    furniture: FurniturePlace[]
    footprints?: Footprint[]     // multi-tile buildings; omitted from old snapshots
}

// Citizen mirrors overseer.Citizen on the Go side. Stress is the dfhack
// "stress category" integer: 0 = most stressed (miserable), 6 = least
// stressed (ecstatic).
export interface Citizen {
    id: number           // DFHack unit.id — stable handle chat uses to target this dwarf
    name: string
    profession: string
    age: number
    job?: string         // empty / omitted when idle
    stress: number       // 0 (most stressed) .. 6 (least stressed)
    position: Position
}

export interface MapSnapshot {
    captured_at: string  // ISO timestamp
    origin: Position     // x, y valid for all levels; z = lowest level
    width: number        // tile count along X (same for every level)
    height: number       // tile count along Y (same for every level)
    levels: ZLevel[]     // sorted ascending by Z, contiguous
    z_offset: number     // add to raw z to get in-game elevation: elev = z + z_offset
    citizens?: Citizen[] // active fortress citizens (optional in case server omits)
    focus?: Position     // F1 map-hotkey location; the canvas centers here on first load
}
