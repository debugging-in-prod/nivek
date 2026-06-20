// Client-only plugin that boots the auth store after hydration. Equivalent
// to the SPA's main.ts calling authStore.initAuth() at app startup.
//
// .client.ts suffix scopes this to the browser — the JWT lives in
// localStorage, so there's nothing to do on the server.
export default defineNuxtPlugin(async () => {
    const auth = useAuthStore()
    await auth.initAuth()
})
