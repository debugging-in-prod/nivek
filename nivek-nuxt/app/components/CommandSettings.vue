<script setup lang="ts">
import { API_ROUTES, ROLES, MIN_COOLDOWN, MAX_COOLDOWN } from '~/utils/constants'
import type { CommandSetting, CommandSettingsResponse } from '~/types/commands'

// Per-channel overrides of the bot's built-in commands
// (nivek.channel_command_settings). "Default" means the channel inherits the
// command's global setting; the server scopes every write to the session
// user's channel and validates every field.

interface CommandDraft {
  enabled: '' | 'on' | 'off'
  min_role: string // '' = default
  cooldown: string | number // '' = default (Vue casts number inputs)
}

const settings = ref<CommandSetting[]>([])
const drafts = reactive<Record<number, CommandDraft>>({})
const loading = ref(true)
const savingId = ref<number | null>(null)
const error = ref<string | null>(null)
const savedId = ref<number | null>(null)

function errMessage(err: unknown, fallback: string): string {
  const data = (err as { data?: { error?: string, message?: string } })?.data
  return data?.error || data?.message || fallback
}

function draftOf(s: CommandSetting): CommandDraft {
  return {
    enabled: s.enabled === null ? '' : s.enabled ? 'on' : 'off',
    min_role: s.min_role ?? '',
    cooldown: s.cooldown_secs === null ? '' : String(s.cooldown_secs),
  }
}

function isOverridden(s: CommandSetting): boolean {
  return s.enabled !== null || s.min_role !== null || s.cooldown_secs !== null
}

async function load() {
  loading.value = true
  try {
    const res = await api<CommandSettingsResponse>(API_ROUTES.Secure.GetCommandSettings)
    settings.value = res.settings ?? []
    settings.value.forEach((s) => { drafts[s.command_id] = draftOf(s) })
  } catch (err: unknown) {
    console.error('error fetching command settings: ', err)
    error.value = errMessage(err, 'Could not load command settings.')
  } finally {
    loading.value = false
  }
}

async function save(s: CommandSetting) {
  const d = drafts[s.command_id]
  if (!d || savingId.value !== null) return
  error.value = null
  savedId.value = null
  const cooldown = String(d.cooldown).trim()
  savingId.value = s.command_id
  try {
    await api(API_ROUTES.Secure.PutCommandSetting(s.command_id), {
      method: 'PUT',
      body: {
        enabled: d.enabled === '' ? null : d.enabled === 'on',
        min_role: d.min_role === '' ? null : d.min_role,
        cooldown_secs: cooldown === '' ? null : Math.round(Number(cooldown)),
      },
    })
    await load()
    savedId.value = s.command_id
  } catch (err: unknown) {
    error.value = `${s.trigger}: ${errMessage(err, 'could not save.')}`
  } finally {
    savingId.value = null
  }
}

async function reset(s: CommandSetting) {
  if (savingId.value !== null) return
  error.value = null
  savedId.value = null
  savingId.value = s.command_id
  try {
    await api(API_ROUTES.Secure.DeleteCommandSetting(s.command_id), { method: 'DELETE' })
    await load()
  } catch (err: unknown) {
    error.value = `${s.trigger}: ${errMessage(err, 'could not reset.')}`
  } finally {
    savingId.value = null
  }
}

onMounted(load)
</script>

<template>
  <h4 class="title">Command Settings</h4>
  <p class="mb-2">
    Choose how the bot's built-in commands behave in your channel. "Default" keeps the bot-wide setting.
  </p>
  <p class="mb-2 note">
    Your preferences are saved now; the bot will start applying per-channel settings in an upcoming update.
  </p>

  <p v-if="error" class="error" role="alert">{{ error }}</p>
  <p v-if="loading" class="empty">Loading…</p>

  <ul v-else class="set-list list-group">
    <li v-for="s in settings" :key="s.command_id" class="list-group-item">
      <div class="set-head">
        <code>{{ s.trigger }}</code>
        <span v-if="isOverridden(s)" class="badge">customized</span>
        <span v-if="savedId === s.command_id" class="saved">saved</span>
        <div class="meta">{{ s.description }}</div>
      </div>
      <div v-if="drafts[s.command_id]" class="set-fields">
        <label>Enabled
          <select v-model="drafts[s.command_id]!.enabled" class="form-control">
            <option value="">Default ({{ s.default_enabled ? 'on' : 'off' }})</option>
            <option value="on">On</option>
            <option value="off">Off</option>
          </select>
        </label>
        <label>Who can use it
          <select v-model="drafts[s.command_id]!.min_role" class="form-control">
            <option value="">Default ({{ s.default_min_role }})</option>
            <option v-for="r in ROLES" :key="r" :value="r">{{ r }}</option>
          </select>
        </label>
        <label>Cooldown (s)
          <input
            v-model="drafts[s.command_id]!.cooldown"
            type="number"
            class="form-control"
            :min="MIN_COOLDOWN"
            :max="MAX_COOLDOWN"
            :placeholder="`Default (${s.default_cooldown_secs})`"
          />
        </label>
        <div class="set-actions">
          <button class="btn btn-sm btn-success" :disabled="savingId !== null" @click="save(s)">Save</button>
          <button
            class="btn btn-sm btn-secondary"
            :disabled="savingId !== null || !isOverridden(s)"
            @click="reset(s)"
          >Reset</button>
        </div>
      </div>
    </li>
  </ul>
</template>

<style scoped>
.title { border-bottom: 2px solid grey; }
.note { font-size: 0.9rem; opacity: 0.85; }
.error { color: #e5484d; }
.saved { font-size: 0.75rem; opacity: 0.7; margin-left: 0.4rem; }
.set-list { max-height: 560px; overflow-y: auto; }
.set-list li {
  background: inherit;
  border-top: 0;
  border-left: 0;
  border-right: 0;
  border-color: var(--color-text);
  color: inherit;
}
.set-head { margin-bottom: 0.4rem; }
.set-fields { display: flex; gap: 0.75rem; flex-wrap: wrap; align-items: flex-end; }
.set-fields label { flex: 1 1 9rem; }
.set-actions { display: flex; gap: 0.35rem; }
.form-control {
  background-color: unset;
  border: 1px solid gray;
  border-radius: 6px;
  color: unset;
  width: 100%;
}
select.form-control option { color: initial; }
.meta { font-size: 0.8rem; opacity: 0.6; margin-top: 0.25rem; }
.badge { border: 1px solid gray; border-radius: 6px; padding: 0.1rem 0.4rem; font-size: 0.7rem; margin-left: 0.4rem; }
</style>
