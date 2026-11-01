import type { MapSnapshot } from '~/types/df'
import { api } from '~/utils/api'
import { API_ROUTES } from '~/utils/constants'

// DF dashboard endpoint — public, no auth. Goes through the shared api()
// client like every other core-api call; anonymous viewers simply have no
// session cookie, which this endpoint doesn't need.
//
// Returns null when the server has no snapshot yet (HTTP 404 — happens
// immediately after a Vultr container restart, before the DFHost pusher
// delivers its first POST). Callers should render a "waiting for data"
// placeholder in that case rather than treating it as an error.
//
// Throws for actual failures (network errors, 5xx, malformed response).
export async function fetchSnapshot(): Promise<MapSnapshot | null> {
    try {
        return await api<MapSnapshot>(API_ROUTES.Public.DfSnapshot)
    } catch (err: unknown) {
        const statusCode = (err as { statusCode?: number; status?: number })?.statusCode
            ?? (err as { status?: number })?.status
        if (statusCode === 404) return null
        throw err
    }
}
