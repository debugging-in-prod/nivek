<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'
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

// Zoom: pixels per tile. The sidebar +/− buttons adjust it; the canvas
// re-renders at the new scale and the scrollable viewport wrapper keeps it
// contained. Mouse wheel is left to the browser for native scrolling.
const cellSize = ref<number>(DEFAULT_CELL_SIZE)

// Index into snapshot.levels[] of the currently-displayed Z level. On the
// first snapshot it's set to the F1 hotkey's Z (or the middle of the range
// if no focus); later polls preserve whatever level the user navigated to.
const currentLevelIdx = ref<number>(0)

// First-load gate. The initial view (Z level + centered scroll) is set
// once from the snapshot's focus; subsequent polls must not yank the view
// out from under the user.
let hasInitialized = false

type Coord = { x: number; y: number; z: number }
const hoverCoord = ref<Coord | null>(null)
// Multiple tiles can be selected at once; clicking a selected tile again
// deselects it, and the sidebar "clear" button empties the list.
const selectedCoords = ref<Coord[]>([])

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

// In-game elevation = raw embark-local z + snapshot offset. Offset is
// constant per fort; computed in the lua scanner from world map.region_z
// minus DF's sea-level reference (100). Display only — internal lookups
// (level index, click/hover comparisons) continue to use raw z.
function toElev(z: number): number | string {
    if (!snapshot.value) return '—'
    return z + snapshot.value.z_offset
}

// Elevation bounds for the jump input's placeholder. Levels are sorted
// ascending by z and contiguous (per the wire contract), so first/last
// give the full range without scanning.
const minElev = computed(() => {
    if (!snapshot.value || snapshot.value.levels.length === 0) return null
    return snapshot.value.levels[0].z + snapshot.value.z_offset
})
const maxElev = computed(() => {
    if (!snapshot.value || snapshot.value.levels.length === 0) return null
    return snapshot.value.levels[snapshot.value.levels.length - 1].z + snapshot.value.z_offset
})

// v-model on <input type="number"> hands us a number when populated and
// the empty string when cleared (Vue 3 implicitly applies the `.number`
// modifier for type="number" inputs). Type the ref as the union so the
// guard below covers both.
const jumpElev = ref<number | ''>('')

// Jump to the level whose elevation matches the input. Out-of-range
// targets clamp to the nearest valid level (avoids dead-ending the user
// on a typo / mis-remembered number); empty / non-numeric input is a no-op.
function jumpToElev() {
    if (!snapshot.value) return
    const target = jumpElev.value
    if (typeof target !== 'number' || !Number.isFinite(target)) return
    const levels = snapshot.value.levels
    if (levels.length === 0) return
    const rawZ = Math.floor(target - snapshot.value.z_offset)
    let idx = rawZ - levels[0].z
    if (idx < 0) idx = 0
    if (idx >= levels.length) idx = levels.length - 1
    currentLevelIdx.value = idx
    jumpElev.value = ''
}

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
        const firstLoad = !hasInitialized
        snapshot.value = fresh
        if (firstLoad) {
            hasInitialized = true
            currentLevelIdx.value = pickInitialLevel(fresh)
            // Wait for the canvas to mount + size at the chosen level, then
            // scroll so the F1 hotkey tile is centered in the viewport.
            await nextTick()
            renderCurrent()
            if (fresh.focus) {
                const f = fresh.focus
                // rAF: scrollLeft/Top clamp to the canvas's laid-out size,
                // which is only settled after the post-render reflow.
                requestAnimationFrame(() => centerOn(f.x, f.y))
            }
        } else {
            // Preserve the user's current Z if still in range; otherwise clamp
            // back to the middle of the (possibly shifted) range.
            if (currentLevelIdx.value >= fresh.levels.length || currentLevelIdx.value < 0) {
                currentLevelIdx.value = Math.floor(fresh.levels.length / 2)
            }
            renderCurrent()
        }
    } catch (err: any) {
        error.value = err?.message ?? 'unknown error'
    }
}

// Initial Z level: the level matching the F1 hotkey's Z if the dump
// included one and it's within the dumped range, else the middle of the
// range. The lua dump emits `window_z ± Z_RADIUS`, so the middle index is
// DF's viewport at dump time — where the fortress almost always is. The
// top of the range is usually empty above-ground sky (all Unknown tiles,
// a featureless dark rectangle that looks broken to viewers).
function pickInitialLevel(snap: MapSnapshot): number {
    if (snap.focus) {
        const i = snap.levels.findIndex(l => l.z === snap.focus!.z)
        if (i >= 0) return i
    }
    return Math.floor(snap.levels.length / 2)
}

