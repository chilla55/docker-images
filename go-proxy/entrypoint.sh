#!/bin/sh
set -e

echo "Starting proxy-manager with internal cron..."

: "${PROXY_SITES_DIR:=/etc/nginx/sites-enabled}"
: "${PROXY_CERTS_DIR:=/mnt/storagebox/certs}"

if [ ! -d "$PROXY_SITES_DIR" ]; then
    echo "ERROR: Sites directory not found: $PROXY_SITES_DIR" >&2
    exit 1
fi

if [ ! -d "$PROXY_CERTS_DIR" ]; then
    echo "ERROR: Certs directory not found: $PROXY_CERTS_DIR" >&2
    exit 1
fi

# Start cron daemon in background
crond -b -l 2 -L /var/log/cron.log || true
sleep 1

echo "Cron daemon started, jobs scheduled:"
crontab -l || true

echo "Starting proxy manager..."
exec /usr/local/bin/proxy-manager "$@"
