<script setup lang="ts">
import { API_ROUTES } from '~/utils/constants'
import { errorMessage } from '~/utils/errorMessage'
import type { AutoShoutChatter } from '~/types/autoshout'

const autoShoutChatters = ref<AutoShoutChatter[]>([])
const chattername = ref('')
const loading = ref(true)
const error = ref<string | null>(null)

// Track which chatter is awaiting delete confirmation.
const confirmingDelete = reactive<Record<number, boolean>>({})

async function getChatters() {
  loading.value = true
  error.value = null
  try {
    autoShoutChatters.value = await api<AutoShoutChatter[]>(API_ROUTES.Secure.GetAutoShoutChatters)
    autoShoutChatters.value.forEach((c) => (confirmingDelete[c.id] = false))
  } catch (err: unknown) {
    console.error('error fetching auto shout chatters: ', err)
    error.value = errorMessage(err, "Couldn't load your AutoShout chatters.")
  } finally {
    loading.value = false
  }
}

async function addNewChatter() {
  error.value = null
  try {
    await api(API_ROUTES.Secure.PostCreateAutoShoutChatter, {
      method: 'POST',
      body: { chattername: chattername.value },
    })
    await getChatters()
    chattername.value = ''
  } catch (err: unknown) {
    console.error('error creating auto shout chatter: ', err)
    error.value = errorMessage(err, "Couldn't add the chatter.")
  }
}

async function removeChatter(id: number) {
  error.value = null
  try {
    await api(API_ROUTES.Secure.DeleteAutoShoutChatter(id), {
      method: 'DELETE',
    })
    await getChatters()
  } catch (err: unknown) {
    console.error('error deleting auto shout chatter: ', err)
    error.value = errorMessage(err, "Couldn't remove the chatter.")
  }
}

onMounted(() => {
  getChatters()
})
</script>

<template>
  <h4 class="title">AutoShout</h4>
  <div>
    <p class="mb-2"
      >These are chatters that will get automatic shoutouts for every 1st message they post in your
      chat when you go live</p
    >
    <form @submit.prevent="addNewChatter()" class="mb-3 py-3">
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
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p v-if="loading">Loading…</p>
    <ul v-else class="auto-shout-list list-group">
      <li
        v-for="chatter in autoShoutChatters"
        :key="chatter.id"
        class="list-group-item d-flex justify-content-between align-items-start"
      >
        <div>
          <span>{{ chatter.chattername }}</span>
          <div
            >Shouts: <span>{{ chatter.shout_count }}</span></div
          >
        </div>
        <div class="text-end">
          <div v-if="!confirmingDelete[chatter.id]">
            <button @click="confirmingDelete[chatter.id] = true" class="btn btn-sm btn-danger">
              Remove
            </button>
          </div>
          <div v-else>
            <button @click="removeChatter(chatter.id)" class="btn btn-sm btn-success"> YES </button>
            <button @click="confirmingDelete[chatter.id] = false" class="btn btn-sm btn-secondary">
              NO
            </button>
          </div>
        </div>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.title {
  border-bottom: 2px solid grey;
}
.auto-shout-list {
  max-height: 600px;
  overflow-y: scroll;
}
.auto-shout-list li {
  background: inherit;
  border-top: 0;
  border-left: 0;
  border-right: 0;
  border-color: var(--color-text);
  color: inherit;
}
.hidden {
  display: none !important;
}
.form-control {
  background-color: unset;
  border: 0;
  color: unset;
}
input.form-control::placeholder {
  color: unset;
  font-style: italic;
  opacity: 0.6;
}
.btn.btn-primary {
  background-color: transparent;
  border: 1px solid gray;
  color: inherit;
}
form {
  border-top: 2px solid gray;
  border-bottom: 2px solid gray;
}
.error {
  color: #e5484d;
}
</style>
