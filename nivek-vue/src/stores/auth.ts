import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { createHttpClient } from '@/services/HttpClient'
import { AxiosAdapter } from '@/services/AxiosAdapter'
import { TokenManager } from '@/utils/TokenManager'
import { User } from '@/constants'

export const useAuthStore = defineStore('auth', () => {
    const user = ref<User | null>(null)
    const tokenManager = TokenManager.getInstance()

    const httpClient = createHttpClient(AxiosAdapter)

    const token = computed(() => tokenManager.getToken())
    const isAuthenticated = computed(() => !!token.value)

    if (typeof window !== 'undefined') {
        window.addEventListener('auth:unauthorized', () => {
            logout()
        })
    }

    const logout = () => {
        tokenManager.clearToken()
        user.value = null
    }

    const initAuth = () => {
        if (isAuthenticated.value) {
            fetchUserProfile()
        }
    }

    const fetchUserProfile = async () => {
        try {
            const userProfileResponse = await httpClient.post(`/profile`)
            user.value = userProfileResponse.data
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
        fetchUserProfile
    }
})
