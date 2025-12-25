<script setup lang="ts">
import { createHttpClient } from '@/services/HttpClient'
import { AxiosAdapter } from '@/services/AxiosAdapter'
import { onMounted, ref, reactive } from 'vue'
import { API_ROUTES } from '@/constants'

const http = createHttpClient(AxiosAdapter)

interface AutoShoutChatter {
  id: number
  channelname: string
  chattername: string
  shout_count: number
  created_at: string
  updated_at: string
}

// @TODO::add ci/cd for vue
// @TODO::identify which files need rebuild and which just need recreate
// other than this it's just nginx running, but that runs off an official image with very basic config
// so no big need for a pipeline there
// and PSQL does the same & uses a mounted volume for data persistence
// still, it would feel strange if I need to manually pull after a hypothetical 3-9 years goes by without an update
// and after automated pipelines have managed the other two services for those 3-9 years
// But if those other pipelines do any amount of git pull, and the nginx.conf is tracked by that
// then all I'd need is a manual reboot instead of full manual deploy
// neither of these are a big deal
// but in the event that you forget these steps, it becomes a big deal

// list of chatters
let autoShoutChatters = ref<AutoShoutChatter[]>([])
let chattername = ref('')

let displayList = ref(false)

// track which chatter is awaiting delete confirmation
let confirmingDelete = reactive<Record<number, boolean>>({})

async function getChatters() {
  try {
    const resp = await http.get<string>(API_ROUTES.Secure.GetAutoShoutChatters)
    if (!resp) {
      console.error('error fetching auto shout chatters')
      return;
    }

    autoShoutChatters.value = resp.data
    // reset confirmation states
    autoShoutChatters.value.forEach(c => confirmingDelete[c.id] = false)
  } catch (err: unknown) {
    console.error("error fetching auto shout chatters: ", err)
  }
}

async function addNewChatter() {
  try {
    const resp = await http.post(API_ROUTES.Secure.PostCreateAutoShoutChatter, {
      chattername: chattername.value
    })
    if (!resp) {
      console.error('error creating auto shout chatter')
      return;
    }

    // Refresh the list and clear input
    await getChatters()
    chattername.value = ''
  } catch (err: unknown) {
    console.error("error creating auto shout chatter: ", err)
  }
}

async function removeChatter(id: number) {
  try {
    const resp = await http.del(API_ROUTES.Secure.DeleteAutoShoutChatter(id))
    if (!resp) {
      console.error('error deleting auto shout chatter')
      return;
    }

    // Refresh the list after deletion
    await getChatters()
  } catch (err: unknown) {
    console.error("error deleting auto shout chatter: ", err)
  }
}

onMounted(() => {
  getChatters()
})
</script>

<template>
  <h2>AutoShout</h2>
  <button class="btn btn-primary" @click="displayList = !displayList">
    <span>Click here to {{ displayList ? 'hide' : 'show'}}</span>
  </button>
  <div :class="[{ hidden: !displayList }]">
    <p>These are chatters that will get automatic shoutouts for every 1st message they post in your chat when you go live</p>
    <form @submit.prevent="addNewChatter()" class="mb-3">
      <div class="form-group">
        <label for="chattername">Chatter Name</label>
        <input
            type="text"
            class="form-control"
            id="chattername"
            v-model="chattername"
            placeholder="Enter chatter name"
            required
        />
      </div>
      <button type="submit" class="btn btn-primary mt-2">Add Chatter</button>
    </form>
    <ul class="auto-shout-list list-group">
      <li v-for="chatter in autoShoutChatters" 
        :key="chatter.id" 
        class="list-group-item d-flex justify-content-between align-items-start"
      >
        <div>{{ chatter.chattername }}</div>
        <div class="text-end">
          <div>Shouts: <span>{{ chatter.shout_count }}</span></div>

          <!-- Conditional rendering for delete confirmation -->
          <div v-if="!confirmingDelete[chatter.id]">
            <button @click="confirmingDelete[chatter.id] = true" class="btn btn-sm btn-danger mt-1 mb-2">
              Remove
            </button>
          </div>
          <div v-else>
            <button @click="removeChatter(chatter.id)" class="btn btn-sm btn-success mt-1 mb-2 me-1">
              YES
            </button>
            <button @click="confirmingDelete[chatter.id] = false" class="btn btn-sm btn-secondary mt-1 mb-2">
              NO
            </button>
          </div>
        </div>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.auto-shout-list li {
  background: inherit;
  border-color: var(--color-text);
  color: inherit;
}
.hidden {
  display: none !important;
}
</style>
