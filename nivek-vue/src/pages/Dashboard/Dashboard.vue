<script setup lang="ts">
import { useAuthStore } from '@/stores/auth'
import Weather from '@/components/Weather.vue'
import AutoShout from "@/components/AutoShout.vue";
import FishScore from "@/components/FishScore.vue";

const auth = useAuthStore()

function getGreeting(date: Date = new Date()): string {
  const hour = date.getHours()
  if (hour >= 12 && hour < 18) {
    return "Good Afternoon"
  } else if (hour >= 18 || hour < 5) {
    return "Good Evening"
  } else {
    return "Good Morning"
  }
}

</script>

<template>
  <div>
    <h1 v-if="auth.user" class="text-center green">{{ getGreeting() }} {{ auth.user?.username }}</h1>
    <Weather />
  </div>

  <div class="container">
    <div class="row">
      <div class="col-md-6 mb-5">
        <AutoShout />
      </div>

      <div class="col-md-6">
        <FishScore />
      </div>
    </div>
  </div>

  <p class="small text-center disclaimer">
    This bot is in very early stages of active development. Expect bugs, errors, failures, letdowns, mischeif, mayhem
    disappointments, heartbreaks, debauchery, trickery, and general disruptive behavior
  </p>
</template>

<style scoped>
.disclaimer {
  bottom: 2rem;
  left: 0;
  position: absolute;
  width: 100vw;
}
</style>