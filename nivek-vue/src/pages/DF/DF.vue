<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { fetchSnapshot } from '@/services/DFService'
import type { MapSnapshot, ZLevel } from '@/types/df'
import { drawLevel, pixelToWorld, DEFAULT_CELL_SIZE, MIN_CELL_SIZE, MAX_CELL_SIZE } from './renderer'

// Poll the snapshot endpoint on a timer. Matched to the DFHost pusher's
// 30s push cadence so the page sees each push rather than every other one.
const POLL_INTERVAL_MS = 30_000

const canvasRef = ref<HTMLCanvasElement | null>(null)
const viewportRef = ref<HTMLDivElement | null>(null)
const snapshot = ref<MapSnapshot | null>(null)
const error = ref<string | null>(null)

// Zoom: pixels per tile. Mouse wheel adjusts it; the canvas re-renders at
// the new scale and the scrollable viewport wrapper keeps it contained.
const cellSize = ref<number>(DEFAULT_CELL_SIZE)

// Index into snapshot.levels[] of the currently-displayed Z level. Reset
// to "the highest level" each snapshot refresh so the user lands on the
// surface-ish view by default.
const currentLevelIdx = ref<number>(0)

const hoverCoord = ref<{ x: number; y: number; z: number } | null>(null)
const selectedCoord = ref<{ x: number; y: number; z: number } | null>(null)

let pollTimer: number | undefined

const currentLevel = computed<ZLevel | null>(() => {
    if (!snapshot.value) return null
    return snapshot.value.levels[currentLevelIdx.value] ?? null
})

const canGoUp = computed(() => {
    if (!snapshot.value) return false
    return currentLevelIdx.value < snapshot.value.levels.length - 1
})

const canGoDown = computed(() => {
    return currentLevelIdx.value > 0
})

async function loadSnapshot() {
    try {
        const fresh = await fetchSnapshot()
        error.value = null
        if (fresh === null) {
            // No snapshot available yet (server cold-start, before first push).
            // Leave snapshot.value as-is so previously-rendered state persists
            // if a poll briefly returns 404; the template will show the
            // waiting placeholder only when snapshot is still null.
            snapshot.value = null
            return
        }
        snapshot.value = fresh
        // Default to the MIDDLE of the snapshot's Z range on first load.
        // The lua dump emits `window_z ± Z_RADIUS` so the middle index
        // corresponds to DF's viewport at dump time — that's where the
        // fortress almost always is. The top of the range is usually
        // empty above-ground sky (all Unknown tiles, renders as a
        // featureless dark rectangle that looks broken to viewers).
        // Subsequent polls preserve the user's current Z if still in range.
        if (currentLevelIdx.value >= fresh.levels.length || currentLevelIdx.value < 0) {
            currentLevelIdx.value = Math.floor(fresh.levels.length / 2)
        }
        renderCurrent()
    } catch (err: any) {
        error.value = err?.message ?? 'unknown error'
    }
}

function renderCurrent() {
    if (!canvasRef.value || !snapshot.value || !currentLevel.value) return
    drawLevel(canvasRef.value, snapshot.value, currentLevel.value, cellSize.value)
}

function goUp() {
    if (canGoUp.value) currentLevelIdx.value += 1
}

function goDown() {
    if (canGoDown.value) currentLevelIdx.value -= 1
}

function onMouseMove(ev: MouseEvent) {
    if (!snapshot.value || !canvasRef.value || !currentLevel.value) return
    const rect = canvasRef.value.getBoundingClientRect()
    hoverCoord.value = pixelToWorld(snapshot.value, currentLevel.value, ev.clientX - rect.left, ev.clientY - rect.top, cellSize.value)
}

function onMouseLeave() {
    hoverCoord.value = null
}

