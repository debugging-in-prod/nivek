import type { MapSnapshot } from '~/types/df'

// DF dashboard endpoint — public, no auth. Uses plain $fetch (NOT the
// auth-attaching api()) so anonymous viewers can render the map without
// holding a JWT.
//
// Returns null when the server has no snapshot yet (HTTP 404 — happens
// immediately after a Vultr container restart, before the DFHost pusher
// delivers its first POST). Callers should render a "waiting for data"
// placeholder in that case rather than treating it as an error.
//
// Throws for actual failures (network errors, 5xx, malformed response).
export async function fetchSnapshot(): Promise<MapSnapshot | null> {
    try {
        return await $fetch<MapSnapshot>('/api/df/snapshot')
    } catch (err: unknown) {
        const statusCode = (err as { statusCode?: number; status?: number })?.statusCode
            ?? (err as { status?: number })?.status
        if (statusCode === 404) return null
        throw err
    }
}
