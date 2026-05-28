# Plan: Automated Error → GitHub Issue Pipeline

## Overview
A bash script in the nivek repo that runs on a cron schedule on the Pi, scans `twitch-bot` journalctl logs for errors, and creates GitHub issues via `gh` CLI with a `bot-error` label. Deduplicates to avoid flooding.

## Prerequisites
- Install `gh` CLI on the Pi (`apt install gh` or binary download)
- Authenticate `gh` on the Pi (`gh auth login` — one-time setup by user)
- Create the `bot-error` label in the GitHub repo

## Files to Create

### 1. `scripts/error-monitor.sh`
Bash script that:
1. Reads journalctl for `twitch-bot` errors from the last 30 minutes
2. Filters for actionable error patterns:
   - `panic:` or `Recovered from Twitch IRC panic`
   - `ERROR` / `error` / `failed` in log lines (excluding routine "Error connecting" retries)
   - `duplicate key` constraint violations
   - Stack traces from `upper: slow query` (only when paired with actual errors)
3. Groups related log lines into a single error event (e.g., a stack trace + error message = one event)
4. Generates a fingerprint (hash) for each error to deduplicate
5. Checks open GitHub issues with `bot-error` label for matching fingerprint
6. If no matching issue exists, creates one with:
   - Title: short error summary
   - Body: full log excerpt, timestamp, fingerprint hash
   - Label: `bot-error`

### 2. `scripts/install-error-monitor.sh`
One-time setup script that:
1. Installs `gh` if not present
2. Creates the `bot-error` label in the repo (idempotent)
3. Installs the cron job (every 30 minutes)
4. Verifies `gh auth status`

## Cron Schedule
```
*/30 * * * * /home/nut/nivek/scripts/error-monitor.sh >> /home/nut/nivek/data/error-monitor.log 2>&1
```

## Error Fingerprinting
- Extract the core error message (strip timestamps, PIDs, variable data like usernames/IPs)
- MD5 hash the normalized error string
- Store fingerprint in the issue body as `<!-- fingerprint: abc123 -->`
- Search existing open issues for that fingerprint before creating

## What Gets Captured vs Ignored

**Capture:**
- Panics and panic recovery
- Database constraint violations
- Failed database operations (create, update, delete)
- Service initialization failures
- Connection failures that persist (not transient retries)

**Ignore:**
- Routine "Midnight reached! Resetting all counters..."
- "Connected to Twitch IRC" (success)
- "Joining channel:" (success)
- Single transient "Error connecting to Twitch" (retries are expected)
- "upper: slow query" warnings without actual errors

## Deployment
- Script lives in `scripts/` in the repo
- Deployed to Pi via `git pull` (same as the bot)
- Cron job installed once via `install-error-monitor.sh`