function onClick(ev: MouseEvent) {
    if (!snapshot.value || !canvasRef.value || !currentLevel.value) return
    const rect = canvasRef.value.getBoundingClientRect()
    selectedCoord.value = pixelToWorld(snapshot.value, currentLevel.value, ev.clientX - rect.left, ev.clientY - rect.top, cellSize.value)
}

// Mouse wheel zooms the map, anchored on the tile under the cursor so it
// stays put as the scale changes (the scrollable viewport is adjusted to
// compensate). preventDefault stops the page/container from scrolling.
function onWheel(ev: WheelEvent) {
    if (!canvasRef.value || !viewportRef.value) return
    ev.preventDefault()

    const old = cellSize.value
    const factor = ev.deltaY < 0 ? 1.15 : 1 / 1.15
    const next = Math.max(MIN_CELL_SIZE, Math.min(MAX_CELL_SIZE, Math.round(old * factor)))
    if (next === old) return

    // World pixel under the cursor before the zoom (canvas-local + scroll).
    const vp = viewportRef.value
    const rect = canvasRef.value.getBoundingClientRect()
    const cursorCanvasX = ev.clientX - rect.left
    const cursorCanvasY = ev.clientY - rect.top
    const tileX = cursorCanvasX / old
    const tileY = cursorCanvasY / old

    cellSize.value = next
    renderCurrent()

    // After re-render at the new scale, scroll so the same tile sits under
    // the cursor. cursorCanvasX/Y were measured from the canvas's visible
    // top-left, which already accounts for the prior scroll offset.
    vp.scrollLeft += tileX * next - cursorCanvasX
    vp.scrollTop += tileY * next - cursorCanvasY
}

function zoomBy(factor: number) {
    const old = cellSize.value
    const next = Math.max(MIN_CELL_SIZE, Math.min(MAX_CELL_SIZE, Math.round(old * factor)))
    if (next === old) return
    cellSize.value = next
    renderCurrent()
}

function onKeydown(ev: KeyboardEvent) {
    // Only steal arrow keys when the user isn't typing in an input/textarea.
    const t = ev.target as HTMLElement | null
    if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return
    if (ev.key === 'ArrowUp' || ev.key === 'PageUp') {
        ev.preventDefault()
        goUp()
    } else if (ev.key === 'ArrowDown' || ev.key === 'PageDown') {
        ev.preventDefault()
        goDown()
    }
}

// Re-render whenever the active Z level or zoom changes (button click,
// key press, wheel, or snapshot refresh shifting the index).
watch([currentLevelIdx, cellSize], renderCurrent)

