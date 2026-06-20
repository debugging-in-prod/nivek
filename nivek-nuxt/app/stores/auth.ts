import { defineStore } from 'pinia'

// Minimal placeholder during the SPA→SSR migration. Real implementation
// (TokenManager, /profile fetch, auth:unauthorized listener) lands when
// the auth flow is ported. Until then components can rely on the shape
// (user/isAuthenticated) without crashing during SSR.
export interface User {
  id: string
  role: string
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const isAuthenticated = computed(() => user.value !== null)

  return { user, isAuthenticated }
})
