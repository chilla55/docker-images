#!/bin/sh
# Vaultwarden healthcheck

set -e

# Check if Vaultwarden is responding
if wget --no-verbose --tries=1 --spider http://localhost:80/alive 2>/dev/null; then
    exit 0
else
    exit 1
fi
