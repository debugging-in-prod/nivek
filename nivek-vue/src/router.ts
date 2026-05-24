import { createWebHistory, createRouter, RouteRecordRaw } from 'vue-router'

import Welcome from '@/pages/Welcome/Welcome.vue'
import LoginPage from '@/pages/Login/Login.vue'
import SignupPage from '@/pages/Signup/Signup.vue'
import DashboardPage from '@/pages/Dashboard/Dashboard.vue'
import DFPage from '@/pages/DF/DF.vue'
import DFCitizensPage from '@/pages/DF/DFCitizens.vue'

import { TokenManager } from '@/utils/TokenManager'
import { useAuthStore } from '@/stores/auth'

import { API_ROUTES } from '@/constants'

const routes: Array<RouteRecordRaw> = [
    { name: 'Welcome', path: '/', component: Welcome },
    { name: 'Login', path: API_ROUTES.Login, component: LoginPage },
    { name: 'Signup', path: API_ROUTES.Signup, component: SignupPage },
    {
        name: 'Dashboard',
        path: '/dashboard',
        component: DashboardPage,
        meta: { requiresAuth: true, roles: ['user', 'admin'] }
    },
    { name: 'DF', path: '/df', component: DFPage },
    { name: 'DFCitizens', path: '/df/citizens', component: DFCitizensPage },
]

// Switched from createMemoryHistory to createWebHistory so URLs reflect
// app state and direct links (e.g. https://nivek.life/df) work. nginx
// is already configured with `try_files $uri $uri/ /index.html =404`
// so SPA fallback handles unknown paths.
const router = createRouter({
    history: createWebHistory(),
    routes,
})

router.beforeEach((to, from, next) => {
    // Check if route requires authentication
    const requiresAuth = to.meta.requiresAuth
    const hideForAuth = to.meta.hideForAuth
    const requiresRole = to.meta.requiresRole

    const authStore = useAuthStore()
    const tokenManager = TokenManager.getInstance()
    const isAuthenticated = tokenManager.getToken() !== null

    // If user is authenticated but accessing login/register, redirect to dashboard
    if (hideForAuth && isAuthenticated) {
        next({ name: 'Dashboard' })
        return
    }

    // If route requires auth but user is not authenticated
    if (requiresAuth && !isAuthenticated) {
        next({
            name: 'Login',
            query: { redirect: to.fullPath } // Save intended destination
        })
        return
    }

    // Check role-based access
    if (requiresRole && isAuthenticated) {
        // Ensure user data is loaded
        if (!authStore.user) {
            console.warn('no user found!')
            console.log(authStore.user)
            next({ name: 'Login' })
            return
        }

        // Check if user has required role
        if (authStore.user?.role !== requiresRole) {
            next({ name: 'Dashboard' }) // Redirect to dashboard if insufficient permissions
            return
        }

        console.log('authentication successful')
    }

    if (isAuthenticated && to.name == 'Welcome') {
        next({name: 'Dashboard'})
        return
    }

    next()
})

export default router
