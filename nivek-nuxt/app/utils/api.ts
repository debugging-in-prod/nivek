// Configured $fetch instance for authenticated calls to /api/*.
// Replaces the SPA's AxiosAdapter + HttpClient pair.
//
// - Attaches the Authorization: Bearer <jwt> header from TokenManager
//   on every request, but ONLY on the client — the server has no
//   access to localStorage so the JWT is unreachable during SSR.
//   Auth-gated fetches must therefore happen client-side (in onMounted,
//   in event handlers, or via useAsyncData with { server: false }).
//
// - On 401, clears the token and emits the `auth:unauthorized`
//   window event the auth store listens for. Same behavior as the
//   axios response interceptor.
//
// Note this is the BARE configured instance, not a wrapper that
// returns { data, headers, status }. Call sites get the response body
// directly, matching $fetch's native shape.
import { TokenManager } from './TokenManager'

export const api = $fetch.create({
    onRequest({ options }) {
        if (typeof window === 'undefined') return
        const token = TokenManager.getInstance().getToken()
        if (!token) return
        const headers = new Headers(options.headers as HeadersInit | undefined)
        headers.set('Authorization', `Bearer ${token}`)
        options.headers = headers
    },
    onResponseError({ response }) {
        if (typeof window === 'undefined') return
        if (response.status === 401) {
            TokenManager.getInstance().clearToken()
            window.dispatchEvent(new CustomEvent('auth:unauthorized'))
        }
    },
})
