<script setup lang="ts">
import { createHttpClient } from '@/services/HttpClient'
import { AxiosAdapter } from '@/services/AxiosAdapter'
import { onMounted } from 'vue'
import { API_ROUTES } from '@/constants'
import { ref } from 'vue'

const http = createHttpClient(AxiosAdapter)

interface AutoShoutChatter {
  id: int
  channelname: string
  chattername: string
  shout_count: int
  created_at: time
  updated_at: time
}

let autoShoutChatters = ref<AutoShoutChatter[]>({})

async function getChatters() {
  try {
    const resp = await http.get<string>(API_ROUTES.GetAutoShoutChatters)
    if (!resp) {
      console.error('error fetching auto shout chatters')
      return;
    }

    autoShoutChatters.value = resp.data
  }
}

async function removeChatter(id: int) {
  try {
    const resp = await http.delete<string>(API_ROUTES.DeleteAutoShoutChatter(id))
    if (!resp) {
      console.error('error deleting auto shout chatter')
      return;
    }
  } catch (err: unknown) {
    console.error("error fetching auto shout chatters: ", err)
  }
}

onMounted(() => {
  getChatters()
})
</script>

<template>
  <p>Auto Shoutout Chatters</p>
  <ul class="list-group">
    <li v-for="chatter in autoShoutChatters" class="list-group-item">
      <p>{{ chatter.chattername }} shouts: <span>{{ chatter.shout_count }}</span></p>
      <button @click="removeChatter(chatter.id)">Remove</button>
    </li>
  </ul>
</template>

<style scoped>

</style>