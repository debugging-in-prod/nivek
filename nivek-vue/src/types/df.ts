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

export interface ZLevel {
    z: number
    tiles: number[]              // row-major TileType values; length = width*height
    furniture: FurniturePlace[]
}

export interface MapSnapshot {
    captured_at: string  // ISO timestamp
    origin: Position     // x, y valid for all levels; z = lowest level
    width: number        // tile count along X (same for every level)
    height: number       // tile count along Y (same for every level)
    levels: ZLevel[]     // sorted ascending by Z, contiguous
}
