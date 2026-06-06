<script setup lang="ts">
// Final stop in the Twitch OAuth flow: the backend's /auth/twitch/callback
// 302s the browser here with the JWT in the URL fragment (#token=...). We
// pull it out, hand it to the TokenManager, strip the fragment so the token
// doesn't sit in window history, fetch the user profile, then continue to
// /dashboard. If the backend redirected here with ?error=... we surface that.
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { TokenManager } from '@/utils/TokenManager'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()
const error = ref('')

onMounted(async () => {
    const params = new URLSearchParams(window.location.search)
    const backendError = params.get('error')
    if (backendError) {
        error.value = backendError
        return
    }

    // Fragment starts with '#', URLSearchParams wants what comes after.
    const fragment = window.location.hash.startsWith('#')
        ? window.location.hash.slice(1)
        : window.location.hash
    const fragParams = new URLSearchParams(fragment)
    const token = fragParams.get('token')

    if (!token) {
        error.value = 'no_token_in_callback'
        return
    }

    TokenManager.getInstance().setToken(token)

    // Replace the URL so the token vanishes from the address bar and won't be
    // restored on reload. replaceState avoids adding a new history entry.
    window.history.replaceState({}, '', '/auth/landing')

    await auth.fetchUserProfile()
    await router.replace('/dashboard')
})
</script>

<template>
    <div class="auth-landing">
        <p v-if="!error">Finishing sign-in…</p>
        <div v-else>
            <h2 class="red">Sign-in failed</h2>
            <p>Reason: <code>{{ error }}</code></p>
            <p><a href="/api/auth/twitch/start">Try again</a></p>
        </div>
    </div>
</template>

<style scoped>
.auth-landing {
    max-width: 480px;
    margin: 4rem auto;
    text-align: center;
}
.red {
    color: #e06c75;
}
</style>
