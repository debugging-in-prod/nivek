// Build version surfaced in the App.vue footer. The deploy workflow rewrites
// this file with the actual commit SHA before the production build runs;
// local dev / non-CI builds see 'dev' so the footer never reads a stale SHA.
export const BUILD_VERSION = 'dev'
