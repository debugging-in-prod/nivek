import { API_URL } from '@/constants'
import type { MapSnapshot } from '@/types/df'

// DF dashboard endpoint — public, no auth, simple fetch (intentionally
// bypasses the JWT-aware HttpClient since the dashboard is anonymous).
//
// Returns null when the server has no snapshot yet (HTTP 404 — happens
// immediately after a Vultr container restart, before the DFHost pusher
// delivers its first POST). Callers should render a "waiting for data"
// placeholder in that case rather than treating it as an error.
//
// Throws for actual failures (network errors, 5xx, malformed response).
export async function fetchSnapshot(): Promise<MapSnapshot | null> {
    const res = await fetch(`${API_URL}/df/snapshot`, { method: 'GET' })
    if (res.status === 404) {
        return null
    }
    if (!res.ok) {
        throw new Error(`snapshot fetch failed: ${res.status} ${res.statusText}`)
    }
    return (await res.json()) as MapSnapshot
}
