<script setup lang="ts">
import Navigation from "@/components/Navigation.vue"
import Logout from '@/components/Logout.vue'
import { useAuthStore } from '@/stores/auth'
import { BUILD_VERSION } from '@/version'

const auth = useAuthStore()
</script>

<template>
  <header>
    <img v-if="auth.user" class="logo" src="./assets/munk.gif" width="125" height="125"/>
  </header>
  <Navigation/>

  <div v-if="auth.user"><Logout/></div>

  <main>
    <div class="wrapper">
      <RouterView />
    </div>
  </main>

  <footer class="build-tag">
    peanutbudderbot · build <code>{{ BUILD_VERSION }}</code>
  </footer>
</template>

<style scoped>
.build-tag {
  font-size: 0.75rem;
  color: var(--color-text-muted, #888);
  text-align: right;
  padding: 0.5rem 1rem;
}
.build-tag code {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
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