onMounted(() => {
    loadSnapshot()
    pollTimer = window.setInterval(loadSnapshot, POLL_INTERVAL_MS)
    window.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
    if (pollTimer !== undefined) window.clearInterval(pollTimer)
    window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
    <div class="df-page">
        <header>
            <div class="header-row">
                <h1>DF Twitch-plays — live overview</h1>
                <router-link to="/df/citizens" class="nav-btn">Citizens</router-link>
            </div>
            <p v-if="snapshot" class="capture-info">
                Captured {{ new Date(snapshot.captured_at).toLocaleString() }}
                · {{ snapshot.width }}×{{ snapshot.height }} tiles
                · {{ snapshot.levels.length }} Z {{ snapshot.levels.length === 1 ? 'level' : 'levels' }}
                · origin ({{ snapshot.origin.x }}, {{ snapshot.origin.y }})
            </p>
        </header>

        <div v-if="error" class="error">Snapshot unavailable: {{ error }}</div>

        <div v-if="!snapshot && !error" class="waiting">
            <p class="waiting-text">Waiting for data…</p>
            <p class="waiting-hint">
                Server hasn't received a snapshot from the DFHost yet.
                The pusher sends one every 30s once DF is running.
            </p>
        </div>

        <div v-else class="layout">
            <div class="canvas-viewport" ref="viewportRef" @wheel="onWheel">
                <canvas
                    ref="canvasRef"
                    @mousemove="onMouseMove"
                    @mouseleave="onMouseLeave"
                    @click="onClick"
                />
            </div>
            <aside class="hud">
                <h3>Zoom</h3>
                <div class="zoom-nav">
                    <button class="z-btn" @click="zoomBy(1/1.15)" aria-label="Zoom out">−</button>
                    <span class="z-current">{{ cellSize }} px/tile</span>
                    <button class="z-btn" @click="zoomBy(1.15)" aria-label="Zoom in">+</button>
                </div>
                <p class="z-hint">scroll wheel over the map also zooms</p>

                <h3>Z Level</h3>
                <div class="z-nav">
                    <button
                        class="z-btn"
                        :disabled="!canGoUp"
                        @click="goUp"
                        aria-label="Go up one Z level"
                    >▲ up</button>
                    <span class="z-current">
                        Z = <strong>{{ currentLevel?.z ?? '—' }}</strong>
                    </span>
                    <button
                        class="z-btn"
                        :disabled="!canGoDown"
                        @click="goDown"
                        aria-label="Go down one Z level"
                    >▼ down</button>
                </div>
                <p class="z-hint">↑/↓ or PageUp/PageDown also work</p>

                <h3>Coordinates</h3>
                <div class="coord-row">
                    <span class="label">Hover:</span>
                    <span v-if="hoverCoord" class="coord">
                        ({{ hoverCoord.x }}, {{ hoverCoord.y }}, {{ hoverCoord.z }})
                    </span>
                    <span v-else class="coord muted">—</span>
                </div>
                <div class="coord-row">
                    <span class="label">Selected:</span>
                    <span v-if="selectedCoord" class="coord">
                        ({{ selectedCoord.x }}, {{ selectedCoord.y }}, {{ selectedCoord.z }})
                    </span>
                    <span v-else class="coord muted">click a tile</span>
                </div>

                <h3>Legend</h3>
                <ul class="legend">
                    <li><span class="swatch wall" /> wall</li>
                    <li><span class="swatch floor" /> floor</li>
                    <li><span class="swatch ramp" /> ramp</li>
                    <li><span class="swatch stair" /> stair</li>
                    <li><span class="swatch water" /> water</li>
                    <li><span class="swatch magma" /> magma</li>
                    <li><span class="swatch tree" /> tree</li>
                </ul>
                <h3>Furniture glyphs</h3>
                <ul class="furniture-legend">
                    <li><code>T</code> table</li>
                    <li><code>h</code> chair</li>
                    <li><code>B</code> bed</li>
                    <li><code>D</code> door</li>
                    <li><code>C</code> coffin</li>
                    <li><code>F</code> cabinet</li>
                    <li><code>X</code> chest</li>
                    <li><code>S</code> statue</li>
                    <li><code>#</code> floodgate</li>
                </ul>
            </aside>
        </div>

        <p class="note">
            Live overview. Browser refreshes every {{ POLL_INTERVAL_MS / 1000 }}s; the DFHost pushes
            new data every 30s. Citizen roster is on the <router-link to="/df/citizens">Citizens</router-link> page.
        </p>
    </div>
</template>

<style scoped>
.df-page {
    padding: 1rem 2rem;
    color: #ddd;
    background: #1a1a1a;
    min-height: 100vh;
    font-family: system-ui, sans-serif;
}

.header-row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 1rem;
    flex-wrap: wrap;
}

header h1 {
    margin: 0 0 0.25rem 0;
    color: #6fb;
}

/* Themed button-link: dark surface, green accent matching .hud headings. */
.nav-btn {
    display: inline-block;
    background: #222;
    color: #6fb;
    border: 1px solid #3a6;
    border-radius: 4px;
    padding: 0.35rem 0.9rem;
    font-family: monospace;
    font-size: 0.9rem;
    text-decoration: none;
    white-space: nowrap;
}
.nav-btn:hover {
    background: #2a3a30;
    border-color: #6fb;
}

.capture-info {
    color: #888;
    font-size: 0.85rem;
    margin: 0 0 1rem 0;
}

.error {
    background: #422;
    border: 1px solid #844;
    padding: 0.5rem 1rem;
    border-radius: 4px;
    margin-bottom: 1rem;
}

.waiting {
    background: #000;
    border: 1px solid #333;
    border-radius: 4px;
    padding: 4rem 2rem;
    text-align: center;
    min-height: 400px;
    display: flex;
    flex-direction: column;
    justify-content: center;
    align-items: center;
}

.waiting-text {
    color: #6fb;
    font-size: 1.5rem;
    font-family: monospace;
    margin: 0;
}

.waiting-hint {
    color: #666;
    font-size: 0.9rem;
    margin-top: 0.5rem;
}

.layout {
    display: flex;
    gap: 1.5rem;
    align-items: flex-start;
    flex-wrap: wrap;
}

/* Scrollable, size-bounded window over the canvas. The canvas itself can
   be larger than the viewport (zoomed in); this wrapper clips + scrolls
   it so it never overflows the page. flex: 1 1 0 lets it take available
   width while min-width:0 allows it to actually shrink in a flex row. */
.canvas-viewport {
    flex: 1 1 480px;
    min-width: 0;
    max-height: 78vh;
    overflow: auto;
    border: 1px solid #333;
    background: #000;
    /* containment + a hint that wheel events are handled here, not scrolled */
    overscroll-behavior: contain;
}

canvas {
    display: block;
    background: #000;
    cursor: crosshair;
    image-rendering: pixelated;
}

.hud {
    background: #222;
    border: 1px solid #333;
    border-radius: 4px;
    padding: 1rem;
    min-width: 240px;
    font-family: monospace;
}

.hud h3 {
    margin: 0 0 0.5rem 0;
    color: #6fb;
    font-size: 0.9rem;
    text-transform: uppercase;
    letter-spacing: 1px;
}

.hud h3:not(:first-child) {
    margin-top: 1rem;
}

.z-nav, .zoom-nav {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
}

.z-btn {
    background: #333;
    color: #fff;
    border: 1px solid #555;
    border-radius: 3px;
    padding: 0.25rem 0.6rem;
    cursor: pointer;
    font-family: inherit;
    font-size: 0.9rem;
}

.z-btn:hover:not(:disabled) {
    background: #444;
    border-color: #777;
}

.z-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
}

