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

export interface MapSnapshot {
    captured_at: string  // ISO timestamp
    origin: Position     // world coord of the (0,0) cell in tiles[]
    width: number        // tile count along X
    height: number       // tile count along Y
    z: number            // Z level this snapshot covers
    tiles: number[]      // row-major TileType values; length = width*height
    furniture: FurniturePlace[]
}
