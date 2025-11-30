<script setup lang="ts">
import { useAuthStore } from '@/stores/auth'
import Weather from '@/components/Weather.vue'
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

// @TODO::add interface for auto shoutout whitelist editing

</script>

<template>
  <div class="text-center">
    <h1 v-if="auth.user" class="green">{{ getGreeting() }} {{ auth.user?.username }}</h1>
  </div>

  <Weather />
  <FishScore />


</template>

<style scoped>

</style>