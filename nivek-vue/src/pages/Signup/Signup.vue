<script setup lang="ts">
import { reactive } from 'vue'
import { createHttpClient } from '@/services/HttpClient'
import { AxiosAdapter } from '@/services/AxiosAdapter'
import { User, API_ROUTES } from '@/constants'
import {useRouter} from "vue-router";

const router = useRouter()

interface SignupFormData {
  username: string
  email:    string
  password: string
}

const http = createHttpClient(AxiosAdapter)

const formData = reactive(<SignupFormData>{
  username: '',
  email: '',
  password: '',
})

async function doSignup() {
  try {
    const success = await http.post<User[]>(API_ROUTES.Signup, formData)
    if (success) {
      await router.push("/login")
    } else {
      console.warn("signup failed! try a different username")
    }
  } catch (err: unknown) {
    console.error('error signing up: ', err)
  }
}
</script>

<template>
  <div class="form-signup m-auto">
    <h1 class="green mb-4">Sign Up</h1>
    <p>Right now the only function of this website is to allow you to control a twitch chatbot known as @peanutbudderbot.</p>
    <p>If you sign up with your <b>twitch username</b> as your <b>'username'</b> here, then the bot will join your chat next time its rebooted (dm me to get this done faster)</p>
    <form class="pb-3" @submit.prevent="doSignup">
      <div class="form-group mb-1">
        <label for="usernameInput">Username</label>
        <input type="text"
               id="usernameInput"
               class="form-control"
               aria-describedby="usernameHelp"
               placeholder="Choose a Username"
               v-model="formData.username"
               required
        >
        <small id="usernameHelp" class="form-text text-secondary">We'll share your username with everyone</small>
      </div>
      <div class="form-group mb-1">
        <label for="exampleInputEmail1">Email</label>
        <input type="email"
               id="exampleInputEmail1"
               class="form-control"
               aria-describedby="emailHelp"
               placeholder="Enter email"
               v-model="formData.email"
               required
        >
        <small id="emailHelp" class="form-text text-secondary">We'll never share your email with anyone else.</small>
      </div>
      <div class="form-group mb-4">
        <label for="exampleInputPassword1">Password</label>
        <input type="password"
               class="form-control"
               id="exampleInputPassword1"
               placeholder="Password"
               v-model="formData.password"
               required
        >
      </div>
      <button type="submit" class="btn btn-primary">Submit</button>
    </form>    
    <p>
      Your email and password are safe. Your 'email' here does not need to be a real email, it just needs to look like an email. 
      I recommend doing [your-twitch-username]@nivek.life. At some point in the future, I plan on updating this to use real email verification. 
    </p>
    <p>
      Do not be concerned about losing access to your account or data. This <b class="text-decoration-underline">will</b> happen. 
      It is a question of when, not if. Just don't be concerned
    </p>
    <p>
      This bot is in very early stages of active development. Expect bugs, errors, failures, letdowns, mischeif, mayhem
      disappointments, heartbreaks, debauchery, trickery, and general disruptive behavior
    </p>
  </div>
</template>

<style scoped>
.form-signup {
  max-width: 420px;
}
</style>