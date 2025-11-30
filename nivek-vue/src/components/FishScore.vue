<script setup lang="ts">
import { createHttpClient } from '@/services/HttpClient'
import { AxiosAdapter } from '@/services/AxiosAdapter'
import { onMounted } from 'vue'
import { API_ROUTES } from '@/constants'
import { ref } from 'vue'

const http = createHttpClient(AxiosAdapter)

interface FishScore {
  id: int
  channelName: string
  chatterName: string
  score:       int
  fish:        FishArray
  trashCaught: int
  timesFished: int
  createdAt:   time
  updatedAt:   time
}

interface Fish {
  value:    int
  name:     string
  scarcity: int
}

let fishScores = ref<FishScore[]>({})

async function getFishScore() {
  console.log('getFishScore running')
  try {
    const resp = await http.get<string>(API_ROUTES.GetFishScore)
    if (!resp) {
      console.error('error fetching fish score')
      return;
    }

    fishScores.value = resp.data
  } catch (err: unknown) {
    console.error("error fetching fish score: ", err)
  }
}

onMounted(() => {
  getFishScore()
})
</script>

<template>
  <div v-for="fishScore in fishScores" class="text-center mb-5">
    <h3>Your Fish Score for <span>{{ fishScore.chatterName }}</span></h3>
    <p>Total Score: <span>{{ fishScore.score }}</span></p>
    <h4>Fish Caught</h4>
    <ul>
      <li v-for="fish in fishScore.fish">
        <p>{{ fish.name }}</p>
        <p>{{ fish.value }}</p>
        <p>{{ fish.scarcity }}</p>
      </li>
    </ul>
  </div>
</template>

<style scoped>

</style>