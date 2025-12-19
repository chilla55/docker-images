#!/bin/bash
# ============================================================================
# Pterodactyl Panel Healthcheck Script (4-Service Architecture)
# ============================================================================
# Checks different services based on running processes
# ============================================================================

set -e

# Detect which service is running
if pgrep -f "php-fpm" > /dev/null 2>&1; then
    # PHP-FPM: Check if listening on port 9000
    if nc -z -w2 127.0.0.1 9000 2>/dev/null; then
        echo "[HEALTHCHECK] PHP-FPM healthy (port 9000)"
        exit 0
    else
        echo "[HEALTHCHECK ERROR] PHP-FPM not listening on port 9000" >&2
        exit 1
    fi
elif pgrep -f "caddy" > /dev/null 2>&1; then
    # Caddy: Check if HTTP endpoint responds
    if curl -sf -o /dev/null --max-time 3 "http://localhost:80/" 2>/dev/null; then
        echo "[HEALTHCHECK] Caddy healthy (port 80)"
        exit 0
    else
        echo "[HEALTHCHECK ERROR] Caddy HTTP endpoint not responding" >&2
        exit 1
    fi
elif pgrep -f "queue:work" > /dev/null 2>&1; then
    # Queue worker: Check if process is running
    echo "[HEALTHCHECK] Queue worker healthy"
    exit 0
elif pgrep -f "schedule:run" > /dev/null 2>&1 || pgrep -f "cron" > /dev/null 2>&1; then
    # Cron: Check if process is running
    echo "[HEALTHCHECK] Cron scheduler healthy"
    exit 0
else
    echo "[HEALTHCHECK ERROR] No recognized service running" >&2
    exit 1
fi

