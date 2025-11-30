<script setup lang="ts">

const http = createHttpClient(AxiosAdapter)

interface FishScore {

}

let fishScore = ref<FishScore>({})

async function getFishScore() {
  try {
    const resp = await http.get<string>(API_ROUTES.GetFishScore)
    if (!resp) {
      console.error('error fetching fish score')
      return;
    }

    fishScore.value = resp.data
  } catch (err: unknown) {
    console.error("error fetching fish score: ", err)
  }
}

onMounted(() => {
  getFishScore()
})
</script>

<template>
  <div class="text-center mb-5">
    <h3>Your Fish Score</h3>
    <h4>Fish Caught</h4>
    <ul>
      <li ></li>
    </ul>
  </div>
</template>

<style scoped>

</style>