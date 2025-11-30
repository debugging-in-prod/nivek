<script setup lang="ts">
import { createHttpClient } from '@/services/HttpClient'
import { AxiosAdapter } from '@/services/AxiosAdapter'
import { onMounted } from 'vue'
import { API_ROUTES } from '@/constants'
import { ref } from 'vue'

const http = createHttpClient(AxiosAdapter)

interface FishScore {
  id: int
  channelname: string
  chattername: string
  score:       int
  fish:        FishArray
  trash_caught: int
  times_fished: int
  created_at:   time
  updated_at:   time
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
  if (!fish?.length) return []

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

const totalFishCaught = (fishArray: Fish[]) => {
  if (!fishArray?.length) return 0
  return fishArray.reduce((sum, f) => sum + 1, 0)
}

const rarityBadgeClass = (rarity) => {
  const classes = {
    'Common': 'bg-secondary',
    'Uncommon': 'bg-info',
    'Rare': 'bg-primary',
    'Epic': 'bg-warning text-dark',
    'Legendary': 'bg-danger',
    'Mythic': 'bg-dark text-white'
  }
  return ['badge', classes[rarity] || 'bg-light text-dark']
}

onMounted(() => {
  getFishScore()
})
</script>

<template>
  <h3>Fish Scores!</h3>
  <h4>As Chatter: </h4>
  <div v-for="fishScore in fishScores.as_chatter" class="mb-5">
    <!-- Header Card -->
    <div class="card shadow-sm mb-4">
      <div class="card-body text-center bg-primary text-white">
        <div class="d-flex justify-content-between align-items-center">
          <div class="text-start">
            <h5 class="card-title mb-0">
              <strong>{{ fishScore.chattername }}</strong>
              <small class="d-block fs-8 mt-1">in #{{ fishScore.channelname }}</small>
            </h5>
          </div>
          <div>
            <p class="fw-bold mb-0">
              Score: {{ fishScore.score }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <div class="table-responsive">
      <table class="table table-hover align-middle">
        <thead class="table-light">
        <tr>
          <th>Fish</th>
          <th class="text-center">Qty</th>
          <th class="text-center">Value</th>
          <th class="text-center">Total Points</th>
          <th>Rarity</th>
        </tr>
        </thead>
        <tbody>
        <tr v-for="(groupedFish, name) in groupFishByName(fishScore.fish)" :key="name">
          <td>
            <strong>{{ name }}</strong>
          </td>
          <td class="text-center fw-bold">
            <span class="badge bg-success fs-6">×{{ groupedFish.count }}</span>
          </td>
          <td class="text-center">{{ groupedFish.value }} pts</td>
          <td class="text-center fw-bold text-primary">
            {{ (groupedFish.count * groupedFish.value) }}
          </td>
          <td>
              <span :class="rarityBadgeClass(groupedFish.scarcity)">
                {{ groupedFish.scarcity }}
              </span>
          </td>
        </tr>
        <tr v-if="Object.keys(groupFishByName(fishScore.fish)).length === 0">
          <td colspan="5" class="text-center text-muted py-4">
            No fish caught yet. Time to cast a line!
          </td>
        </tr>
        </tbody>
        <tfoot class="table-secondary">
        <tr>
          <th colspan="3">Grand Total</th>
          <th class="text-center text-primary fw-bold">
            {{ fishScore.score }}
          </th>
          <th></th>
        </tr>
        </tfoot>
      </table>
    </div>
  </div>
  <h4>As Channel: </h4>
  <div v-for="fishScore in fishScores.as_channel" class="mb-5">
    <!-- Header Card -->
    <div class="card shadow-sm mb-4">
      <div class="card-body text-center bg-primary text-white">
        <div class="d-flex justify-content-between align-items-center">
          <div class="text-start">
            <h5 class="card-title mb-0">
              <strong>{{ fishScore.chattername }}</strong>
              <small class="d-block fs-8 mt-1">in #{{ fishScore.channelname }}</small>
            </h5>
          </div>
          <div>
            <p class="fw-bold mb-0">
              Score: {{ fishScore.score }}
            </p>
          </div>
        </div>
      </div>
    </div>

    <div class="table-responsive">
      <table class="table table-hover align-middle">
        <thead class="table-light">
        <tr>
          <th>Fish</th>
          <th class="text-center">Qty</th>
          <th class="text-center">Value</th>
          <th class="text-center">Total Points</th>
          <th>Rarity</th>
        </tr>
        </thead>
        <tbody>
        <tr v-for="(groupedFish, name) in groupFishByName(fishScore.fish)" :key="name">
          <td>
            <strong>{{ name }}</strong>
          </td>
          <td class="text-center fw-bold">
            <span class="badge bg-success fs-6">×{{ groupedFish.count }}</span>
          </td>
          <td class="text-center">{{ groupedFish.value }} pts</td>
          <td class="text-center fw-bold text-primary">
            {{ (groupedFish.count * groupedFish.value) }}
          </td>
          <td>
              <span :class="rarityBadgeClass(groupedFish.scarcity)">
                {{ groupedFish.scarcity }}
              </span>
          </td>
        </tr>
        <tr v-if="Object.keys(groupFishByName(fishScore.fish)).length === 0">
          <td colspan="5" class="text-center text-muted py-4">
            No fish caught yet. Time to cast a line!
          </td>
        </tr>
        </tbody>
        <tfoot class="table-secondary">
        <tr>
          <th colspan="3">Grand Total</th>
          <th class="text-center text-primary fw-bold">
            {{ fishScore.score }}
          </th>
          <th></th>
        </tr>
        </tfoot>
      </table>
    </div>
  </div>
</template>

<style scoped>
.bi-fish::before {
  font-weight: 900 !important;
}
.list-group-item {
  background: unset;
  color: unset;
}
</style>