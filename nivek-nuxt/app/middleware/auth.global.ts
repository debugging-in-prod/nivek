// Global route guard ported from the SPA's router.beforeEach. Decides
// auth-driven redirects (where to send a logged-out user trying to hit
// a protected route, where to send a logged-in user hitting Welcome).
//
// Server-side: early-out. The JWT lives in localStorage so the server
// can't make any auth decision. Pages SSR in their unauthenticated form
// and the actual gating happens at hydration / subsequent navigation.
export default defineNuxtRouteMiddleware((to) => {
    if (import.meta.server) return

    const auth = useAuthStore()
    const requiresAuth = to.meta.requiresAuth
    const hideForAuth = to.meta.hideForAuth
    const requiresRole = to.meta.requiresRole

    // Authenticated user landing on /auth/landing or Welcome — push them
    // straight to the dashboard. (hideForAuth on landing/login routes;
    // the explicit / check matches the SPA's beforeEach.)
    if (hideForAuth && auth.isAuthenticated) {
        return navigateTo('/dashboard')
    }
    if (auth.isAuthenticated && to.path === '/') {
        return navigateTo('/dashboard')
    }

    // Unauthenticated user hitting a protected route — back to Welcome,
    // where the Sign-in-with-Twitch entry point lives.
    if (requiresAuth && !auth.isAuthenticated) {
        return navigateTo('/')
    }

    if (requiresRole && auth.isAuthenticated) {
        if (!auth.user) {
            console.warn('no user found!')
            return navigateTo('/')
        }
        if (auth.user.role !== requiresRole) {
            return navigateTo('/dashboard')
        }
    }
})
