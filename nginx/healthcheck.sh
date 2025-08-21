#!/bin/sh
set -eu
STATUS_FILE="${CF_REALIP_STATUS:-/var/cache/nginx/cf-realip.status.json}"
INTERVAL="${CF_REALIP_INTERVAL:-21600}"
MAX_AGE=$(( INTERVAL * 2 ))
MAX_FAILS="${CF_REALIP_MAX_FAILS:-5}"

pgrep -x nginx >/dev/null || { echo "nginx not running"; exit 1; }
[ -f "$STATUS_FILE" ] || { echo "no status file"; exit 1; }

FAILS="$(jq -r '.consecutive_failures // 0' < "$STATUS_FILE")"
LAST_TS="$(jq -r '.last_ok_ts // 0' < "$STATUS_FILE")"

if [ "$FAILS" -ge "$MAX_FAILS" ]; then echo "too many failures: $FAILS"; exit 1; fi
NOW="$(date -u +%s)"; AGE=$(( NOW - LAST_TS ))
if [ "$LAST_TS" -eq 0 ] || [ "$AGE" -gt "$MAX_AGE" ]; then echo "stale status (age ${AGE}s > ${MAX_AGE}s)"; exit 1; fi
echo "ok"
