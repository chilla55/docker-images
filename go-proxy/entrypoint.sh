#!/bin/sh
set -e

echo "Starting proxy-manager with internal cron..."

# Use proxy manager environment defaults
: "${SITES_PATH:=/etc/proxy/sites-available}"
: "${PROXY_CERTS_DIR:=/etc/proxy/certs}"

if [ ! -d "$SITES_PATH" ]; then
    echo "ERROR: Sites directory not found: $SITES_PATH" >&2
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
