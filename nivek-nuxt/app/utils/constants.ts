// Route paths consumed by api.ts / pages. The SPA had an API_URL constant
// that interpolated window.location at module load; Nuxt's $fetch handles
// same-origin resolution natively, so paths are stored relative to /api.
export const API_ROUTES = {
    // Twitch OAuth — the app navigates the browser to TwitchStart and the
    // backend handles the full redirect dance, landing the user back at
    // /auth/landing with the JWT in the URL fragment.
    TwitchStart: '/api/auth/twitch/start',
    AuthLanding: '/auth/landing',

    Secure: {
        Profile: '/profile',
        Weather: '/weather',
        GetFishScore: '/fishing',
        GetAutoShoutChatters: '/auto-shout',
        PostCreateAutoShoutChatter: '/auto-shout',
        PostUpdateAutoShoutChatter: (id: number) => `/auto-shout/${id}`,
        DeleteAutoShoutChatter: (id: number) => `/auto-shout/${id}`,
        PostCreateMessage: '/message',
        GetMessages: '/message',
        Tasks: {
            Create: (id: number) => `/user/${id}/task`,
            GetAll: (id: number) => `/user/${id}/task`,
        },
    },
} as const

export interface User {
    id: number
    username: string
    role: string
    createdAt: number
}

export interface Task {
    id: number
    uuid: string
    user_id: number
    title: string
    description: string
    priority: string
    status: string
    expires_at: string
    completed_at: string
    created_at: string
    updated_at: string
    is_important: boolean
    position: number
    estimated_duration: string
    actual_duration: string
}
