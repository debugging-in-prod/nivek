<script setup lang="ts">
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
</script>

<template>
  <nav class="nav">
    <div><RouterLink to="/">Home</RouterLink></div>
    <!--
      Plain <a>, not <RouterLink>: /api/auth/twitch/start is a backend route
      that issues a 302 to Twitch. Vue Router would intercept a <RouterLink>
      and try to match it against SPA routes.
    -->
    <div v-if="!auth.user"><a href="/api/auth/twitch/start">Sign in with Twitch</a></div>
  </nav>
</template>

<style scoped>
.nav {
  display: flex;
  justify-content: center;
  list-style: none;
  margin: 0 auto 2rem;
  padding: 0;
}

.nav div:not(:last-child)::after {
  color: #f2f2f2;
  content: '|';
  padding-inline: 5px;
}

</style>