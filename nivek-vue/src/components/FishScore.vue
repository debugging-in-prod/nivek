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
  try {
    const resp = await http.get<string>(API_ROUTES.Secure.GetFishScore)
    if (!resp) {
      console.error('error fetching fish score')
      return;
    }

    fishScores.value = resp.data
  } catch (err: unknown) {
    console.error("error fetching fish score: ", err)
  }
}

// Group fish by name
function groupFishByName(fish: Fish[]) {
  const grouped: Record<string, { count: number; value: number; scarcity: number }> = {}

  fish.forEach(f => {
    if (!grouped[f.name]) {
      grouped[f.name] = {
        count: 0,
        value: f.value,
        scarcity: f.scarcity
      }
    }
    grouped[f.name].count++
  })

  return grouped
}

onMounted(() => {
  getFishScore()
})
</script>

<template>
  <div v-for="fishScore in fishScores" class="text-center mb-5">
    <pre>{{ fishScore }}</pre>
    <h3>Your Fish Score for <span>{{ fishScore.chatterName }}</span></h3>
    <p>Total Score: <span>{{ fishScore.score }}</span></p>
    <h4>Fish Caught</h4>
    <ul class="list-group">
      <li v-for="(groupedFish, name) in groupFishByName(fishScore.fish)"
          :key="name"
          class="list-group-item">
        <p>🐟 <span>{{ name }}</span> × {{ groupedFish.count }} - <span>{{ groupedFish.value }}</span> points each</p>
        <p>Rarity: {{ groupedFish.scarcity }}</p>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.list-group-item {
  background: unset;
  color: unset;
}
</style>