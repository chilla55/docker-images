#!/bin/sh
set -eu

CERT_PATH="${CERT_WATCH_PATH:-}"
INTERVAL="${CERT_WATCH_INTERVAL:-300}"

if [ -z "$CERT_PATH" ]; then
  echo "[cert-watch] CERT_WATCH_PATH not set – watcher disabled."
  exit 0
fi

echo "[cert-watch] Watching $CERT_PATH every ${INTERVAL}s"

# Wait for first cert
while [ ! -e "$CERT_PATH" ]; do
  echo "[cert-watch] Waiting for certificate file..."
  sleep 30
done

LAST_MTIME="$(stat -c %Y "$CERT_PATH" || echo 0)"

while :; do
  if [ -e "$CERT_PATH" ]; then
    MTIME="$(stat -c %Y "$CERT_PATH" || echo 0)"
    if [ "$MTIME" != "$LAST_MTIME" ]; then
      echo "[cert-watch] Change detected on $CERT_PATH (mtime $LAST_MTIME → $MTIME)"
      if nginx -t >/dev/null 2>&1; then
        echo "[cert-watch] nginx -t OK, reloading..."
        nginx -s reload 2>/dev/null || kill -HUP 1 || echo "[cert-watch] reload failed"
      else
        echo "[cert-watch] nginx -t FAILED, not reloading."
      fi
      LAST_MTIME="$MTIME"
    fi
  else
    echo "[cert-watch] $CERT_PATH missing; waiting..."
  fi

  sleep "$INTERVAL"
done
