<script setup lang="ts">
import { createHttpClient } from '@/services/HttpClient'
import { AxiosAdapter } from '@/services/AxiosAdapter'
import { onMounted, ref } from 'vue'
import { API_ROUTES } from '@/constants'

const http = createHttpClient(AxiosAdapter)

interface FishScore {
  id: number
  channelname: string
  chattername: string
  score:       number
  fish:        Fish[]
  trash_caught: number
  times_fished: number
  created_at:   string
  updated_at:   string
}

interface Fish {
  value:    number
  name:     string
  scarcity: number
}

const expandedRows = ref<Record<string | number, boolean>>({})

function toggleRow(id: string | number) {
  expandedRows.value[id] = !expandedRows.value[id]
}

let fishScores = ref<{ as_channel: FishScore[]; as_chatter: FishScore[] }>({
  as_channel: [],
  as_chatter: []
})

async function getFishScore() {
  try {
    const resp = await http.get(API_ROUTES.Secure.GetFishScore)
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

onMounted(() => {
  getFishScore()
})
</script>

<!-- @TODO::template this better -->
<template>
  <h4 class="title pb-1 mb-4">Fishing</h4>
  
  <!-- Loop over both As Channel and As Chatter -->
  <div v-for="(fishScoresGroup, key) in fishScores" :key="key"
    class="fishermen mb-4"
  >
    <h5 class="mb-2">{{ key === 'as_channel' ? 'Fishermen in your chat 🎣' : 'You fishing in chats 🎣🐟' }}</h5>

    <div v-for="fishScore in fishScoresGroup" :key="fishScore.id"
      class="card-container"
    >
      <!-- Header Card -->
      <div
        class="fisher-card shadow-sm"
        role="button"
        @click="toggleRow(fishScore.id)"
      >
        <div class="card-body text-center">
          <div class="d-flex justify-content-between align-items-center">
            <div class="text-start">
              <p class="card-title h6 mb-0">
                <strong>{{ fishScore.chattername }}</strong>
                <small class="ps-2 fs-8 mt-1">Fishing in <span class="green">#{{ fishScore.channelname }}'s</span> chat</small>
              </p>
            </div>
            <div>
              <p class="fw-bold mb-0">
                Score: {{ fishScore.score }}
              </p>
            </div>
          </div>
        </div>
      </div>

      <div
        class="table-responsive"
        v-show="expandedRows[fishScore.id]"
      >
        <table class="table table-hover align-middle fisher-card-content">
          <thead>
            <tr>
              <th><strong>Fish</strong></th>
              <th class="text-center"><strong>Qty</strong></th>
              <th class="text-center"><strong>Value</strong></th>
              <th class="text-center"><strong>Total Points</strong></th>
              <th><strong>Rarity</strong></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(groupedFish, name) in groupFishByName(fishScore.fish)" :key="name">
              <td>{{ name }}</td>
              <td class="text-center fw-bold">
                <span class="badge bg-success fs-6">×{{ groupedFish.count }}</span>
              </td>
              <td class="text-center">{{ groupedFish.value }} pts</td>
              <td class="text-center fw-bold text-primary">
                {{ (groupedFish.count * groupedFish.value) }}
              </td>
              <td>
                <span>
                  {{ groupedFish.scarcity }}
                </span>
              </td>
            </tr>
            <tr v-if="Object.keys(groupFishByName(fishScore.fish)).length === 0">
              <td colspan="5" class="text-center py-4">
                No fish caught yet. Time to cast a line!
              </td>
            </tr>
          </tbody>
          <tfoot>
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
  </div>
</template>

<style scoped>
.title {
  border-bottom: 2px solid grey;
}
.fisher-card:hover {
  background-color: rgb(100, 100, 100);
  cursor: pointer;
}
.bi-fish::before {
  font-weight: 900 !important;
}
.list-group-item {
  background: unset;
  color: unset;
}
.hidden {
  display: none !important;
}

.fishermen .fisher-card-content.table {
  --bs-table-bg: unset;
  --bs-table-color: unset;
}
.table-hover>tbody>tr:hover>* {
  --bs-table-color-state: unset;
}
</style>