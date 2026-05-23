<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { fetchSnapshot } from '@/services/DFService'
import type { MapSnapshot, ZLevel } from '@/types/df'
import { drawLevel, pixelToWorld } from './renderer'

// Phase 1: poll the snapshot endpoint on a slow timer. Update frequency
// matches the planned executor push cadence (1-5 min); 60s here is a
// reasonable middle so the page feels alive without burning bandwidth.
const POLL_INTERVAL_MS = 60_000

const canvasRef = ref<HTMLCanvasElement | null>(null)
const snapshot = ref<MapSnapshot | null>(null)
const error = ref<string | null>(null)

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
        // Default to the highest Z level (surface or above) on first load.
        // Subsequent polls preserve the user's current Z if still in range.
        if (currentLevelIdx.value >= fresh.levels.length || currentLevelIdx.value < 0) {
            currentLevelIdx.value = fresh.levels.length - 1
        }
        renderCurrent()
    } catch (err: any) {
        error.value = err?.message ?? 'unknown error'
    }
}

function renderCurrent() {
    if (!canvasRef.value || !snapshot.value || !currentLevel.value) return
    drawLevel(canvasRef.value, snapshot.value, currentLevel.value)
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
    hoverCoord.value = pixelToWorld(snapshot.value, currentLevel.value, ev.clientX - rect.left, ev.clientY - rect.top)
}

function onMouseLeave() {
    hoverCoord.value = null
}

function onClick(ev: MouseEvent) {
    if (!snapshot.value || !canvasRef.value || !currentLevel.value) return
    const rect = canvasRef.value.getBoundingClientRect()
    selectedCoord.value = pixelToWorld(snapshot.value, currentLevel.value, ev.clientX - rect.left, ev.clientY - rect.top)
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

// Re-render whenever the active Z level changes (button click, key press,
// or snapshot refresh shifting the index).
watch(currentLevelIdx, renderCurrent)

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
            <h1>DF Twitch-plays — live overview</h1>
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
                Pusher sends one every {{ POLL_INTERVAL_MS / 1000 }}s.
            </p>
        </div>

        <div v-else class="layout">
            <canvas
                ref="canvasRef"
                @mousemove="onMouseMove"
                @mouseleave="onMouseLeave"
                @click="onClick"
            />
            <aside class="hud">
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
            Phase 1 proof-of-concept. Snapshot is currently a hand-crafted fixture; real DFHack data
            wiring lands in the next phase. Snapshot polls every {{ POLL_INTERVAL_MS / 1000 }}s.
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

header h1 {
    margin: 0 0 0.25rem 0;
    color: #6fb;
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
}

canvas {
    background: #000;
    cursor: crosshair;
    border: 1px solid #333;
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

.z-nav {
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
