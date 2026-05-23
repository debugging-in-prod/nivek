import { TileType, type MapSnapshot } from '@/types/df'

export const CELL_SIZE = 12   // pixels per tile; 50-tile fixture = 600px canvas

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

export function drawSnapshot(canvas: HTMLCanvasElement, snap: MapSnapshot): void {
    canvas.width = snap.width * CELL_SIZE
    canvas.height = snap.height * CELL_SIZE
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    // Tiles first (background layer).
    for (let y = 0; y < snap.height; y++) {
        for (let x = 0; x < snap.width; x++) {
            const t = snap.tiles[y * snap.width + x] ?? TileType.Unknown
            ctx.fillStyle = TILE_COLORS[t] ?? TILE_COLORS[TileType.Unknown]
            ctx.fillRect(x * CELL_SIZE, y * CELL_SIZE, CELL_SIZE, CELL_SIZE)
        }
    }

    // Subtle grid lines for click-target visibility.
    ctx.strokeStyle = 'rgba(0,0,0,0.15)'
    ctx.lineWidth = 1
    for (let x = 0; x <= snap.width; x++) {
        ctx.beginPath()
        ctx.moveTo(x * CELL_SIZE + 0.5, 0)
        ctx.lineTo(x * CELL_SIZE + 0.5, snap.height * CELL_SIZE)
        ctx.stroke()
    }
    for (let y = 0; y <= snap.height; y++) {
        ctx.beginPath()
        ctx.moveTo(0, y * CELL_SIZE + 0.5)
        ctx.lineTo(snap.width * CELL_SIZE, y * CELL_SIZE + 0.5)
        ctx.stroke()
    }

    // Furniture overlays (foreground layer).
    ctx.fillStyle = '#fff'
    ctx.font = `${CELL_SIZE - 2}px monospace`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    for (const f of snap.furniture) {
        const localX = f.x - snap.origin.x
        const localY = f.y - snap.origin.y
        if (localX < 0 || localX >= snap.width) continue
        if (localY < 0 || localY >= snap.height) continue
        const glyph = FURNITURE_GLYPHS[f.type] ?? '?'
        ctx.fillText(glyph, localX * CELL_SIZE + CELL_SIZE / 2, localY * CELL_SIZE + CELL_SIZE / 2)
    }
}

// Convert a canvas pixel offset (e.g. mouse coords relative to the canvas)
// into a world coord, given the snapshot's origin. Returns null if the
// pixel is outside the rendered grid.
export function pixelToWorld(
    snap: MapSnapshot,
    pixelX: number,
    pixelY: number,
): { x: number; y: number; z: number } | null {
    const localX = Math.floor(pixelX / CELL_SIZE)
    const localY = Math.floor(pixelY / CELL_SIZE)
    if (localX < 0 || localX >= snap.width) return null
    if (localY < 0 || localY >= snap.height) return null
    return {
        x: snap.origin.x + localX,
        y: snap.origin.y + localY,
        z: snap.z,
    }
}
