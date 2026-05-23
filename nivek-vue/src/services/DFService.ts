import { API_URL } from '@/constants'
import type { MapSnapshot } from '@/types/df'

// DF dashboard endpoint — public, no auth, simple fetch (intentionally
// bypasses the JWT-aware HttpClient since the dashboard is anonymous).
export async function fetchSnapshot(): Promise<MapSnapshot> {
    const res = await fetch(`${API_URL}/df/snapshot`, { method: 'GET' })
    if (!res.ok) {
        throw new Error(`snapshot fetch failed: ${res.status} ${res.statusText}`)
    }
    return (await res.json()) as MapSnapshot
}
