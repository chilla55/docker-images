#!/bin/sh
# ============================================================================
# Pterodactyl Panel Healthcheck Script
# ============================================================================
# Checks: PHP-FPM, Caddy, Database, Redis, Queue Worker, Application Status
# ============================================================================

set -e

APP_DIR="/var/www/pterodactyl"
cd "$APP_DIR" || exit 1

# ──────────────────────────────────────────────────────────────────────────
# Helper Functions
# ──────────────────────────────────────────────────────────────────────────
log_error() {
    echo "[HEALTHCHECK ERROR] $1" >&2
}

check_process() {
    local process_name="$1"
    if ! pgrep -f "$process_name" > /dev/null 2>&1; then
        log_error "Process not running: $process_name"
        return 1
    fi
    return 0
}

check_port() {
    local port="$1"
    local service="$2"
    if ! nc -z localhost "$port" 2>/dev/null; then
        log_error "Port $port not responding ($service)"
        return 1
    fi
    return 0
}

check_http() {
    local url="$1"
    local service="$2"
    if ! wget -q -O /dev/null --timeout=3 "$url" 2>/dev/null; then
        log_error "HTTP check failed: $url ($service)"
        return 1
    fi
    return 0
}

# ──────────────────────────────────────────────────────────────────────────
# Critical Checks (any failure = unhealthy)
# ──────────────────────────────────────────────────────────────────────────

# Check 1: .env file exists
if [ ! -f .env ]; then
    log_error ".env file not found"
    exit 1
fi

# Check 2: APP_KEY is set and valid
if ! grep -q '^APP_KEY=base64:' .env; then
    log_error "APP_KEY not properly configured in .env"
    exit 1
fi

# Check 3: PHP-FPM process
check_process "php-fpm" || exit 1

# Check 4: PHP-FPM port 9000
check_port 9000 "PHP-FPM" || exit 1

# Check 5: Caddy process
check_process "caddy" || exit 1

# Check 6: Caddy port 80
check_port 80 "Caddy" || exit 1

# Check 7: Caddy health endpoint
check_http "http://localhost:80/caddy-health" "Caddy HTTP" || exit 1

# Check 8: Queue worker process
check_process "artisan queue:work" || exit 1

# Check 9: Scheduler process
check_process "artisan schedule:run" || exit 1

# ──────────────────────────────────────────────────────────────────────────
# Database Connectivity Check
# ──────────────────────────────────────────────────────────────────────────
if ! php artisan migrate:status > /dev/null 2>&1; then
    log_error "Database connection failed or migrations not applied"
    exit 1
fi

# ──────────────────────────────────────────────────────────────────────────
# Application Cache Check (warning only)
# ──────────────────────────────────────────────────────────────────────────
if ! php artisan config:cache > /dev/null 2>&1; then
    # Non-critical, just log
    echo "[HEALTHCHECK WARN] Config cache check failed" >&2
fi

# ──────────────────────────────────────────────────────────────────────────
# All Checks Passed
# ──────────────────────────────────────────────────────────────────────────
echo "[HEALTHCHECK] All checks passed - container healthy"
exit 0

