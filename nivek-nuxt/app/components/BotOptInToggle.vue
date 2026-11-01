<script setup lang="ts">
import { API_ROUTES } from '~/utils/constants'

// Opt the bot in/out of the logged-in broadcaster's chat. The initial state
// comes from the profile (bot_opt_in, read fresh from the DB), so a prior
// !banish shows as "off" and can be reversed here.
const auth = useAuthStore()
const saving = ref(false)
const error = ref<string | null>(null)

const optedIn = computed(() => auth.user?.bot_opt_in ?? false)

async function toggle(event: Event) {
  // Keep the checkbox driven by saved state: snap it back until the server
  // confirms, so a failed request never leaves it showing the wrong value.
  ;(event.target as HTMLInputElement).checked = optedIn.value
  if (!auth.user || saving.value) return
  const next = !optedIn.value
  saving.value = true
  error.value = null
  try {
    const res = await api<{ opted_in: boolean }>(API_ROUTES.Secure.PutBotOptIn, {
      method: 'PUT',
      body: { opted_in: next },
    })
    auth.setBotOptIn(res.opted_in)
  } catch (err: unknown) {
    console.error('error updating bot opt-in: ', err)
    error.value = 'Could not update the bot setting. Please try again.'
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="optin">
    <label class="switch-label" for="botOptIn">
      <input
        id="botOptIn"
        type="checkbox"
        role="switch"
        :checked="optedIn"
        :aria-checked="optedIn"
        :disabled="saving || !auth.user"
        @change="toggle"
      />
      <span>Bot in my chat: <strong>{{ optedIn ? 'On' : 'Off' }}</strong></span>
    </label>
    <p v-if="error" class="error">{{ error }}</p>
  </div>
</template>

<style scoped>
.optin {
  margin-top: 0.5rem;
}
.switch-label {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
}
.switch-label input {
  width: 2.25rem;
  height: 1.1rem;
  accent-color: hsla(160, 100%, 37%, 1);
  cursor: pointer;
}
.error {
  color: #e66;
  margin-top: 0.35rem;
}
</style>
