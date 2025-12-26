<script setup lang="ts">
import { createHttpClient } from '@/services/HttpClient'
import { AxiosAdapter } from '@/services/AxiosAdapter'
import { onMounted, ref } from 'vue'
import { API_ROUTES } from '@/constants'

const http = createHttpClient(AxiosAdapter)

interface Message {
    sender:     string
    message:    string
    created_at: string
    updated_at: string
}
let messages = ref<Message[]>([])
let newMessage = ref<Message>([])

// for later...
// API_ROUTES.Secure.PostCreateMessage
// API_ROUTES.Secure.GetMessages
async function createMessage() {
  try {
    const resp = await http.post(API_ROUTES.Secure.PostCreateMessage, {
        sender: newMessage.value.sender,
        message: newMessage.value.message
    })
    if (!resp) {
      console.error('error creating message')
      return;
    }

    // Refresh the list and clear input
    await getMessages()
  } catch (err: unknown) {
    console.error("error creating message: ", err)
  }
}

async function getMessages() {
  try {
    const resp = await http.get<string>(API_ROUTES.Secure.GetMessages)
    if (!resp) {
      console.error('error fetching auto shout chatters')
      return;
    }

    messages.value = resp.data
  } catch (err: unknown) {
    console.error("error fetching auto shout chatters: ", err)
  }
}

onMounted(() => {
  getMessages()
})

let displayComponent = ref(true)
let displayNewMessage = ref(false)
let displayMessageList = ref(true)
</script>

<template>
    <div class="messenger">
        <div class="head clickme" @click="displayComponent = !displayComponent">
            <span class="pe-2">Messages</span>
            <span :class="['triangle', { open: displayComponent }]">&#9654;</span>
        </div>
        <div :class="['body', { hidden: !displayComponent }]">
            <div class="new-message-form-container">
                <p class="small clickme" @click="displayNewMessage = !displayNewMessage">
                    Write a message...<span :class="['triangle ps-2', { open: displayNewMessage }]">&#9654;</span>
                </p>
                <form :class="['new-message pb-2', { hidden: !displayNewMessage }]" @submit.prevent="createMessage">
                    <div><input type="text" name="name" 
                        placeholder="Your name here"
                        v-model="newMessage.sender"
                    ></div>
                    <div><textarea type="text" name="message" 
                        placeholder="Your message here"
                        v-model="newMessage.message"
                    ></textarea></div>
                    <button type="submit">Send</button>
                </form>
            </div>
            <div class="message-list-container">
                <p class="small clickme" @click="displayMessageList = !displayMessageList">
                    Read some messages<span :class="['triangle ps-2', { open: displayMessageList }]">&#9654;</span>
                </p>
                <ol :class="['message-list', { hidden: !displayMessageList }]">
                    <li v-for="message in messages">
                        <div class="d-flex justify-content-between small">
                            <span class="text-secondary">ッ⃝<strong>{{ message.sender }}</strong></span>
                            <span class="text-secondary">{{ message.created_at }}</span>
                        </div>
                        <p class="m-0">{{  message.message }}</p>
                    </li>
                </ol>
            </div>
        </div>
    </div>
</template>

<style scoped>
.clickme:hover {
    cursor: pointer;
}
.messenger .hidden {
    display: none !important;
}
.messenger .triangle {
    display: inline-block;
}
.messenger .triangle.open {
    transform: rotate(90deg);
}
.messenger {
    display: inline-block;
    max-width: 100%;
    min-width: 100%;
    overflow: hidden;
    padding: 0;
}
.messenger .head {
    border-bottom: 2px solid gray;
}
.messenger .new-message-form-container {
    border-bottom: 2px solid gray;
}
.messenger .new-message-form-container > p {
    margin-bottom: 0;
}
.messenger .new-message * {
    background: transparent;
    border: 0;
}
.messenger .new-message input,
.messenger .new-message textarea {
    width: 100%;
}
.messenger .new-message > *:not(:last-child) > * {
    border-bottom: 2px solid gray;
    color: gray;
}
.messenger .new-message > *:last-child {
    background: darkgray;
    border-radius: 30px;
    font-style: italic;
}

.messenger .message-list {
    border-top: 2px solid gray;
    list-style: none;
    margin: 10px 0 0;
    max-height: 450px;
    overflow-y: scroll;
    padding: 0;
}
.messenger .message-list::-webkit-scrollbar {
    display: none;
}
.messenger .message-list > *:not(:last-child) {
    border-bottom: 2px solid gray;
}
</style>