import { defineStore } from 'pinia'
import { TokenManager } from '~/utils/TokenManager'
import { api } from '~/utils/api'
import type { User } from '~/utils/constants'

export const useAuthStore = defineStore('auth', () => {
    const user = ref<User | null>(null)

    // Computed against TokenManager so getter reads the latest localStorage
    // state even after a manual clearToken() outside the store.
    const token = computed(() => TokenManager.getInstance().getToken())
    const isAuthenticated = computed(() => !!token.value)

    // Browser-only side effect: the api util dispatches 'auth:unauthorized'
    // on 401 so any caller — not just the one whose request 401'd — drops
    // the user out of an authenticated UI state.
    if (typeof window !== 'undefined') {
        window.addEventListener('auth:unauthorized', () => {
            logout()
        })
    }

    const logout = () => {
        TokenManager.getInstance().clearToken()
        user.value = null
    }

    // Called from the client-side auth plugin at app startup. No-op on
    // server because the token lives in localStorage.
    const initAuth = async () => {
        if (typeof window === 'undefined') return
        if (isAuthenticated.value) {
            await fetchUserProfile()
        }
    }

    const fetchUserProfile = async () => {
        try {
            const profile = await api<User>('/api/profile', { method: 'POST' })
            user.value = profile
        } catch (error) {
            console.error('Failed to fetch user profile:', error)
        }
    }

    return {
        user,
        token,
        isAuthenticated,
        logout,
        initAuth,
        fetchUserProfile,
    }
})
