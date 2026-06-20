<script setup lang="ts">
import { API_ROUTES } from '~/utils/constants'

interface WeatherReport {
    city?: string
    temp?: string
}

const weatherReport = ref<WeatherReport>({})

// External IP lookup uses plain $fetch (no JWT). Auth-attaching api()
// would leak the Bearer token to a third-party service.
async function getWeather() {
    try {
        const ip = await $fetch<string>('https://ipapi.co/ip/')
        if (!ip) {
            console.error('error fetching public IP')
            return
        }
        weatherReport.value = await api<WeatherReport>(API_ROUTES.Secure.Weather, {
            method: 'POST',
            body: { ip },
        })
    } catch (err: unknown) {
        console.error('error fetching weather info: ', err)
    }
}

onMounted(() => {
    getWeather()
})
</script>

<template>
    <div class="text-center mb-5">
        <p>Weather in {{ weatherReport.city }}: <span class="green">{{ weatherReport.temp }}</span></p>
    </div>
</template>

<style scoped></style>
