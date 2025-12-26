<script setup lang="ts">
import Navigation from "@/components/Navigation.vue"
import Logout from '@/components/Logout.vue'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
</script>

<template>
  <header>
    <img v-if="auth.user" class="logo" src="./assets/munk.gif" width="125" height="125"/>
    <img v-else class="logo" src="./assets/pepelogo.jpg" width="125" height="125">
  </header>
  <Navigation/>

  <div v-if="auth.user"><Logout/></div>

  <main>
    <div class="wrapper">
      <RouterView />
    </div>
  </main>
  

  <p class="small text-center disclaimer p-5">
    This bot is in very early stages of active development. Expect bugs, errors, failures, letdowns, mischeif, mayhem
    disappointments, heartbreaks, debauchery, trickery, and general disruptive behavior
  </p>
</template>

<style scoped>
.disclaimer {
  background: var(--color-background);
  margin: 0;
}
header {
  line-height: 1.5;
}

.logo {
  display: block;
  margin: 0 auto 2rem;
}

@media (min-width: 1024px) {
  header {
    display: flex;
    place-items: center;
  }

  .logo {
    margin: 0 2rem 0 0;
  }

  header .wrapper {
    display: flex;
    place-items: flex-start;
    flex-wrap: wrap;
  }
}
</style>
