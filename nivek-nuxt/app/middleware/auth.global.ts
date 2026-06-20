// Global route guard. Now that `/` serves both the Welcome shell and
// the authed dashboard via conditional render, there's no separate
// /dashboard route to gate or redirect to — the only remaining job is
// kicking an already-authed user off the OAuth landing page if they
// somehow navigate back to it (e.g. browser-back after the success
// redirect to /).
//
// Server-side: early-out. JWT lives in localStorage so the server has
// no auth signal anyway.
export default defineNuxtRouteMiddleware((to) => {
    if (import.meta.server) return

    const auth = useAuthStore()
    if (to.meta.hideForAuth && auth.isAuthenticated) {
        return navigateTo('/')
    }
})
