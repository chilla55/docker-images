#!/bin/bash
# ============================================================================
# Deploy NGINX Site Configurations to Storagebox
# ============================================================================
# Copies site configurations from repository to storagebox for nginx container
# ============================================================================

set -eu

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SITES_SOURCE="${SCRIPT_DIR}/sites-available"
STORAGEBOX_SITES="/mnt/storagebox/sites"

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

# Check if storagebox is mounted
if [ ! -d "/mnt/storagebox" ]; then
    log "ERROR: Storagebox not mounted at /mnt/storagebox"
    log "Please mount storagebox first"
    exit 1
fi

# Create sites directory on storagebox if it doesn't exist
if [ ! -d "$STORAGEBOX_SITES" ]; then
    log "Creating sites directory on storagebox: $STORAGEBOX_SITES"
    mkdir -p "$STORAGEBOX_SITES"
fi

# Check if sites-available directory exists
if [ ! -d "$SITES_SOURCE" ]; then
    log "ERROR: Source directory not found: $SITES_SOURCE"
    exit 1
fi

# Count available site configurations
SITE_COUNT=$(find "$SITES_SOURCE" -maxdepth 1 -type f -name "*.conf" | wc -l)

if [ "$SITE_COUNT" -eq 0 ]; then
    log "WARNING: No site configurations found in $SITES_SOURCE"
    exit 0
fi

log "Found $SITE_COUNT site configuration(s) to deploy"
log "Source: $SITES_SOURCE"
log "Destination: $STORAGEBOX_SITES"
echo ""

# Deploy each site configuration
for site_conf in "$SITES_SOURCE"/*.conf; do
    [ -f "$site_conf" ] || continue
    
    site_name=$(basename "$site_conf")
    dest_path="$STORAGEBOX_SITES/$site_name"
    
    log "Deploying: $site_name"
    
    # Copy the file
    cp "$site_conf" "$dest_path"
    chmod 644 "$dest_path"
    
    log "  ✓ Deployed to $dest_path"
done

echo ""
log "=========================================="
log "Deployment Complete!"
log "=========================================="
log ""
log "Next steps:"
log "1. The nginx container will detect these files automatically"
log "2. The watch-sites-reload.sh script checks every 30 seconds"
log "3. When upstreams are resolvable, sites will be auto-enabled"
log ""
log "To monitor the process:"
log "  docker service logs -f nginx_nginx"
log ""
log "To manually verify:"
log "  ls -la $STORAGEBOX_SITES"

