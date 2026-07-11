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
                    <p>Hello! Its been a while since I've done this. Today's work involves a review of the bot's architecture. The main idea is to look into eliminating API calls from the bot when processing a chat command. Prior to this work, the Bot lived on a raspberry pi in my living room, but would reach out to an API that is running on a remote VPS. This was done because I wanted to involved a database in this project to allow for higher level logic like comparing command scores from one chat to another. I also wanted my users to be able to view this information, and they were not going to be able to do that if I self-hosted the database. I have no interest in opening up my home network to internet scanners looking for vulnerabilities to exploit, so I ended up with this home infra/vps infra split. The result was a poor UX.</p>
                    <p>Prior to work today, I already moved the bot to the VPS. So now the work is just migrating systems around within the repo itself</p>
                    <p>After reviewing, it appears that having the bot run on the same VPS that the API and DB live on largely eliminates most of the latency between command and response, and it also appears that I would not be able to eliminate API calls altogether unless I exposed the bot to the DB directly, which I do not want to do.</p>
                    <p>So now, the work becomes subscribing to webhooks. Currently the bot joins every channel on startup, and users have no opt-in or opt-out once they log in to the website. So regardless of if the users are live or not, the bot is maintaining an active connection to their channel. In theory, if 500,000 people were to sign up for this bot, that becomes 500,000 active connections to manage 24/7 when really only probably ~10% of those users would be streaming at any given time. Twitch supplies webhooks for go-live and go-offline events, so I just need to get the bot to listen for these and respond accordingly</p>
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

