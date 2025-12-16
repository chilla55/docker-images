#!/bin/bash
set -eu

CF_REALIP_STATE_DIR="${CF_REALIP_STATE_DIR:-/tmp/cf-realip}"
: "${CF_REALIP_AUTO:=1}"
: "${CF_REALIP_INTERVAL:=21600}"  # 6h
: "${CF_REALIP_STATUS:=${CF_REALIP_STATE_DIR}/cf-realip.status.json}"

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

# Trap ERR to catch unexpected exits
trap 'log "[entrypoint] TRAP: unexpected error on line $LINENO"; exit 1' ERR
trap cleanup TERM INT

# ──────────────────────────────────────────────────────────────────────────
# Setup
# ──────────────────────────────────────────────────────────────────────────

# Ensure state dir + status file exist before first run (keep outside volatile cache)
mkdir -p "$(dirname "$CF_REALIP_STATUS")"

# Create a minimal valid cloudflare_realip.conf if missing (prevents nginx startup failure)
CF_REALIP_CONF="${CF_REALIP_OUT:-${CF_REALIP_STATE_DIR}/cloudflare_realip.conf}"
CF_REALIP_ETAG="${CF_REALIP_ETAG:-${CF_REALIP_STATE_DIR}/cloudflare_ips.etag}"
if [ ! -f "$CF_REALIP_CONF" ] || [ ! -s "$CF_REALIP_CONF" ]; then
  mkdir -p "$(dirname "$CF_REALIP_CONF")"
  cat > "$CF_REALIP_CONF" << 'EOF'
# Cloudflare Real IP config - auto-generated
# Placeholder until update-cf-ips.sh populates this
real_ip_header CF-Connecting-IP;
real_ip_recursive on;
# IPs will be populated by update-cf-ips.sh
EOF
fi

# Initialize etag file if missing/empty
if [ ! -f "$CF_REALIP_ETAG" ] || [ ! -s "$CF_REALIP_ETAG" ]; then
  mkdir -p "$(dirname "$CF_REALIP_ETAG")"
  printf 'pending' > "$CF_REALIP_ETAG"
fi

# Initialize status file with valid JSON
if [ ! -f "$CF_REALIP_STATUS" ] || [ ! -s "$CF_REALIP_STATUS" ]; then
  printf '{"last_ok_iso":"","last_ok_ts":0,"last_error":"init","last_etag":"","consecutive_failures":0}\n' > "$CF_REALIP_STATUS"
fi

run_once() { /usr/local/bin/update-cf-ips.sh || echo "[cloudflare-realip] update failed"; }

# Cloudflare updater - RUN ONCE BEFORE NGINX STARTS
if [ "$CF_REALIP_AUTO" = "1" ] || [ "$CF_REALIP_AUTO" = "true" ]; then
  log "[entrypoint] Running Cloudflare IP update before nginx startup..."
  run_once
  
  # Wait for files to be valid (at least 50 bytes each)
  RETRY=0
  MAX_RETRIES=30
  while [ $RETRY -lt $MAX_RETRIES ]; do
    STATUS_SIZE=$(stat -c%s "$CF_REALIP_STATUS" 2>/dev/null || echo 0)
    ETAG_SIZE=$(stat -c%s "$CF_REALIP_ETAG" 2>/dev/null || echo 0)
    
    if [ "$STATUS_SIZE" -gt 50 ] && [ "$ETAG_SIZE" -gt 0 ]; then
      log "[entrypoint] Cloudflare files validated (status=$STATUS_SIZE bytes, etag=$ETAG_SIZE bytes)"
      break
    fi
    
    RETRY=$((RETRY + 1))
    if [ $RETRY -lt $MAX_RETRIES ]; then
      log "[entrypoint] Waiting for Cloudflare files to be ready (attempt $RETRY/$MAX_RETRIES)..."
      sleep 1
    fi
  done
  
  if [ $RETRY -ge $MAX_RETRIES ]; then
    log "[entrypoint] WARNING: Cloudflare files may not be ready, proceeding anyway"
  fi
  
  # Start background loop for periodic updates
  ( while :; do sleep "$CF_REALIP_INTERVAL"; run_once || true; done ) &
else
  echo "[cloudflare-realip] Auto-update disabled."
fi

