#!/bin/sh
set -eu

CERT_PATH="${CERT_WATCH_PATH:-}"
INTERVAL="${CERT_WATCH_INTERVAL:-300}"
DEBUG="${CERT_WATCH_DEBUG:-0}"

log() {
  [ "$DEBUG" = "1" ] && echo "[cert-watch] $*"
}

if [ -z "$CERT_PATH" ]; then
  log "CERT_WATCH_PATH not set – watcher disabled."
  exit 0
fi

log "Watching $CERT_PATH every ${INTERVAL}s"

# Wait for first cert
while [ ! -e "$CERT_PATH" ]; do
  log "Waiting for certificate file..."
  sleep 30
done

get_mtime() {
  date -r "$CERT_PATH" +%s 2>/dev/null || echo 0
}

LAST_MTIME="$(get_mtime)"
log "Initial mtime: $LAST_MTIME"

while :; do
  if [ -e "$CERT_PATH" ]; then
    MTIME="$(get_mtime)"

    if [ "$MTIME" != "$LAST_MTIME" ]; then
      echo "[cert-watch] Certificate changed (mtime $LAST_MTIME → $MTIME)"
      if [ -x /usr/local/bin/update-cf-ips.sh ]; then
        echo "[nginx-pre-reload] Updating Cloudflare IP list..."
        /usr/local/bin/update-cf-ips.sh || echo "[nginx-pre-reload] Cloudflare update failed"
      fi
      if nginx -t >/dev/null 2>&1; then
        echo "[cert-watch] nginx -t OK, reloading..."
        nginx -s reload 2>/dev/null || kill -HUP 1 || echo "[cert-watch] reload failed"
      else
        echo "[cert-watch] nginx -t FAILED, not reloading."
      fi
      LAST_MTIME="$MTIME"
    fi
  else
    log "$CERT_PATH missing; waiting..."
  fi

  sleep "$INTERVAL"
done
