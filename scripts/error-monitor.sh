#!/bin/bash
# error-monitor.sh - Scans twitch-bot logs for errors and creates GitHub issues
# Run via cron every 30 minutes

set -euo pipefail

REPO="tim-the-toolman-taylor/nivek"
LABEL="bot-error"
LOOKBACK="30 min ago"
LOG_UNIT="twitch-bot"

# Patterns that indicate real errors (grep -E pattern)
ERROR_PATTERNS='panic:|Recovered from Twitch IRC panic|duplicate key|SQLSTATE|failed to (get|create|update|delete|open|find)|Bot error:|level=error'

# Patterns to ignore (routine/transient)
IGNORE_PATTERNS='Error connecting to Twitch: client called Disconnect\(\)|upper: slow query'

extract_errors() {
    journalctl -u "$LOG_UNIT" --no-pager --since "$LOOKBACK" 2>/dev/null \
        | grep -E "$ERROR_PATTERNS" \
        | grep -vE "$IGNORE_PATTERNS" \
        || true
}

# Group consecutive log lines into error events.
# A new event starts when there's a >2 second gap between lines or a new error pattern.
group_errors() {
    local current_event=""
    local last_ts=0

    while IFS= read -r line; do
        # Extract timestamp (seconds since epoch) from journal format
        local ts
        ts=$(date -d "$(echo "$line" | awk '{print $1, $2, $3}')" +%s 2>/dev/null || echo 0)

        # If gap > 2 seconds or first line, start new event
        if [[ $last_ts -eq 0 ]] || [[ $((ts - last_ts)) -gt 2 ]]; then
            # Emit previous event if exists
            if [[ -n "$current_event" ]]; then
                echo "---EVENT---"
                echo "$current_event"
            fi
            current_event="$line"
        else
            current_event="$current_event
$line"
        fi
        last_ts=$ts
    done

    # Emit final event
    if [[ -n "$current_event" ]]; then
        echo "---EVENT---"
        echo "$current_event"
    fi
}

# Create a fingerprint from an error event by stripping variable data
fingerprint() {
    local event="$1"
    echo "$event" \
        | sed -E 's/[0-9]{4}\/[0-9]{2}\/[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}//g' \
        | sed -E 's/\b[0-9]+\.[0-9]+s\b//g' \
        | sed -E 's/pid=[0-9]+//g' \
        | sed -E 's/\[[0-9]+\]//g' \
        | sed -E 's/Session ID:\s*[0-9]+//g' \
        | sed -E 's/Time taken:\s*[0-9.]+s//g' \
        | sed -E 's/(dial tcp|connect:) [0-9.:]+/\1 ADDR/g' \
        | sed -E 's/[a-zA-Z0-9_]+@[a-zA-Z0-9._]+/USER/g' \
        | md5sum | awk '{print $1}'
}

# Extract a short title from an error event
make_title() {
    local event="$1"
    # Find the most descriptive error line
    local title
    title=$(echo "$event" \
        | grep -oE '(panic:.*|Recovered from.*|failed to [^:]+|duplicate key.*constraint "[^"]*"|ERROR:.*SQLSTATE.*|level=error msg="[^"]*")' \
        | head -1 \
        | cut -c1-80)

    if [[ -z "$title" ]]; then
        title=$(echo "$event" | grep -iE 'error|fail|panic' | head -1 | sed -E 's/^.*twitch-bot[^:]*: //' | cut -c1-80)
    fi

    if [[ -z "$title" ]]; then
        title="twitch-bot error detected"
    fi

    echo "[bot-error] $title"
}

# Check if an issue with this fingerprint already exists
issue_exists() {
    local fp="$1"
    local count
    count=$(gh issue list --repo "$REPO" --label "$LABEL" --state open --search "$fp" --json number --jq 'length' 2>/dev/null || echo "0")
    [[ "$count" -gt 0 ]]
}

# Create a GitHub issue for an error event
create_issue() {
    local event="$1"
    local fp="$2"
    local title
    title=$(make_title "$event")
    local timestamp
    timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    local body
    body=$(cat <<EOF
## Auto-detected bot error

**Detected at:** $timestamp
**Service:** $LOG_UNIT
**Host:** $(hostname 2>/dev/null || echo "unknown")

### Log excerpt
\`\`\`
$event
\`\`\`

### Metadata
- Fingerprint: \`$fp\`
- Detection window: last 30 minutes

<!-- fingerprint: $fp -->
EOF
)

    gh issue create \
        --repo "$REPO" \
        --title "$title" \
        --body "$body" \
        --label "$LABEL" \
        2>&1

    echo "$(date): Created issue: $title (fingerprint: $fp)"
}

main() {
    # Verify gh is available and authenticated
    if ! command -v gh &>/dev/null; then
        echo "$(date): ERROR - gh CLI not found"
        exit 1
    fi

    if ! gh auth status &>/dev/null; then
        echo "$(date): ERROR - gh not authenticated"
        exit 1
    fi

    # Extract and group errors
    local errors
    errors=$(extract_errors)

    if [[ -z "$errors" ]]; then
        exit 0
    fi

    # Process each error event
    echo "$errors" | group_errors | while IFS= read -r line; do
        if [[ "$line" == "---EVENT---" ]]; then
            current_event=""
            continue
        fi

        if [[ -z "${current_event:-}" ]]; then
            current_event="$line"
        else
            current_event="$current_event
$line"
        fi

        # Check if next line is a new event marker or end of input
        # We process on event boundaries, handled by reading ahead
    done < <(
        echo "$errors" | group_errors
        echo "---EVENT---"  # sentinel to flush last event
    ) 2>/dev/null || true

    # Simpler approach: process events delimited by ---EVENT---
    local events
    events=$(echo "$errors" | group_errors)

    local current=""
    while IFS= read -r line; do
        if [[ "$line" == "---EVENT---" ]]; then
            if [[ -n "$current" ]]; then
                local fp
                fp=$(fingerprint "$current")
                if ! issue_exists "$fp"; then
                    create_issue "$current" "$fp"
                else
                    echo "$(date): Skipping duplicate (fingerprint: $fp)"
                fi
            fi
            current=""
        else
            if [[ -z "$current" ]]; then
                current="$line"
            else
                current="$current
$line"
            fi
        fi
    done <<< "$events
---EVENT---"
}

main "$@"
