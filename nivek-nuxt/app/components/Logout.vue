<script setup lang="ts">
const auth = useAuthStore()

async function doLogout() {
    auth.logout()
    await navigateTo('/')
}
</script>

<template>
    <div class="logout-button">
        <button @click="doLogout" class="btn btn-secondary" type="button">Log out</button>
    </div>
</template>

<style scoped>
.logout-button {
    position: absolute;
    top: 2rem;
    right: 2rem;
}
</style>
