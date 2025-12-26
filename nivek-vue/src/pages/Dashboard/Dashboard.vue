<script setup lang="ts">
import { ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import Weather from '@/components/Weather.vue'
import AutoShout from "@/components/AutoShout.vue";
import FishScore from "@/components/FishScore.vue";
import AnonMessager from '@/components/AnonMessager.vue';

const auth = useAuthStore()

function getGreeting(date: Date = new Date()): string {
  const hour = date.getHours()
  if (hour >= 12 && hour < 18) {
    return "Good Afternoon"
  } else if (hour >= 18 || hour < 5) {
    return "Good Evening"
  } else {
    return "Good Morning"
  }
}

let hideAutoShout = ref(true)
let hideFishing = ref(true)
</script>

<template>
  <div>
    <h1 v-if="auth.user" class="text-center green">{{ getGreeting() }} {{ auth.user?.username }}</h1>
    <Weather />
  </div>

  <div class="container">
    <div class="row">
      <div class="col-md-2">
        <ul class="command-config-nav">
          <li @click="hideAutoShout = !hideAutoShout">AutoShout</li>
          <li @click="hideFishing = !hideFishing">Fishing</li>
        </ul>
      </div>

      <div class="col-md-8 pt-1 pb-5">
        <p :class="{ hidden: (!hideAutoShout || !hideFishing) }">Select a command on the left</p>
        <div :class="{ hidden: hideAutoShout }"><AutoShout /></div>
        <div :class="{ hidden: hideFishing }"div><FishScore /></div>
      </div>
      <div class="col-md-2">
        <AnonMessager />
      </div>
    </div>
  </div>
</template>

<style scoped>
.hidden { 
  display: none !important;
}
.container {
  border: 2px solid grey;
  border-radius: 5px;
}
.container .row > *:not(:last-child) {
  border-right: 2px solid grey;
}
.container .row {
  min-height: 500px;
}

.command-config-nav {
  list-style: none;
  margin: 0;
  padding: 0;
}
.command-config-nav > *:hover {
  cursor: pointer;
}
.command-config-nav > *:not(:last-child) {
  border-bottom: 2px solid grey;
}
</style>