import { TileType, type MapSnapshot, type ZLevel } from '@/types/df'

// Zoom bounds (pixels per tile). Default sits in the middle so there's
// room to zoom both directions.
export const DEFAULT_CELL_SIZE = 10
export const MIN_CELL_SIZE = 2
export const MAX_CELL_SIZE = 40

// Tile fill colors. Distinct enough to read at a glance, muted enough that
// furniture overlays sit on top legibly.
const TILE_COLORS: Record<number, string> = {
    [TileType.Unknown]: '#111',
    [TileType.Wall]:    '#555',
    [TileType.Floor]:   '#888',
    [TileType.Ramp]:    '#a07040',
    [TileType.Stair]:   '#665',
    [TileType.Water]:   '#3a8fc7',
    [TileType.Magma]:   '#d04020',
    [TileType.Tree]:    '#2d8a3f',
}

// One-character glyph for each known furniture type. Anything unrecognized
// falls back to '?' so missing mappings are visible rather than invisible.
const FURNITURE_GLYPHS: Record<string, string> = {
    table:     'T',
    chair:     'h',
    bed:       'B',
    door:      'D',
    coffin:    'C',
    cabinet:   'F',
    chest:     'X',
    block:     'b',
    statue:    'S',
    floodgate: '#',
}

// drawLevel renders one Z level of the snapshot at the given cell size
// (zoom). The canvas's internal resolution is sized to the full map at
// the current zoom — containment/scrolling is handled by the CSS wrapper.
export function drawLevel(canvas: HTMLCanvasElement, snap: MapSnapshot, level: ZLevel, cellSize: number): void {
    canvas.width = snap.width * cellSize
    canvas.height = snap.height * cellSize
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    // Tiles first (background layer).
    for (let y = 0; y < snap.height; y++) {
        for (let x = 0; x < snap.width; x++) {
            const t = level.tiles[y * snap.width + x] ?? TileType.Unknown
            ctx.fillStyle = TILE_COLORS[t] ?? TILE_COLORS[TileType.Unknown]
            ctx.fillRect(x * cellSize, y * cellSize, cellSize, cellSize)
        }
    }

    // Grid lines only when cells are big enough that they aid the eye
    // rather than smearing the whole canvas into lines.
    if (cellSize >= 6) {
        ctx.strokeStyle = 'rgba(0,0,0,0.15)'
        ctx.lineWidth = 1
        for (let x = 0; x <= snap.width; x++) {
            ctx.beginPath()
            ctx.moveTo(x * cellSize + 0.5, 0)
            ctx.lineTo(x * cellSize + 0.5, snap.height * cellSize)
            ctx.stroke()
        }
        for (let y = 0; y <= snap.height; y++) {
            ctx.beginPath()
            ctx.moveTo(0, y * cellSize + 0.5)
            ctx.lineTo(snap.width * cellSize, y * cellSize + 0.5)
            ctx.stroke()
        }
    }

    // Furniture overlays (foreground layer). Glyphs need room to read, so
    // skip them when zoomed too far out.
    if (cellSize >= 8) {
        ctx.fillStyle = '#fff'
        ctx.font = `${cellSize - 2}px monospace`
        ctx.textAlign = 'center'
        ctx.textBaseline = 'middle'
        for (const f of level.furniture) {
            const localX = f.x - snap.origin.x
            const localY = f.y - snap.origin.y
            if (localX < 0 || localX >= snap.width) continue
            if (localY < 0 || localY >= snap.height) continue
            const glyph = FURNITURE_GLYPHS[f.type] ?? '?'
            ctx.fillText(glyph, localX * cellSize + cellSize / 2, localY * cellSize + cellSize / 2)
        }
    }
}

// pixelToWorld converts a canvas pixel offset (mouse coords relative to
// the canvas top-left) into a world coord at the given cell size. Returns
// null if the pixel is outside the rendered grid.
export function pixelToWorld(
    snap: MapSnapshot,
    level: ZLevel,
    pixelX: number,
    pixelY: number,
    cellSize: number,
): { x: number; y: number; z: number } | null {
    const localX = Math.floor(pixelX / cellSize)
    const localY = Math.floor(pixelY / cellSize)
    if (localX < 0 || localX >= snap.width) return null
    if (localY < 0 || localY >= snap.height) return null
    return {
        x: snap.origin.x + localX,
        y: snap.origin.y + localY,
        z: level.z,
    }
}
