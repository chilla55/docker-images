#!/bin/bash
set -eu

SITES_PATH="${SITES_WATCH_PATH:-/etc/nginx/sites-enabled}"
INTERVAL="${SITES_WATCH_INTERVAL:-30}"
DEBUG="${SITES_WATCH_DEBUG:-0}"

log() {
  echo "[sites-watch] $*"
}

debug() {
  [ "$DEBUG" = "1" ] && echo "[sites-watch-debug] $*"
}

if [ ! -d "$SITES_PATH" ]; then
  log "ERROR: Sites directory does not exist: $SITES_PATH"
  exit 1
fi

log "Watching $SITES_PATH every ${INTERVAL}s for changes"

# Track all file mtimes in the directory
declare -A last_mtimes

# Initialize with current mtimes
update_mtimes() {
  local current_mtimes
  current_mtimes=$(find "$SITES_PATH" -type f 2>/dev/null | xargs -I {} sh -c 'echo "$(stat -c %Y {}):$(basename {})"' | sort)
  echo "$current_mtimes"
}

LAST_STATE="$(update_mtimes)"
debug "Initial state: $LAST_STATE"

while :; do
  sleep "$INTERVAL"
  
  if [ ! -d "$SITES_PATH" ]; then
    log "ERROR: Sites directory was removed: $SITES_PATH"
    sleep 10
    continue
  fi
  
  CURRENT_STATE="$(update_mtimes)"
  
  if [ "$CURRENT_STATE" != "$LAST_STATE" ]; then
    log "Site configuration changed detected"
    debug "Old state: $LAST_STATE"
    debug "New state: $CURRENT_STATE"
    
    # Update Cloudflare IPs before testing config
    if [ -x /usr/local/bin/update-cf-ips.sh ]; then
      log "Updating Cloudflare IP list before config test..."
      /usr/local/bin/update-cf-ips.sh || log "WARNING: Cloudflare update failed, continuing anyway"
    fi
    
    # Validate nginx configuration
    if nginx -t 2>&1 | grep -q "successful"; then
      log "nginx config test OK, reloading..."
      
      if nginx -s reload 2>/dev/null; then
        log "Successfully reloaded nginx"
      else
        # Try HUP signal as fallback
        if kill -HUP 1 2>/dev/null; then
          log "Successfully sent HUP signal to nginx"
        else
          log "ERROR: Failed to reload nginx"
        fi
      fi
    else
      log "ERROR: nginx configuration test failed, NOT reloading"
      log "Checking if this is an upstream resolution issue..."
      
      # Check if the error is due to upstream resolution
      if nginx -t 2>&1 | grep -q "host not found in upstream"; then
        log "Upstream host not found - removing problematic site link"
        
        # Extract the problematic file and remove it
        problem_file=$(nginx -t 2>&1 | grep -oP 'in \K/[^ ]+\.conf' | head -1)
        if [ -n "$problem_file" ] && [ -e "$problem_file" ]; then
          site_name=$(basename "$problem_file")
          log "Removing site link: $site_name (upstream not available)"
          rm -f "$problem_file"
          
          # Try reload after removal
          if nginx -t 2>&1 | grep -q "successful"; then
            log "Config OK after removing $site_name, reloading nginx"
            nginx -s reload 2>/dev/null || kill -HUP 1 2>/dev/null
          fi
        fi
      else
        nginx -t 2>&1 | head -20
      fi
    fi
    
    LAST_STATE="$CURRENT_STATE"
  else
    debug "No changes detected"
  fi
done
