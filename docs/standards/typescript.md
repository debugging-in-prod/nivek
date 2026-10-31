# TypeScript / Nuxt standards

These rules apply to `nivek-nuxt/` (Nuxt 4, Vue 3, Pinia, Bootstrap 5 CSS).
Source lives under `nivek-nuxt/app/`. CI job `nuxt` runs `npm ci && npm run build`.

## 1. Directory layout

| Directory | Holds | Rule |
| --- | --- | --- |
| `app/pages/` | Route components only | Files that aren't pages (e.g. a canvas renderer) don't belong here, because Nuxt may turn them into routes. Put them in `app/utils/`. |
| `app/layouts/` | `default.vue` (sidebar, top bar, logout) | |
| `app/components/` | Feature panels and widgets, PascalCase (`CustomCommands.vue`) | One feature panel per file. Panels own their own fetching. |
| `app/composables/` | `useX.ts`, shared reactive logic (`useState`-backed) | The file name matches the main export (`useDashPanels.ts` → `useDashPanels`). |
| `app/stores/` | Pinia setup stores (`defineStore('auth', () => …)`) | Use them for app-wide state only (auth today). Other components call store actions and don't mutate store state directly. |
| `app/middleware/` | Route middleware, kebab-case; `*.global.ts` for global | |
| `app/plugins/` | Plugins; `.client.ts` when they need the browser | |
| `app/utils/` | Framework-free helpers: `api.ts` (HTTP client), `constants.ts` (`API_ROUTES`), normalisers (`publicStats.ts`) | Code that doesn't need Nuxt imports goes here so `node --test` can test it. |
| `app/types/` | Shared TypeScript types for API payloads (`df.ts`) | All API request and response types go here (§3). |
| `app/assets/css/` | `main.css` → `base.css` (CSS variables) | |
| `tests/` | `*.test.ts` run by `npm test` (`node --test`) | |

There is no `server/` directory and there must not be one. The browser calls
core-api directly at same-origin `/api` (Traefik routes it). Don't add Nuxt
server routes or a proxy layer.

## 2. Calling core-api

**Use `api<T>(API_ROUTES.<…>, opts)` from `app/utils/api.ts`.** This is the
dominant pattern (AutoShout, BotOptInToggle, CommandSettings, CustomCommands,
DadResponses, FishScore, Promos, Stalk, Weather). The shared client:

- sets `baseURL: '/api'` and `credentials: 'include'` (HttpOnly session cookie);
- attaches the double-submit CSRF header (`X-CSRF-Token` from `nivek_csrf`) on
  unsafe methods;
- dispatches `auth:unauthorized` on 401, which the auth store handles.

Rules:

1. **Paths come from `API_ROUTES`** in `utils/constants.ts`. Don't write
   string literals like `'/profile'` or `'/api/df/snapshot'` at call sites.
   `API_ROUTES` entries are relative to `/api` (they don't include it). The
   exception is the full-page navigation link to `/api/auth/twitch/start`,
   which still uses a constant (`API_ROUTES.TwitchStart`).
2. **Don't call `$fetch` directly** for core-api, because it skips CSRF and 401
   handling. `useFetch` is allowed only for public, read-only data where
   SSR-safe reactive status helps (`BotStats.vue`), and it still takes its
   path from `API_ROUTES`.
3. Client-only data loads in `onMounted`, which is the existing pattern. A
   page that only works in the browser may set
   `definePageMeta({ ssr: false })`.
4. Third-party calls from the browser need a CSP entry in `nuxt.config.ts`
   and a reason in a comment. Prefer adding a core-api endpoint instead.
5. Group fetching functions per component as `load<Thing>()`, `create<Thing>()`,
   `update<Thing>()`, `remove<Thing>()`.

## 3. Types

- Write API payload types **once** in `app/types/<feature>.ts`, named after the
  core-api JSON shape (`snake_case` fields kept as-is to match the Go `json`
  tags), and import them where they're used.
- UI-only types (form drafts) may stay in the component, named `<Thing>Draft`.
- Write response envelopes as named types too
  (`type CommandSettingsResponse = { settings: CommandSetting[] }`), not inline
  generics.
- Don't use `any`. Catch errors as `unknown` and narrow them through a shared
  helper (§6).
- Shared enums and limits (`ROLES`, cooldown min/max) go in
  `utils/constants.ts`, mirroring the Go constants, with a comment naming the
  Go source.

## 4. Components and SFCs

- Always `<script setup lang="ts">` with the Composition API. Block order is
  `<script>`, `<template>`, `<style scoped>`.
- Components are PascalCase. Pages and middleware are kebab/lowercase. Utils
  are camelCase. Composables are `useX`.
- Rely on Nuxt auto-imports for `utils/`, `composables/`, and `stores/`
  consistently. Either import explicitly everywhere in a file or nowhere, and
  don't mix the two for the same symbol across files.
- Indentation is 2 spaces. Some files use 4, and nothing enforces it yet (see
  [refactor-plan.md](refactor-plan.md)).
- `v-html` is allowed only for trusted, repo-local content (devlog markdown).
  Never use it for API data.

## 5. Styling

- Bootstrap 5 **CSS only** (loaded globally in `nuxt.config.ts`), plus
  `<style scoped>` per component. No Tailwind, no UI component library, no
  Bootstrap JS.
- Colours come from the CSS variables in `assets/css/base.css`. Don't add new
  hex literals. If a colour is missing, add a variable.
- Delete empty `<style scoped>` blocks.

## 6. Loading and error states

Every panel that fetches data uses the same shape:

```ts
const loading = ref(true)
const error = ref<string | null>(null)

async function loadThings() {
  loading.value = true
  error.value = null
  try {
    things.value = (await api<ThingsResponse>(API_ROUTES.Secure.Things)).things
  } catch (err: unknown) {
    console.error('loadThings failed', err)
    error.value = errorMessage(err, "Couldn't load things.")
  } finally {
    loading.value = false
  }
}
```

```vue
<p v-if="loading">Loading…</p>
<p v-else-if="error" class="error" role="alert">{{ error }}</p>
```

- Never fail silently: a failed load or save shows a message.
- Show the server's `{ "error": "…" }` text for 4xx (validation) responses,
  and a friendly generic message otherwise. Use one shared `errorMessage(err,
  fallback)` helper in `utils/`, not a copy per component.
- Don't render raw `err.message` from fetch internals.

## 7. Tests and tooling

- Pure logic in `utils/` gets a `tests/<name>.test.ts` (Node test runner with
  native TS stripping; see `tests/publicStats.test.ts`).
- `npm run build` must pass. ESLint, Prettier, and `nuxi typecheck` aren't set
  up yet. Adding them is a planned refactor PR, not something to slip into a
  feature PR.
