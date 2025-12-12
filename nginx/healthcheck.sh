#!/bin/bash
set -e
STATUS_FILE="${CF_REALIP_STATUS:-/tmp/cf-realip/cf-realip.status.json}"
INTERVAL="${CF_REALIP_INTERVAL:-21600}"
MAX_AGE=$(( INTERVAL * 2 ))
MAX_FAILS="${CF_REALIP_MAX_FAILS:-5}"
CHECK_MOUNTS="${CHECK_STORAGEBOX_MOUNTS:-1}"
CERTS_DIR="${CERTS_DIR:-/etc/nginx/certs}"
SITES_DIR="${SITES_DIR:-/etc/nginx/sites-enabled}"
CERT_WATCH_PATH="${CERT_WATCH_PATH:-}"

# Check nginx is responding via HTTP instead of pgrep
if ! curl -f -s http://localhost:80/ >/dev/null 2>&1; then
    echo "nginx not responding on port 80"
    exit 1
fi

[ -f "$STATUS_FILE" ] || { echo "no status file"; exit 1; }

# Optional: verify bind mounts are present (detect accidental unmounts)
if [ "$CHECK_MOUNTS" = "1" ] || [ "$CHECK_MOUNTS" = "true" ]; then
	for d in "$CERTS_DIR" "$SITES_DIR"; do
		[ -d "$d" ] || { echo "mount dir missing: $d"; exit 1; }
		if grep -q " $d " /proc/self/mountinfo; then
			LINE=$(grep " $d " /proc/self/mountinfo | head -1)
			FSTYPE=$(echo "$LINE" | awk -F' - ' '{print $2}' | awk '{print $1}')
			SOURCE=$(echo "$LINE" | awk -F' - ' '{print $2}' | awk '{print $2}')
			PROP=$(echo "$LINE" | awk '{for(i=1;i<=NF;i++){if($i~/(shared|master):/){print $i;exit}}}')
			echo "✓ Mount present at $d (type=${FSTYPE:-unknown}, source=${SOURCE:-unknown}, prop=${PROP:-n/a})"
		else
			echo "WARNING: No mount detected at $d (may be local files)"
			# Expect non-empty as a minimal sanity check
			ls -A "$d" >/dev/null 2>&1 || { echo "empty or unreadable: $d"; exit 1; }
		fi
	done
fi

# If CERT_WATCH_PATH is set, lightly validate cert presence like certbot
if [ -n "$CERT_WATCH_PATH" ]; then
	if [ ! -f "$CERT_WATCH_PATH" ]; then
		echo "certificate file not found: $CERT_WATCH_PATH"
		exit 1
	fi
	# Try to parse cert expiry and warn if <30 days (non-fatal)
	if openssl x509 -in "$CERT_WATCH_PATH" -noout -enddate >/dev/null 2>&1; then
		EXPIRY_DATE=$(openssl x509 -in "$CERT_WATCH_PATH" -noout -enddate | cut -d= -f2)
		EXPIRY_EPOCH=$(date -d "$EXPIRY_DATE" +%s 2>/dev/null || echo 0)
		NOW_EPOCH=$(date +%s)
		if [ "$EXPIRY_EPOCH" -gt 0 ]; then
			DAYS_LEFT=$(( (EXPIRY_EPOCH - NOW_EPOCH) / 86400 ))
			[ $DAYS_LEFT -lt 0 ] && { echo "certificate has EXPIRED"; exit 1; }
			[ $DAYS_LEFT -lt 30 ] && echo "WARNING: certificate expires in $DAYS_LEFT days" || true
		fi
	fi
fi

FAILS="$(jq -r '.consecutive_failures // 0' < "$STATUS_FILE")"
LAST_TS="$(jq -r '.last_ok_ts // 0' < "$STATUS_FILE")"

if [ "$FAILS" -ge "$MAX_FAILS" ]; then echo "too many failures: $FAILS"; exit 1; fi
NOW="$(date -u +%s)"; AGE=$(( NOW - LAST_TS ))
if [ "$LAST_TS" -eq 0 ] || [ "$AGE" -gt "$MAX_AGE" ]; then echo "stale status (age ${AGE}s > ${MAX_AGE}s)"; exit 1; fi
echo "ok"
