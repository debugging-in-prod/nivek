// Normalization for the public GET /api/stats/public payload (core-api,
// PR #180). The front page renders whatever this returns, so it must never
// throw: any malformed/missing field degrades to "no data" rather than
// breaking the page. Kept dependency-free (no Nuxt auto-imports) so it can be
// unit-tested with `node --test`.

export interface PublicStatsChannel {
  display_name: string
  uses: number
}

export interface PublicStatsFisher {
  display_name: string
  score: number
  catches: number
}

export interface PublicStats {
  active_chats: number | null
  active_chats_definition: string
  top_channels: PublicStatsChannel[]
  // 'all_time' since issue #184 (then top_channels_window_days is 0, kept
  // for compatibility). null for older payloads, which only carry the days.
  top_channels_window: 'all_time' | null
  top_channels_window_days: number | null
  top_fishers: PublicStatsFisher[]
  generated_at: string | null
}

export const MAX_TOP_CHANNELS = 5
export const MAX_TOP_FISHERS = 10
const MAX_NAME_LEN = 64
const MAX_DEFINITION_LEN = 500

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v)
}

function toCount(v: unknown): number | null {
  if (typeof v !== 'number' || !Number.isFinite(v) || v < 0) return null
  return Math.floor(v)
}

function toName(v: unknown): string | null {
  if (typeof v !== 'string') return null
  const name = v.trim()
  if (name === '') return null
  return name.length > MAX_NAME_LEN ? name.slice(0, MAX_NAME_LEN) : name
}

export function normalizePublicStats(raw: unknown): PublicStats | null {
  if (!isRecord(raw)) return null

  const channels: PublicStatsChannel[] = []
  if (Array.isArray(raw.top_channels)) {
    for (const c of raw.top_channels) {
      if (channels.length >= MAX_TOP_CHANNELS) break
      if (!isRecord(c)) continue
      const name = toName(c.display_name)
      const uses = toCount(c.uses)
      if (name === null || uses === null) continue
      channels.push({ display_name: name, uses })
    }
  }

  const fishers: PublicStatsFisher[] = []
  if (Array.isArray(raw.top_fishers)) {
    for (const f of raw.top_fishers) {
      if (fishers.length >= MAX_TOP_FISHERS) break
      if (!isRecord(f)) continue
      const name = toName(f.display_name)
      const score = toCount(f.score)
      if (name === null || score === null) continue
      fishers.push({ display_name: name, score, catches: toCount(f.catches) ?? 0 })
    }
  }

  const definition =
    typeof raw.active_chats_definition === 'string'
      ? raw.active_chats_definition.trim().slice(0, MAX_DEFINITION_LEN)
      : ''

  const generatedAt =
    typeof raw.generated_at === 'string' && !Number.isNaN(Date.parse(raw.generated_at))
      ? raw.generated_at
      : null

  const windowDays = toCount(raw.top_channels_window_days)

  return {
    active_chats: toCount(raw.active_chats),
    active_chats_definition: definition,
    top_channels: channels,
    top_channels_window: raw.top_channels_window === 'all_time' ? 'all_time' : null,
    top_channels_window_days: windowDays && windowDays > 0 ? windowDays : null,
    top_fishers: fishers,
    generated_at: generatedAt,
  }
}

// True when the payload carries nothing worth showing.
export function isPublicStatsEmpty(s: PublicStats): boolean {
  return (
    (s.active_chats === null || s.active_chats === 0) &&
    s.top_channels.length === 0 &&
    s.top_fishers.length === 0
  )
}

// Parenthetical label for the "Top channels" heading. A missing or zero day
// window means the API counts all time, so it never falls back to a vague
// "recently".
export function topChannelsWindowLabel(s: PublicStats): string {
  const days = s.top_channels_window_days
  if (s.top_channels_window === 'all_time' || !days) return 'all time'
  return days === 1 ? 'last 1 day' : `last ${days} days`
}

const countFormatter = new Intl.NumberFormat('en-US')

export function formatCount(n: number): string {
  return countFormatter.format(n)
}
