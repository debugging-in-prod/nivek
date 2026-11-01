<script setup lang="ts">
// Public "Bot stats" panel for the front page. Data comes from core-api's
// public GET /api/stats/public (cached server-side for 60s). The site's /api
// prefix is routed straight to core-api by Traefik and the Nuxt server has no
// proxy for it, so — like every other page — this fetches client-side only
// (server: false) and lazily, so it never blocks or breaks page render. Any
// error, 503 or malformed body degrades to a muted message. All text is
// rendered via interpolation (escaped); no v-html.
import {
  normalizePublicStats,
  isPublicStatsEmpty,
  formatCount,
  topChannelsWindowLabel,
  type PublicStats,
} from '~/utils/publicStats'
import { API_ROUTES } from '~/utils/constants'

const { data, status } = useFetch(API_ROUTES.Public.Stats, {
  baseURL: '/api',
  key: 'public-stats',
  server: false,
  lazy: true,
  retry: 0,
  timeout: 10000,
  transform: (raw: unknown): PublicStats | null => normalizePublicStats(raw),
})

const stats = computed<PublicStats | null>(() => data.value ?? null)
const loading = computed(() => status.value === 'idle' || status.value === 'pending')
const unavailable = computed(
  () => !loading.value && (status.value === 'error' || stats.value === null),
)
const empty = computed(() => !!stats.value && isPublicStatsEmpty(stats.value))

const windowLabel = computed(() => (stats.value ? topChannelsWindowLabel(stats.value) : 'all time'))

const updatedLabel = computed(() => {
  const ts = stats.value?.generated_at
  if (!ts) return ''
  const d = new Date(ts)
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString()
})
</script>

<template>
  <section class="bot-stats" aria-labelledby="bot-stats-heading">
    <h2 id="bot-stats-heading">Bot stats</h2>

    <p v-if="loading" class="muted" aria-live="polite">Loading stats…</p>
    <p v-else-if="unavailable" class="muted">Stats are unavailable right now.</p>
    <p v-else-if="empty" class="muted">No data yet — check back soon.</p>

    <template v-else-if="stats">
      <div class="active">
        <span class="active-count">{{
          stats.active_chats === null ? '—' : formatCount(stats.active_chats)
        }}</span>
        <span class="active-label" :title="stats.active_chats_definition || undefined"
          >active chats</span
        >
        <p v-if="stats.active_chats_definition" class="caption">{{
          stats.active_chats_definition
        }}</p>
      </div>

      <div class="boards">
        <div class="board">
          <h3
            >Top channels <small>({{ windowLabel }})</small></h3
          >
          <p v-if="stats.top_channels.length === 0" class="muted">No data yet.</p>
          <ol v-else>
            <li v-for="(c, i) in stats.top_channels" :key="`c-${i}-${c.display_name}`">
              <span class="name">{{ c.display_name }}</span>
              <span class="value"
                >{{ formatCount(c.uses) }} {{ c.uses === 1 ? 'command' : 'commands' }}</span
              >
            </li>
          </ol>
        </div>

        <div class="board">
          <h3>Top fishers</h3>
          <p v-if="stats.top_fishers.length === 0" class="muted">No data yet.</p>
          <ol v-else>
            <li v-for="(f, i) in stats.top_fishers" :key="`f-${i}-${f.display_name}`">
              <span class="name">{{ f.display_name }}</span>
              <span class="value">
                {{ formatCount(f.score) }} pts · {{ formatCount(f.catches) }}
                {{ f.catches === 1 ? 'catch' : 'catches' }}
              </span>
            </li>
          </ol>
        </div>
      </div>

      <p v-if="updatedLabel" class="caption updated">Updated {{ updatedLabel }}</p>
    </template>
  </section>
</template>

<style scoped>
.bot-stats {
  background: var(--color-background-soft);
  border: 1px solid var(--color-border);
  border-radius: 10px;
  padding: 1.25rem 1.5rem;
  margin-top: 1.5rem;
}
.bot-stats h2 {
  margin: 0 0 1rem 0;
  color: var(--color-heading);
  font-size: 1.4rem;
}
.muted {
  color: var(--color-text);
  opacity: 0.7;
  margin: 0;
}
.caption {
  font-size: 0.85rem;
  opacity: 0.7;
  margin: 0.25rem 0 0 0;
}
.active {
  margin-bottom: 1.25rem;
}
.active-count {
  font-size: 2rem;
  font-weight: 600;
  color: hsla(160, 100%, 37%, 1);
  margin-right: 0.5rem;
}
.active-label {
  cursor: help;
  border-bottom: 1px dotted var(--color-border-hover);
}
.boards {
  display: grid;
  grid-template-columns: 1fr;
  gap: 1.25rem;
}
@media (min-width: 768px) {
  .boards {
    grid-template-columns: 1fr 1fr;
  }
}
.board h3 {
  font-size: 1.1rem;
  margin: 0 0 0.5rem 0;
  color: var(--color-heading);
}
.board h3 small {
  font-size: 0.8rem;
  opacity: 0.7;
  font-weight: normal;
}
.board ol {
  margin: 0;
  padding-left: 1.5rem;
}
.board li {
  padding: 0.2rem 0;
  border-bottom: 1px solid var(--color-border);
}
.board li::after {
  content: '';
  display: block;
  clear: both;
}
.board li:last-child {
  border-bottom: none;
}
.board li .name {
  overflow-wrap: anywhere;
}
.board li .value {
  float: right;
  margin-left: 1rem;
  opacity: 0.8;
  white-space: nowrap;
}
.updated {
  margin-top: 1rem;
  text-align: right;
}
</style>
