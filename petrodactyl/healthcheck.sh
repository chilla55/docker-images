#!/bin/bash
# ============================================================================
# Pterodactyl Panel Healthcheck Script
# ============================================================================
# Simplified: Checks supervisord and HTTP endpoint
# Supervisord handles individual process crashes
# ============================================================================

set -e

# ──────────────────────────────────────────────────────────────────────────
# Check 1: Supervisord is running
# ──────────────────────────────────────────────────────────────────────────
if ! pgrep -x supervisord > /dev/null 2>&1; then
    echo "[HEALTHCHECK ERROR] Supervisord not running" >&2
    exit 1
fi

# ──────────────────────────────────────────────────────────────────────────
# Check 2: PHP-FPM socket exists and is accessible
# ──────────────────────────────────────────────────────────────────────────
if [ ! -S /run/php-fpm/pterodactyl.sock ]; then
    echo "[HEALTHCHECK ERROR] PHP-FPM socket not found" >&2
    exit 1
fi

# ──────────────────────────────────────────────────────────────────────────
# Check 3: HTTP endpoint responds
# ──────────────────────────────────────────────────────────────────────────
if ! curl -sf -o /dev/null --max-time 3 "http://localhost:80/" 2>/dev/null; then
    echo "[HEALTHCHECK ERROR] HTTP endpoint not responding" >&2
    exit 1
fi

# ──────────────────────────────────────────────────────────────────────────
# All Checks Passed
# ──────────────────────────────────────────────────────────────────────────
echo "[HEALTHCHECK] All checks passed - container healthy"
exit 0

