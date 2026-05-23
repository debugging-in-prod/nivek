<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { fetchSnapshot } from '@/services/DFService'
import type { MapSnapshot } from '@/types/df'
import { drawSnapshot, pixelToWorld } from './renderer'

// Phase 1: poll the snapshot endpoint on a slow timer. Update frequency
// matches the planned executor push cadence (1-5 min); 60s here is a
// reasonable middle so the page feels alive without burning bandwidth.
const POLL_INTERVAL_MS = 60_000

const canvasRef = ref<HTMLCanvasElement | null>(null)
const snapshot = ref<MapSnapshot | null>(null)
const error = ref<string | null>(null)

const hoverCoord = ref<{ x: number; y: number; z: number } | null>(null)
const selectedCoord = ref<{ x: number; y: number; z: number } | null>(null)

let pollTimer: number | undefined

async function loadSnapshot() {
    try {
        snapshot.value = await fetchSnapshot()
        error.value = null
        renderCurrent()
    } catch (err: any) {
        error.value = err?.message ?? 'unknown error'
    }
}

function renderCurrent() {
    if (!canvasRef.value || !snapshot.value) return
    drawSnapshot(canvasRef.value, snapshot.value)
}

function onMouseMove(ev: MouseEvent) {
    if (!snapshot.value || !canvasRef.value) return
    const rect = canvasRef.value.getBoundingClientRect()
    hoverCoord.value = pixelToWorld(snapshot.value, ev.clientX - rect.left, ev.clientY - rect.top)
}

function onMouseLeave() {
    hoverCoord.value = null
}

function onClick(ev: MouseEvent) {
    if (!snapshot.value || !canvasRef.value) return
    const rect = canvasRef.value.getBoundingClientRect()
    selectedCoord.value = pixelToWorld(snapshot.value, ev.clientX - rect.left, ev.clientY - rect.top)
}

onMounted(() => {
    loadSnapshot()
    pollTimer = window.setInterval(loadSnapshot, POLL_INTERVAL_MS)
})

onBeforeUnmount(() => {
    if (pollTimer !== undefined) window.clearInterval(pollTimer)
})
</script>

<template>
    <div class="df-page">
        <header>
            <h1>DF Twitch-plays — live overview</h1>
            <p v-if="snapshot" class="capture-info">
                Captured {{ new Date(snapshot.captured_at).toLocaleString() }}
                · Z={{ snapshot.z }}
                · {{ snapshot.width }}×{{ snapshot.height }} tiles
                · origin ({{ snapshot.origin.x }}, {{ snapshot.origin.y }}, {{ snapshot.origin.z }})
            </p>
        </header>

        <div v-if="error" class="error">Snapshot unavailable: {{ error }}</div>

        <div class="layout">
            <canvas
                ref="canvasRef"
                @mousemove="onMouseMove"
                @mouseleave="onMouseLeave"
                @click="onClick"
            />
            <aside class="hud">
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
    min-width: 220px;
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
