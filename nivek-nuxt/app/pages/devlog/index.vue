<script setup lang="ts">

onMounted(() => {
    console.log('hi mom')
})

</script>

<template>
    <div>
        <header>
            <div class="header-row">
                <h1>Devlog</h1>
                <div class="nav-group">
                    <NuxtLink to="/df/citizens" class="nav-btn">Citizens</NuxtLink>
                    <NuxtLink to="/df/help" class="nav-btn">Help</NuxtLink>
                </div>
            </div>
        </header>

        <div class="layout">
            <div>
                Main devlog here
            </div>
            <aside class="hud">
                <h3>Aside!</h3>
            </aside>
        </div>
   </div>
</template>

<style scoped>
.header-row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 1rem;
    flex-wrap: wrap;
}

header h1 {
    margin: 0 0 0.25rem 0;
    color: #6fb;
}

.nav-group {
    display: flex;
    gap: 0.5rem;
}

/* Themed button-link: dark surface, green accent matching .hud headings. */
.nav-btn {
    display: inline-block;
    background: #222;
    color: #6fb;
    border: 1px solid #3a6;
    border-radius: 4px;
    padding: 0.35rem 0.9rem;
    font-family: monospace;
    font-size: 0.9rem;
    text-decoration: none;
    white-space: nowrap;
}
.nav-btn:hover {
    background: #2a3a30;
    border-color: #6fb;
}

.layout {
    display: flex;
    gap: 1.5rem;
    align-items: flex-start;
    flex-wrap: wrap;
}

</style>

