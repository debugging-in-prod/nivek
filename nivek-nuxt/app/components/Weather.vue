<script setup lang="ts">
import { API_ROUTES } from '~/utils/constants'
import { errorMessage } from '~/utils/errorMessage'
import type { WeatherReport } from '~/types/weather'

const weatherReport = ref<WeatherReport>({})
const loading = ref(true)
const error = ref<string | null>(null)

// External IP lookup uses plain $fetch: the shared api() client targets
// core-api (session cookie + CSRF header) and must not be pointed at a
// third-party service.
async function getWeather() {
  loading.value = true
  error.value = null
  try {
    const ip = await $fetch<string>('https://ipapi.co/ip/')
    if (!ip) {
      console.error('error fetching public IP')
      error.value = "Couldn't load the weather."
      return
    }
    weatherReport.value = await api<WeatherReport>(API_ROUTES.Secure.Weather, {
      method: 'POST',
      body: { ip },
    })
  } catch (err: unknown) {
    console.error('error fetching weather info: ', err)
    error.value = errorMessage(err, "Couldn't load the weather.")
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  getWeather()
})
</script>

<template>
  <div class="text-center mb-5">
    <p v-if="loading">Loading…</p>
    <p v-else-if="error" class="error" role="alert">{{ error }}</p>
    <p v-else
      >Weather in {{ weatherReport.city }}: <span class="green">{{ weatherReport.temp }}</span></p
    >
  </div>
</template>

<style scoped>
.error {
  color: #e5484d;
}
</style>
