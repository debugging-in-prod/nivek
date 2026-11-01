// User-facing text for a failed api()/$fetch call (typescript.md §6).
// 4xx responses carry a validation message from core-api as
// `{ "error": "…" }`, so show it. Anything else (5xx, network failure, fetch
// internals) gets the caller's friendly fallback instead of a raw
// `err.message`. Kept dependency-free so it can be unit-tested with
// `node --test`.

interface FetchErrorLike {
  statusCode?: number
  status?: number
  response?: { status?: number }
  data?: { error?: unknown; message?: unknown }
}

export function errorMessage(err: unknown, fallback: string): string {
  if (!err || typeof err !== 'object') return fallback
  const e = err as FetchErrorLike
  const status = e.statusCode ?? e.status ?? e.response?.status
  if (typeof status !== 'number' || status < 400 || status >= 500) return fallback
  const data = e.data
  if (!data || typeof data !== 'object') return fallback
  for (const text of [data.error, data.message]) {
    if (typeof text === 'string' && text.trim()) return text.trim()
  }
  return fallback
}
