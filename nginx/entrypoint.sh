#!/bin/sh
set -eu
: "${CF_REALIP_AUTO:=1}"
: "${CF_REALIP_INTERVAL:=21600}"        # 6h
: "${CF_REALIP_MAX_FAILS:=5}"
: "${CF_REALIP_PANIC_ON_MAX_FAILS:=1}"  # 1 = kill container after too many failures
run_once() {
  if /usr/local/bin/update-cf-ips.sh; then
    return 0
  else
    echo "[cloudflare-realip] update failed"
    if [ "${CF_REALIP_PANIC_ON_MAX_FAILS}" = "1" ]; then
      FAILS="$(jq -r '.consecutive_failures // 0' </var/cache/nginx/cf-realip.status.json 2>/dev/null || echo 0)"
      if [ "$FAILS" -ge "$CF_REALIP_MAX_FAILS" ]; then
        echo "[cloudflare-realip] too many failures ($FAILS) – terminating for restart."
        kill -TERM 1 || true
      fi
    fi
    return 1
  fi
}
if [ "$CF_REALIP_AUTO" = "1" ] || [ "$CF_REALIP_AUTO" = "true" ]; then
  run_once || true
  ( while :; do sleep "$CF_REALIP_INTERVAL"; run_once || true; done ) &
else
  echo "[cloudflare-realip] Auto-update disabled."
fi
exec /usr/sbin/nginx -g 'daemon off;'
