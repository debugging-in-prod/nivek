#!/bin/bash
# install-error-monitor.sh - One-time setup for the error monitoring pipeline
# Run this on the Raspberry Pi after cloning/pulling the repo

set -euo pipefail

REPO="tim-the-toolman-taylor/nivek"
LABEL="bot-error"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MONITOR_SCRIPT="$SCRIPT_DIR/error-monitor.sh"
LOG_FILE="$(dirname "$SCRIPT_DIR")/data/error-monitor.log"
CRON_SCHEDULE="*/30 * * * *"

echo "=== Nivek Error Monitor Setup ==="
echo ""

# 1. Check/install gh CLI
if command -v gh &>/dev/null; then
    echo "[OK] gh CLI already installed: $(gh --version | head -1)"
else
    echo "[INSTALL] Installing gh CLI..."
    # Detect architecture
    ARCH=$(dpkg --print-architecture 2>/dev/null || uname -m)
    case "$ARCH" in
        arm64|aarch64) ARCH="arm64" ;;
        armhf|armv7l)  ARCH="armv6" ;;
        amd64|x86_64)  ARCH="amd64" ;;
        *)
            echo "ERROR: Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac

    GH_VERSION="2.67.0"
    GH_URL="https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${ARCH}.tar.gz"
    TMP_DIR=$(mktemp -d)

    echo "  Downloading gh ${GH_VERSION} for ${ARCH}..."
    curl -sL "$GH_URL" -o "$TMP_DIR/gh.tar.gz"
    tar -xzf "$TMP_DIR/gh.tar.gz" -C "$TMP_DIR"
    sudo cp "$TMP_DIR"/gh_*/bin/gh /usr/local/bin/gh
    sudo chmod +x /usr/local/bin/gh
    rm -rf "$TMP_DIR"

    echo "[OK] gh CLI installed: $(gh --version | head -1)"
fi

# 2. Check gh authentication
echo ""
if gh auth status &>/dev/null; then
    echo "[OK] gh is authenticated"
else
    echo "[AUTH] gh is not authenticated. Please run:"
    echo ""
    echo "  gh auth login"
    echo ""
    echo "  Choose: GitHub.com → HTTPS → Paste an authentication token"
    echo "  Generate a token at: https://github.com/settings/tokens"
    echo "  Required scopes: repo (full control of private repositories)"
    echo ""
    echo "Run this script again after authenticating."
    exit 1
fi

# 3. Create bot-error label (idempotent)
echo ""
if gh label list --repo "$REPO" --search "$LABEL" --json name --jq '.[].name' 2>/dev/null | grep -q "^${LABEL}$"; then
    echo "[OK] Label '$LABEL' already exists"
else
    echo "[CREATE] Creating '$LABEL' label..."
    gh label create "$LABEL" \
        --repo "$REPO" \
        --description "Auto-detected bot errors from log monitoring" \
        --color "d73a4a" \
        2>/dev/null || echo "[WARN] Could not create label (may already exist)"
    echo "[OK] Label '$LABEL' created"
fi

# 4. Ensure data directory exists for log file
mkdir -p "$(dirname "$LOG_FILE")"

# 5. Install cron job
echo ""
CRON_CMD="$CRON_SCHEDULE $MONITOR_SCRIPT >> $LOG_FILE 2>&1"
EXISTING_CRON=$(crontab -l 2>/dev/null || true)

if echo "$EXISTING_CRON" | grep -qF "error-monitor.sh"; then
    echo "[OK] Cron job already installed"
else
    echo "[INSTALL] Installing cron job (every 30 minutes)..."
    (echo "$EXISTING_CRON"; echo "$CRON_CMD") | crontab -
    echo "[OK] Cron job installed"
fi

# 6. Verify everything
echo ""
echo "=== Verification ==="
echo "  gh CLI:      $(which gh)"
echo "  gh auth:     $(gh auth status 2>&1 | grep 'Logged in' | head -1 || echo 'check manually')"
echo "  Monitor:     $MONITOR_SCRIPT"
echo "  Log file:    $LOG_FILE"
echo "  Cron:"
crontab -l 2>/dev/null | grep "error-monitor" | sed 's/^/    /'
echo ""
echo "=== Setup Complete ==="
echo ""
echo "The error monitor will run every 30 minutes."
echo "To test now: $MONITOR_SCRIPT"
echo "To view log: tail -f $LOG_FILE"
