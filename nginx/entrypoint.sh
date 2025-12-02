#!/bin/sh
set -eu

: "${CF_REALIP_AUTO:=1}"
: "${CF_REALIP_INTERVAL:=21600}"  # 6h
: "${CF_REALIP_STATUS:=/var/cache/nginx/cf-realip.status.json}"

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

# Foreground nginx (PID 1)
exec /usr/sbin/nginx -g 'daemon off;'
