<script setup lang="ts">
import { createHttpClient } from '@/services/HttpClient'
import { AxiosAdapter } from '@/services/AxiosAdapter'
import { onMounted, ref } from 'vue'
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

let autoShoutChatters = ref<AutoShoutChatter[]>([])
let chattername = ref('')

async function getChatters() {
  try {
    const resp = await http.get<string>(API_ROUTES.Secure.GetAutoShoutChatters)
    if (!resp) {
      console.error('error fetching auto shout chatters')
      return;
    }

    autoShoutChatters.value = resp.data
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
    const resp = await http.delete(API_ROUTES.Secure.DeleteAutoShoutChatter(id))
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
  <h2>Auto Shoutout Chatters</h2>
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
  <ul class="list-group">
    <li v-for="chatter in autoShoutChatters" :key="chatter.id" class="list-group-item">
      <p>{{ chatter.chattername }} shouts: <span>{{ chatter.shout_count }}</span></p>
      <button @click="removeChatter(chatter.id)">Remove</button>
    </li>
  </ul>
</template>

<style scoped>

</style>