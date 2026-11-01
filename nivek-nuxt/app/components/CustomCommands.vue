<script setup lang="ts">
import { API_ROUTES, ROLES, MIN_COOLDOWN, MAX_COOLDOWN } from '~/utils/constants'
import { errorMessage } from '~/utils/errorMessage'
import type { Command, CommandsResponse } from '~/types/commands'

// Editor for the logged-in channel's own custom chat commands
// (nivek.command, scope='channel', kind='custom'). The server scopes every
// request to the session user's channel and does all validation; the limits
// below only mirror it for a friendlier form.

interface CustomCommandDraft {
  trigger: string
  response_tmpl: string
  description: string
  min_role: string
  cooldown_secs: number
  enabled: boolean
}

const MAX_RESPONSE = 500
const MAX_DESCRIPTION = 200

const commands = ref<Command[]>([])
const loading = ref(true)
const error = ref<string | null>(null)
const saving = ref(false)

const blank = (): CustomCommandDraft => ({
  trigger: '',
  response_tmpl: '',
  description: '',
  min_role: 'everyone',
  cooldown_secs: 30,
  enabled: true,
})
const newDraft = reactive<CustomCommandDraft>(blank())

const editingId = ref<number | null>(null)
const editDraft = reactive<CustomCommandDraft>(blank())
const confirmingDelete = ref<number | null>(null)

async function load() {
  loading.value = true
  try {
    const res = await api<CommandsResponse>(API_ROUTES.Secure.GetChannelCommands)
    commands.value = res.commands ?? []
  } catch (err: unknown) {
    console.error('error fetching custom commands: ', err)
    error.value = errorMessage(err, 'Could not load your commands.')
  } finally {
    loading.value = false
  }
}

function payload(d: CustomCommandDraft) {
  return {
    trigger: d.trigger.trim(),
    response_tmpl: d.response_tmpl.trim(),
    description: d.description.trim(),
    min_role: d.min_role,
    cooldown_secs: Math.round(Number(d.cooldown_secs)),
    enabled: d.enabled,
  }
}

async function create() {
  if (saving.value) return
  saving.value = true
  error.value = null
  try {
    await api(API_ROUTES.Secure.PostCreateChannelCommand, { method: 'POST', body: payload(newDraft) })
    Object.assign(newDraft, blank())
    await load()
  } catch (err: unknown) {
    error.value = errorMessage(err, 'Could not create the command.')
  } finally {
    saving.value = false
  }
}

function startEdit(c: Command) {
  Object.assign(editDraft, {
    trigger: c.trigger,
    response_tmpl: c.response_tmpl ?? '',
    description: c.description,
    min_role: c.min_role,
    // Older rows may have no cooldown; the editor requires one.
    cooldown_secs: Math.max(MIN_COOLDOWN, c.cooldown_secs),
    enabled: c.enabled,
  })
  editingId.value = c.id
  error.value = null
}

async function update(id: number, d: CustomCommandDraft) {
  if (saving.value) return
  saving.value = true
  error.value = null
  try {
    await api(API_ROUTES.Secure.PutUpdateChannelCommand(id), { method: 'PUT', body: payload(d) })
    editingId.value = null
    await load()
  } catch (err: unknown) {
    error.value = errorMessage(err, 'Could not save the command.')
  } finally {
    saving.value = false
  }
}

function toggleEnabled(c: Command) {
  update(c.id, {
    trigger: c.trigger,
    response_tmpl: c.response_tmpl ?? '',
    description: c.description,
    min_role: c.min_role,
    cooldown_secs: Math.max(MIN_COOLDOWN, c.cooldown_secs),
    enabled: !c.enabled,
  })
}

async function remove(id: number) {
  error.value = null
  try {
    await api(API_ROUTES.Secure.DeleteChannelCommand(id), { method: 'DELETE' })
    confirmingDelete.value = null
    await load()
  } catch (err: unknown) {
    error.value = errorMessage(err, 'Could not delete the command.')
  }
}

onMounted(load)
</script>

