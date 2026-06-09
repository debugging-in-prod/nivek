<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { fetchSnapshot } from '@/services/DFService'
import type { MapSnapshot } from '@/types/df'

// Poll faster than the DFHost pusher's 15s cadence so a new push surfaces
// within one poll cycle instead of waiting for an aligned tick.
const POLL_INTERVAL_MS = 10_000

const snapshot = ref<MapSnapshot | null>(null)
const error = ref<string | null>(null)
let pollTimer: number | undefined

const citizens = computed(() => snapshot.value?.citizens ?? [])

async function load() {
    try {
        snapshot.value = await fetchSnapshot()
        error.value = null
    } catch (err: any) {
        error.value = err?.message ?? 'unknown error'
    }
}

// Stress category from dfhack.units.getStressCategory. Per the DFHack Lua
// API the scale runs 0 = MOST stressed .. 6 = LEAST stressed (i.e. 0 is
// miserable, 6 is ecstatic) — the opposite of what reads intuitively, so
// the labels are ordered low→high to match.
const STRESS_LABELS = ['Miserable', 'Stressed', 'Unhappy', 'Fine', 'Content', 'Happy', 'Ecstatic']
function stressLabel(s: number): string {
    return STRESS_LABELS[s] ?? `?(${s})`
}
function stressClass(s: number): string {
    if (s >= 5) return 'stress-happy'               // Happy, Ecstatic
    if (s === 3 || s === 4) return 'stress-neutral' // Fine, Content
    if (s === 2) return 'stress-unhappy'            // Unhappy
    return 'stress-bad'                             // Stressed, Miserable (0–1)
}

// In-game elevation = raw embark-local z + snapshot offset. See DF.vue's
// toElev for the rationale; mirrored here so the position column reads in
// the same units as the in-game UI.
function toElev(z: number): number | string {
    if (!snapshot.value) return '—'
    return z + snapshot.value.z_offset
}

onMounted(() => {
    load()
    pollTimer = window.setInterval(load, POLL_INTERVAL_MS)
})
onBeforeUnmount(() => {
    if (pollTimer !== undefined) window.clearInterval(pollTimer)
})
</script>

<template>
    <div class="df-page">
        <header>
            <div class="header-row">
                <h1>DF Twitch-plays — citizens</h1>
                <div class="nav-group">
                    <router-link to="/df" class="nav-btn">← Map</router-link>
                    <router-link to="/df/help" class="nav-btn">Help</router-link>
                </div>
            </div>
            <p v-if="snapshot" class="capture-info">
                Captured {{ new Date(snapshot.captured_at).toLocaleString() }}
                · {{ citizens.length }} citizen{{ citizens.length === 1 ? '' : 's' }}
            </p>
            <p class="appoint-hint">
                Appoint a dwarf to a fort position in chat with its ID:
                <code>!DF appoint &lt;manager|bookkeeper|broker|doctor|commander&gt; &lt;id&gt;</code>
            </p>
        </header>

        <div v-if="error" class="error">Snapshot unavailable: {{ error }}</div>

        <div v-if="!snapshot && !error" class="waiting">
            <p class="waiting-text">Waiting for data…</p>
            <p class="waiting-hint">The pusher sends one every 15s once DF is running.</p>
        </div>

        <div v-else-if="snapshot && citizens.length === 0" class="waiting">
            <p class="waiting-text">No citizens</p>
            <p class="waiting-hint">The current snapshot has no citizens (empty fort, or none extracted).</p>
        </div>

        <table v-else-if="snapshot" class="citizen-table">
            <thead>
                <tr>
                    <th>Mood</th>
                    <th>ID</th>
                    <th>Name</th>
                    <th>Profession</th>
                    <th>Age</th>
                    <th>Current job</th>
                    <th>Position</th>
                </tr>
            </thead>
            <tbody>
                <tr v-for="(c, i) in citizens" :key="i">
                    <td>
                        <span class="stress-dot" :class="stressClass(c.stress)" :title="stressLabel(c.stress)" />
                        <span class="stress-text">{{ stressLabel(c.stress) }}</span>
                    </td>
                    <td class="id">#{{ c.id }}</td>
                    <td class="name">{{ c.name }}</td>
                    <td>{{ c.profession }}</td>
                    <td class="num">{{ c.age }}</td>
                    <td class="job"><em v-if="c.job">{{ c.job }}</em><span v-else class="muted">idle</span></td>
                    <td class="pos">({{ c.position.x }}, {{ c.position.y }}, {{ toElev(c.position.z) }})</td>
                </tr>
            </tbody>
        </table>
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

.nav-group {
    display: flex;
    gap: 0.5rem;
}

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
    margin: 0 0 0.25rem 0;
}

.appoint-hint {
    color: #888;
    font-size: 0.82rem;
    margin: 0 0 1rem 0;
}
.appoint-hint code {
    background: #2a3a30;
    color: #6fb;
    padding: 0.1rem 0.4rem;
    border-radius: 3px;
    font-size: 0.8rem;
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
    min-height: 300px;
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

.citizen-table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.88rem;
}
.citizen-table th {
    text-align: left;
    color: #6fb;
    font-size: 0.78rem;
    text-transform: uppercase;
    letter-spacing: 1px;
    border-bottom: 1px solid #333;
    padding: 0.4rem 0.75rem;
}
.citizen-table td {
    padding: 0.35rem 0.75rem;
    border-bottom: 1px solid #242424;
}
.citizen-table tbody tr:hover {
    background: #202020;
}
.citizen-table .id {
    font-family: monospace;
    color: #6fb;
    white-space: nowrap;
}
.citizen-table .name {
    color: #fff;
    font-weight: bold;
}
.citizen-table .num {
    text-align: right;
}
.citizen-table .job {
    color: #6fb;
}
.citizen-table .job .muted {
    color: #555;
    font-style: italic;
}
.citizen-table .pos {
    color: #888;
    font-family: monospace;
    font-size: 0.8rem;
}

.stress-dot {
    display: inline-block;
    width: 8px;
    height: 8px;
    border-radius: 50%;
    margin-right: 0.4rem;
    vertical-align: middle;
}
.stress-text {
    font-size: 0.8rem;
    color: #aaa;
}
.stress-dot.stress-happy   { background: #4caf50; }
.stress-dot.stress-neutral { background: #aaa;    }
.stress-dot.stress-unhappy { background: #ff9800; }
.stress-dot.stress-bad     { background: #f44336; }
</style>