.z-current {
    color: #ddd;
    font-size: 0.95rem;
}

.z-current strong {
    color: #6fb;
}

.z-hint {
    color: #666;
    font-size: 0.75rem;
    margin: 0.4rem 0 0 0;
}

.coord-row {
    display: flex;
    gap: 0.5rem;
    align-items: baseline;
    margin: 0.25rem 0;
}

.coord-row .label {
    color: #888;
    width: 70px;
}

.coord-row .coord {
    color: #fff;
}

.coord-row .coord.muted {
    color: #555;
}

.legend, .furniture-legend {
    list-style: none;
    padding: 0;
    margin: 0;
    font-size: 0.85rem;
}

.legend li, .furniture-legend li {
    margin: 0.15rem 0;
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.swatch {
    display: inline-block;
    width: 14px;
    height: 14px;
    border: 1px solid #444;
}

.swatch.wall   { background: #555; }
.swatch.floor  { background: #888; }
.swatch.ramp   { background: #a07040; }
.swatch.stair  { background: #665; }
.swatch.water  { background: #3a8fc7; }
.swatch.magma  { background: #d04020; }
.swatch.tree   { background: #2d8a3f; }

.furniture-legend code {
    background: #333;
    padding: 0 0.3rem;
    border-radius: 2px;
    color: #fff;
}

.note {
    margin-top: 1.5rem;
    font-size: 0.8rem;
    color: #666;
}
</style>