<template>
  <h4 class="title">Custom Commands</h4>
  <p class="mb-2">
    Add your own chat commands that reply with a fixed message — a Discord link, socials, a schedule.
    Built-in commands (like <code>!dad</code> or <code>!so</code>) can't be reused as triggers.
  </p>
  <p class="mb-2 note">
    Changes are saved immediately and the bot picks them up the next time you go live.
  </p>

  <p v-if="error" class="error" role="alert">{{ error }}</p>

  <form class="mb-3 py-3" @submit.prevent="create()">
    <div class="row-fields">
      <label>Trigger
        <input v-model="newDraft.trigger" class="form-control" placeholder="!discord" maxlength="26" required />
      </label>
      <label>Who can use it
        <select v-model="newDraft.min_role" class="form-control">
          <option v-for="r in ROLES" :key="r" :value="r">{{ r }}</option>
        </select>
      </label>
      <label>Cooldown (s)
        <input
          v-model.number="newDraft.cooldown_secs"
          type="number"
          class="form-control"
          :min="MIN_COOLDOWN"
          :max="MAX_COOLDOWN"
          required
        />
      </label>
    </div>
    <label>Response
      <textarea
        v-model="newDraft.response_tmpl"
        class="form-control"
        rows="2"
        :maxlength="MAX_RESPONSE"
        placeholder="Join the Discord: https://discord.gg/..."
        required
      ></textarea>
    </label>
    <label>Description (optional)
      <input v-model="newDraft.description" class="form-control" :maxlength="MAX_DESCRIPTION" />
    </label>
    <button type="submit" class="btn btn-primary mt-2" :disabled="saving">Add Command</button>
  </form>

  <h5 class="section-label">Your Commands</h5>
  <p v-if="loading" class="empty">Loading…</p>
  <ul v-else class="cmd-list list-group">
    <li v-if="commands.length === 0" class="list-group-item empty">No custom commands yet — add one above.</li>

    <li v-for="c in commands" :key="c.id" class="list-group-item" :class="{ disabled: !c.enabled }">
      <template v-if="editingId !== c.id">
        <div class="cmd-body">
          <div><code>{{ c.trigger }}</code> <span v-if="!c.enabled" class="badge">disabled</span></div>
          <div class="cmd-response">{{ c.response_tmpl }}</div>
          <div class="meta">
            {{ c.min_role }} · {{ c.cooldown_secs }}s cooldown<span v-if="c.description"> · {{ c.description }}</span>
          </div>
        </div>
        <div class="cmd-actions">
          <button class="btn btn-sm btn-secondary" :disabled="saving" @click="toggleEnabled(c)">
            {{ c.enabled ? 'Disable' : 'Enable' }}
          </button>
          <button class="btn btn-sm btn-primary" @click="startEdit(c)">Edit</button>
          <template v-if="confirmingDelete !== c.id">
            <button class="btn btn-sm btn-danger" @click="confirmingDelete = c.id">Delete</button>
          </template>
          <template v-else>
            <button class="btn btn-sm btn-success" @click="remove(c.id)">YES</button>
            <button class="btn btn-sm btn-secondary" @click="confirmingDelete = null">NO</button>
          </template>
        </div>
      </template>

      <template v-else>
        <form class="cmd-body" @submit.prevent="update(c.id, editDraft)">
          <div class="row-fields">
            <label>Trigger
              <input v-model="editDraft.trigger" class="form-control" maxlength="26" required />
            </label>
            <label>Who can use it
              <select v-model="editDraft.min_role" class="form-control">
                <option v-for="r in ROLES" :key="r" :value="r">{{ r }}</option>
              </select>
            </label>
            <label>Cooldown (s)
              <input
                v-model.number="editDraft.cooldown_secs"
                type="number"
                class="form-control"
                :min="MIN_COOLDOWN"
                :max="MAX_COOLDOWN"
                required
              />
            </label>
          </div>
          <label>Response
            <textarea v-model="editDraft.response_tmpl" class="form-control" rows="2" :maxlength="MAX_RESPONSE" required></textarea>
          </label>
          <label>Description (optional)
            <input v-model="editDraft.description" class="form-control" :maxlength="MAX_DESCRIPTION" />
          </label>
          <label class="inline"><input v-model="editDraft.enabled" type="checkbox" /> Enabled</label>
          <div class="cmd-actions mt-2">
            <button type="submit" class="btn btn-sm btn-success" :disabled="saving">Save</button>
            <button type="button" class="btn btn-sm btn-secondary" @click="editingId = null">Cancel</button>
          </div>
        </form>
      </template>
    </li>
  </ul>
</template>

<style scoped>
.title { border-bottom: 2px solid grey; }
.section-label { opacity: 0.8; font-size: 0.95rem; }
.note { font-size: 0.9rem; opacity: 0.85; }
.error { color: #e5484d; }
label { display: block; margin-bottom: 0.5rem; }
label.inline { display: flex; gap: 0.4rem; align-items: center; }
.row-fields { display: flex; gap: 0.75rem; flex-wrap: wrap; }
.row-fields label { flex: 1 1 10rem; }
.form-control {
  background-color: unset;
  border: 1px solid gray;
  border-radius: 6px;
  color: unset;
  width: 100%;
}
select.form-control option { color: initial; }
.btn.btn-primary { background-color: transparent; border: 1px solid gray; color: inherit; }
form.mb-3 { border-top: 2px solid gray; border-bottom: 2px solid gray; }
.cmd-list { max-height: 520px; overflow-y: auto; }
.cmd-list li {
  background: inherit;
  border-top: 0;
  border-left: 0;
  border-right: 0;
  border-color: var(--color-text);
  color: inherit;
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
  flex-wrap: wrap;
}
.cmd-list li.disabled { opacity: 0.55; }
.cmd-list li.empty { opacity: 0.6; font-style: italic; display: block; }
.cmd-body { flex: 1 1 60%; min-width: 0; }
.cmd-response { white-space: pre-wrap; overflow-wrap: anywhere; }
.cmd-actions { display: flex; gap: 0.35rem; flex-wrap: wrap; align-items: flex-start; }
.meta { font-size: 0.8rem; opacity: 0.6; margin-top: 0.25rem; }
.badge { border: 1px solid gray; border-radius: 6px; padding: 0.1rem 0.4rem; font-size: 0.7rem; margin-left: 0.4rem; }
</style>
