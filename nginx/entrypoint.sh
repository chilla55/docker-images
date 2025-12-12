#!/bin/bash
set -eu

: "${CF_REALIP_AUTO:=1}"
: "${CF_REALIP_INTERVAL:=21600}"  # 6h
: "${CF_REALIP_STATUS:=/var/cache/nginx/cf-realip.status.json}"

# Sites watcher settings
: "${SITES_WATCH_PATH:=/etc/nginx/sites-enabled}"
: "${SITES_WATCH_INTERVAL:=30}"
: "${SITES_WATCH_DEBUG:=0}"

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

# Cleanup function for graceful shutdown
cleanup() {
    log "[entrypoint] Shutting down..."
    exit 0
}

trap cleanup TERM INT

# ──────────────────────────────────────────────────────────────────────────
# Setup
# ──────────────────────────────────────────────────────────────────────────

# Ensure cache dir + status file exist before first run
mkdir -p "$(dirname "$CF_REALIP_STATUS")"
[ -f "$CF_REALIP_STATUS" ] || : > "$CF_REALIP_STATUS"

run_once() { /usr/local/bin/update-cf-ips.sh || echo "[cloudflare-realip] update failed"; }

# Cloudflare updater loop
if [ "$CF_REALIP_AUTO" = "1" ] || [ "$CF_REALIP_AUTO" = "true" ]; then
  run_once || true
  ( while :; do sleep "$CF_REALIP_INTERVAL"; run_once || true; done ) &
else
  echo "[cloudflare-realip] Auto-update disabled."
fi

# Cert watcher loop (only if path is set)
if [ -n "${CERT_WATCH_PATH:-}" ]; then
  echo "[entrypoint] Starting cert watcher for: $CERT_WATCH_PATH"
  /usr/local/bin/watch-cert-reload.sh &
else
  echo "[entrypoint] CERT_WATCH_PATH not set, cert watcher disabled."
fi

# Sites watcher loop (watches for site configuration changes)
if [ -n "${SITES_WATCH_PATH:-}" ]; then
  echo "[entrypoint] Starting sites watcher for: $SITES_WATCH_PATH"
  /usr/local/bin/watch-sites-reload.sh &
else
  echo "[entrypoint] SITES_WATCH_PATH not set, sites watcher disabled."
fi

# Ensure cache and log dirs are writable
chown -R nginx:nginx /var/cache/nginx /etc/nginx/logs 2>/dev/null || true

# Run nginx (PID 1) as root (safe in container due to Docker isolation)
log "[entrypoint] Starting nginx"
exec /usr/sbin/nginx -g 'daemon off;'