// Scroll the viewport so the given world (x, y) tile sits at its center.
function centerOn(worldX: number, worldY: number) {
    const vp = viewportRef.value
    if (!vp || !snapshot.value) return
    const localX = worldX - snapshot.value.origin.x
    const localY = worldY - snapshot.value.origin.y
    vp.scrollLeft = localX * cellSize.value - vp.clientWidth / 2
    vp.scrollTop = localY * cellSize.value - vp.clientHeight / 2
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
    const c = pixelToWorld(snapshot.value, currentLevel.value, ev.clientX - rect.left, ev.clientY - rect.top, cellSize.value)
    if (!c) return
    // Toggle: clicking an already-selected tile removes it; otherwise add it.
    const idx = selectedCoords.value.findIndex(s => s.x === c.x && s.y === c.y && s.z === c.z)
    if (idx >= 0) {
        selectedCoords.value.splice(idx, 1)
    } else {
        selectedCoords.value.push(c)
    }
}

function clearSelection() {
    selectedCoords.value = []
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
            <div class="canvas-viewport" ref="viewportRef">
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
                <p class="z-hint">scroll the map to pan</p>

                <h3>Elevation</h3>
                <div class="z-nav">
                    <button
                        class="z-btn"
                        :disabled="!canGoUp"
                        @click="goUp"
                        aria-label="Go up one Z level"
                    >▲ up</button>
                    <span class="z-current">
                        Elev = <strong>{{ currentLevel ? toElev(currentLevel.z) : '—' }}</strong>
                    </span>
                    <button
                        class="z-btn"
                        :disabled="!canGoDown"
                        @click="goDown"
                        aria-label="Go down one Z level"
                    >▼ down</button>
                </div>
                <form class="elev-jump" @submit.prevent="jumpToElev">
                    <label for="elev-jump-input">Jump:</label>
                    <input
                        id="elev-jump-input"
                        class="elev-jump-input"
                        type="number"
                        inputmode="numeric"
                        :placeholder="minElev !== null && maxElev !== null ? `${minElev} to ${maxElev}` : '—'"
                        v-model="jumpElev"
                        :disabled="!snapshot"
                        aria-label="Jump to elevation"
                    />
                </form>
                <p class="z-hint">↑/↓ or PageUp/PageDown also work · type & ↵ to jump</p>

                <h3>Coordinates</h3>
                <div class="coord-row">
                    <span class="label">Hover:</span>
                    <span v-if="hoverCoord" class="coord">
                        ({{ hoverCoord.x }}, {{ hoverCoord.y }}, {{ toElev(hoverCoord.z) }})
                    </span>
                    <span v-else class="coord muted">—</span>
                </div>
                <div class="coord-row">
                    <span class="label">Selected:</span>
                    <span class="coord muted">{{ selectedCoords.length }} tile{{ selectedCoords.length === 1 ? '' : 's' }}</span>
                    <button
                        class="clear-btn"
                        :disabled="selectedCoords.length === 0"
                        @click="clearSelection"
                    >clear</button>
                </div>
                <ul v-if="selectedCoords.length" class="selected-list">
                    <li v-for="(c, i) in selectedCoords" :key="i" class="coord">
                        ({{ c.x }}, {{ c.y }}, {{ toElev(c.z) }})
                    </li>
                </ul>
                <p v-else class="z-hint">click tiles to select; click again to deselect</p>

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
    /* keep wheel scrolling inside the map from chaining to the page */
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

.elev-jump {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-top: 0.4rem;
}

.elev-jump label {
    color: #ddd;
    font-size: 0.85rem;
}

.elev-jump-input {
    flex: 1;
    min-width: 0;
    background: #1a1a1a;
    color: #fff;
    border: 1px solid #555;
    border-radius: 3px;
    padding: 0.25rem 0.5rem;
    font-family: inherit;
    font-size: 0.9rem;
}

.elev-jump-input:focus {
    outline: none;
    border-color: #6fb;
}

.elev-jump-input:disabled {
    opacity: 0.4;
    cursor: not-allowed;
}

.elev-jump-input::placeholder {
    color: #666;
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

.clear-btn {
    margin-left: auto;
    background: #333;
    color: #ddd;
    border: 1px solid #555;
    border-radius: 3px;
    padding: 0.1rem 0.55rem;
    cursor: pointer;
    font-family: inherit;
    font-size: 0.78rem;
}
.clear-btn:hover:not(:disabled) {
    background: #444;
    border-color: #777;
}
.clear-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
}

.selected-list {
    list-style: none;
    padding: 0;
    margin: 0.25rem 0 0 0;
    max-height: 160px;
    overflow-y: auto;
}
.selected-list .coord {
    color: #fff;
    font-size: 0.85rem;
    padding: 0.1rem 0;
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
.swatch.tree   { background: #6b3e1c; }

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
