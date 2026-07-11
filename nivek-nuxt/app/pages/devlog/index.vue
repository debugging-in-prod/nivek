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
            </div>
        </header>

        <div class="layout">
            <div>
                <div>
                    <header>July 11, 2026</header>
                    <p>Hello! Its been a while since I've done this. Today's work involves a large overhaul of the bot's architecture. The main goal for work today is to eliminate the API call between the bot recieving a command, and procsssing it. Prior to this work, the Bot lived on a raspberry pi in my living room, but would reach out to an API that I have running on a remote VPS. This was done because I wanted to involved a database in this project to allow for higher level logic like comparing command scores from one chat to another. I also wanted my users to be able to view this information, and they were not going to be able to do that if I self-hosted the database. I have no interest in opening up my home network to internet scanners looking for vulnerabilities to exploit, so I ended up with this home infra/vps infra split. The result was a poor UX.</p>
                    <p>Prior to work today, I already moved the bot to the VPS. So now the work is just migrating systems around within the repo itself</p>
                </div>
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

