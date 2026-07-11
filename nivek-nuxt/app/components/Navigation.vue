<script setup lang="ts">
const auth = useAuthStore()
const route = useRoute()
</script>

<template>
  <nav class="nav">
    <div v-if="route.path !== '/'"><NuxtLink to="/">Home</NuxtLink></div>
    <div v-if="route.path !== '/df'"><NuxtLink to="/df">DF Dashboard</NuxtLink></div>
    <!--
      Plain <a>, not <NuxtLink>: /api/auth/twitch/start is a backend route
      that issues a 302 to Twitch. NuxtLink would intercept and try to match
      against app routes.
    -->
    <div v-if="!auth.user"><a href="/api/auth/twitch/start">Sign in with Twitch</a></div>
    <div><a href="/devlog">Devlog</a></div>
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
