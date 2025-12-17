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

# Extract upstreams from nginx config (BusyBox compatible)
extract_upstreams() {
  local config_file="$1"
  # Extract hostnames from proxy_pass http://hostname or upstream "hostname"
  grep -E 'proxy_pass http://|upstream "' "$config_file" 2>/dev/null | \
    sed -E 's/.*proxy_pass http:\/\/([^/:;]+).*/\1/; s/.*upstream "([^"]+)".*/\1/' | \
    sort -u
}

# Source directory where all site configs are stored (before symlinking)
SITES_SOURCE_DIR="${SITES_SOURCE_DIR:-/etc/nginx/sites-available}"

if [ ! -d "$SITES_PATH" ]; then
  log "ERROR: Sites directory does not exist: $SITES_PATH"
  exit 1
fi

log "Watching $SITES_PATH every ${INTERVAL}s for changes"
if [ -d "$SITES_SOURCE_DIR" ]; then
  log "Will check for sites to enable in $SITES_SOURCE_DIR"
fi

# Initialize with current mtimes
update_mtimes() {
  local current_mtimes
  current_mtimes=$(find "$SITES_PATH" -type f -o -type l 2>/dev/null | xargs -r stat -c '%Y:%n' 2>/dev/null | sort || echo "")
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
  
  # Check already enabled sites to see if upstreams are still available
  if [ -d "$SITES_PATH" ] && [ "$(ls -A $SITES_PATH 2>/dev/null)" ]; then
    for enabled_site in "$SITES_PATH"/*; do
      [ -f "$enabled_site" ] || [ -L "$enabled_site" ] || continue
      
      site_name=$(basename "$enabled_site")
      
      # Extract upstream hostnames and check if they're still resolvable
      upstreams=$(extract_upstreams "$enabled_site")
      
      if [ -n "$upstreams" ]; then
        for upstream in $upstreams; do
          if ! getent hosts "$upstream" >/dev/null 2>&1; then
            log "WARNING: Enabled site $site_name has unresolvable upstream '$upstream'"
            log "Site will be disabled and retried when upstream becomes available"
            rm -f "$enabled_site"
            
            # Try reload after removal
            if nginx -t 2>&1 | grep -q "successful"; then
              log "Config OK after removing $site_name, reloading nginx"
              nginx -s reload 2>/dev/null || kill -HUP 1 2>/dev/null
            fi
            break
          fi
        done
      fi
    done
  fi
  
  # Check for sites in source that aren't symlinked to sites-enabled
  if [ -d "$SITES_SOURCE_DIR" ] && [ "$(ls -A $SITES_SOURCE_DIR 2>/dev/null)" ]; then
    for source_site in "$SITES_SOURCE_DIR"/*; do
      [ -f "$source_site" ] || continue
      
      site_name=$(basename "$source_site")
      enabled_site="$SITES_PATH/$site_name"
      
      # Skip if already enabled
      [ -e "$enabled_site" ] && continue
      
      debug "Found unlinked site: $site_name"
      
      # Extract upstream hostnames from the config and check if they're resolvable
      upstreams=$(extract_upstreams "$source_site")
      
      can_resolve=true
      if [ -n "$upstreams" ]; then
        for upstream in $upstreams; do
          debug "Checking if upstream '$upstream' is resolvable..."
          if ! getent hosts "$upstream" >/dev/null 2>&1; then
            debug "Upstream '$upstream' is not resolvable yet"
            can_resolve=false
            break
          fi
        done
      fi
      
      if [ "$can_resolve" = "false" ]; then
        debug "Site $site_name has unresolvable upstreams, skipping for now"
        continue
      fi
      
      log "Upstreams for $site_name are resolvable, testing configuration..."
      
      # Create symlink to test
      ln -sf "$source_site" "$enabled_site"
      
      # Test if it works now
      if nginx -t 2>&1 | grep -q "successful"; then
        log "Site $site_name works! Enabling..."
        
        # Reload nginx
        if nginx -s reload 2>/dev/null; then
          log "Successfully reloaded nginx with $site_name"
        else
          kill -HUP 1 2>/dev/null
          log "Sent HUP signal to nginx for $site_name"
        fi
      else
        # Still doesn't work, remove the symlink
        log "Site $site_name config test failed, keeping unlinked"
        rm -f "$enabled_site"
      fi
    done
  fi
  
  # Check for configuration changes in sites-enabled
  CURRENT_STATE="$(update_mtimes)"
  
  if [ "$CURRENT_STATE" != "$LAST_STATE" ]; then
    log "Site configuration change detected"
    debug "Old state: $LAST_STATE"
    debug "New state: $CURRENT_STATE"
    
    # Update Cloudflare IPs before testing config
    if [ -x /usr/local/bin/update-cf-ips.sh ]; then
      debug "Updating Cloudflare IP list before config test..."
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
        log "Upstream host not found - removing problematic site symlink"
        
        # Extract the problematic file and remove it
        problem_file=$(nginx -t 2>&1 | grep -oP 'in \K/[^ ]+\.conf' | head -1)
        if [ -n "$problem_file" ] && [ -e "$problem_file" ]; then
          site_name=$(basename "$problem_file")
          log "Removing site symlink: $site_name (upstream not available)"
          rm -f "$problem_file"
          log "Site config remains in $SITES_SOURCE_DIR for automatic retry"
          
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
