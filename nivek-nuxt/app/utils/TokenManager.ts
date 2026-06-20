// Ported from nivek-vue. Singleton wrapper around localStorage that also
// reads a `jwt_token` cookie if present. The cookie path is dormant — the
// SPA used to write it, doesn't anymore (setSecureCookie is unused), but
// the reader is preserved so any cookie a future flow sets still works.
//
// SSR-safe: all browser API access is guarded so this can be imported in
// composables that run on both server and client.
export class TokenManager {
    private static instance: TokenManager
    private token: string | null = null

    private constructor() {
        this.loadToken()
    }

    static getInstance(): TokenManager {
        if (!TokenManager.instance) {
            TokenManager.instance = new TokenManager()
        }
        return TokenManager.instance
    }

    private loadToken(): void {
        if (typeof window === 'undefined') {
            this.token = null
            return
        }
        this.token = this.getTokenFromCookie() || localStorage.getItem('jwt_token')
    }

    private getTokenFromCookie(): string | null {
        if (typeof document === 'undefined') return null

        const cookies = document.cookie.split(';')
        for (const cookie of cookies) {
            const [name, value] = cookie.trim().split('=')
            if (name === 'jwt_token' && value !== undefined) {
                return decodeURIComponent(value)
            }
        }
        return null
    }

    setToken(token: string): void {
        this.token = token
        if (typeof window !== 'undefined') {
            localStorage.setItem('jwt_token', token)
        }
    }

    getToken(): string | null {
        if (!this.token) {
            this.loadToken()
        }
        return this.token
    }

    clearToken(): void {
        this.token = null
        if (typeof window === 'undefined') return
        localStorage.removeItem('jwt_token')
        document.cookie = 'jwt_token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/; secure; samesite=strict'
    }

    getAuthHeader(): Record<string, string> {
        const token = this.getToken()
        return token ? { Authorization: `Bearer ${token}` } : {}
    }
}