# Cert watcher loop (only if path is set)
if [ -n "${CERT_WATCH_PATH:-}" ]; then
  echo "[entrypoint] Starting cert watcher for: $CERT_WATCH_PATH"
  /usr/local/bin/watch-cert-reload.sh >> /dev/stdout 2>> /dev/stderr &
  CERT_WATCHER_PID=$!
  echo "[entrypoint] Cert watcher PID: $CERT_WATCHER_PID"
else
  echo "[entrypoint] CERT_WATCH_PATH not set, cert watcher disabled."
fi

# Sites watcher loop (watches for site configuration changes)
if [ -n "${SITES_WATCH_PATH:-}" ]; then
  echo "[entrypoint] Starting sites watcher for: $SITES_WATCH_PATH"
  /usr/local/bin/watch-sites-reload.sh >> /dev/stdout 2>> /dev/stderr &
  SITES_WATCHER_PID=$!
  echo "[entrypoint] Sites watcher PID: $SITES_WATCHER_PID"
else
  echo "[entrypoint] SITES_WATCH_PATH not set, sites watcher disabled."
fi

# Ensure cache, log, and Cloudflare state dirs are writable
chown -R nginx:nginx /var/cache/nginx /etc/nginx/logs "$CF_REALIP_STATE_DIR" 2>/dev/null || true

# Validate nginx configuration before starting - with graceful degradation for upstream resolution failures
validate_and_fix_config() {
  local max_attempts=3
  local attempt=1
  
  while [ $attempt -le $max_attempts ]; do
    if nginx -t 2>&1; then
      log "[entrypoint] nginx config OK"
      return 0
    fi
    
    local error_output=$(nginx -t 2>&1)
    
    # Check if the error is about upstream host not found
    if echo "$error_output" | grep -q "host not found in upstream"; then
      log "[entrypoint] WARNING: Upstream resolution failure detected (attempt $attempt/$max_attempts)"
      
      if [ $attempt -eq $max_attempts ]; then
        log "[entrypoint] Attempting graceful degradation - temporarily removing problematic site symlinks"
        
        # Extract the problematic file from error message
        local problem_file=$(echo "$error_output" | grep -oP 'in \K/[^ ]+\.conf' | head -1)
        
        if [ -n "$problem_file" ] && [ -e "$problem_file" ]; then
          local site_name=$(basename "$problem_file")
          log "[entrypoint] Temporarily removing site link: $site_name"
          
          # Remove the symlink or file from sites-enabled
          rm -f "$problem_file"
          
          # Test again after removing
          if nginx -t 2>&1; then
            log "[entrypoint] nginx config OK after removing $site_name"
            log "[entrypoint] The sites watcher will recreate the link when the upstream service is available"
            return 0
          fi
        fi
        
        # If we still have issues, try removing all sites-enabled configs
        if [ -d "$SITES_WATCH_PATH" ] && [ "$(ls -A $SITES_WATCH_PATH 2>/dev/null)" ]; then
          log "[entrypoint] Removing all site links from sites-enabled"
          rm -f "$SITES_WATCH_PATH"/* 2>/dev/null || true
          
          if nginx -t 2>&1; then
            log "[entrypoint] nginx will start with default configuration only"
            log "[entrypoint] The sites watcher will recreate links when upstream services are available"
            return 0
          fi
        fi
      fi
      
      # Wait a bit before retry (upstream service might be starting)
      log "[entrypoint] Waiting 3 seconds before retry..."
      sleep 3
      attempt=$((attempt + 1))
    else
      # Different type of error - fail immediately
      log "[entrypoint] CRITICAL: nginx config has non-recoverable errors"
      echo "$error_output"
      return 1
    fi
  done
  
  log "[entrypoint] CRITICAL: Could not recover nginx configuration"
  nginx -t || true
  return 1
}

if ! validate_and_fix_config; then
  exit 1
fi

# Run nginx (PID 1) as root (safe in container due to Docker isolation)
log "[entrypoint] Starting nginx"
log "[entrypoint] Final checks before exec:"
log "  - Cert: ${CERT_WATCH_PATH:-none} exists: $([ -n "${CERT_WATCH_PATH:-}" ] && [ -f "$CERT_WATCH_PATH" ] && echo 'yes' || echo 'no/unset')"
log "  - Sites dir: $SITES_WATCH_PATH exists: $([ -d "$SITES_WATCH_PATH" ] && echo 'yes' || echo 'NO')"
log "  - CF status: $([ -f "$CF_REALIP_STATUS" ] && echo 'yes' || echo 'NO')"
exec /usr/sbin/nginx -g 'daemon off;' 2>&1
