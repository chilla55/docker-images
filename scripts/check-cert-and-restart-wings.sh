#!/bin/bash
# ============================================================================
# Certificate Change Monitor and Wings Restart Script
# ============================================================================
# This script:
# 1. Monitors certificate changes in /mnt/storagebox/certs/live/chilla55.de/
# 2. Restarts pterodactyl wings service when certificates are renewed
# 3. Keeps track of last known certificate modification time
# ============================================================================
# Usage: Run via cron every 5-15 minutes
# */15 * * * * /root/check-cert-and-restart-wings.sh >> /var/log/cert-wings-restart.log 2>&1
# ============================================================================

set -e

# ──────────────────────────────────────────────────────────────────────────
# Configuration
# ──────────────────────────────────────────────────────────────────────────
CERT_DIR="/mnt/storagebox/certs/live/chilla55.de"
CERT_FILE="$CERT_DIR/fullchain.pem"
STATE_FILE="/var/run/cert-wings-last-check"
WINGS_SERVICE="wings"
LOG_TAG="cert-wings-monitor"

# ──────────────────────────────────────────────────────────────────────────
# Logging
# ──────────────────────────────────────────────────────────────────────────
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] [$LOG_TAG] $*"
}

log_error() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] [$LOG_TAG] [ERROR] $*" >&2
}

# ──────────────────────────────────────────────────────────────────────────
# Main Logic
# ──────────────────────────────────────────────────────────────────────────

# Check if certificate file exists
if [ ! -f "$CERT_FILE" ]; then
    log_error "Certificate file not found: $CERT_FILE"
    exit 1
fi

# Get current certificate modification time
CURRENT_MTIME=$(stat -c %Y "$CERT_FILE" 2>/dev/null || stat -f %m "$CERT_FILE" 2>/dev/null)

if [ -z "$CURRENT_MTIME" ]; then
    log_error "Failed to get modification time of $CERT_FILE"
    exit 1
fi

# Check if this is the first run
if [ ! -f "$STATE_FILE" ]; then
    log "First run - initializing state file with current cert mtime: $CURRENT_MTIME"
    echo "$CURRENT_MTIME" > "$STATE_FILE"
    exit 0
fi

# Read last known modification time
LAST_MTIME=$(cat "$STATE_FILE")

# Compare modification times
if [ "$CURRENT_MTIME" != "$LAST_MTIME" ]; then
    log "Certificate change detected!"
    log "Last mtime: $LAST_MTIME"
    log "Current mtime: $CURRENT_MTIME"
    
    # Get certificate expiry for logging
    EXPIRY=$(openssl x509 -in "$CERT_FILE" -noout -enddate 2>/dev/null | cut -d= -f2 || echo "unknown")
    log "New certificate expires: $EXPIRY"
    
    # Restart wings service
    log "Restarting $WINGS_SERVICE service..."
    
    if systemctl is-active --quiet "$WINGS_SERVICE"; then
        systemctl restart "$WINGS_SERVICE"
        if [ $? -eq 0 ]; then
            log "✓ Successfully restarted $WINGS_SERVICE service"
            
            # Wait a moment and verify it's running
            sleep 2
            if systemctl is-active --quiet "$WINGS_SERVICE"; then
                log "✓ $WINGS_SERVICE service is running"
            else
                log_error "$WINGS_SERVICE service failed to start after restart"
                exit 1
            fi
        else
            log_error "Failed to restart $WINGS_SERVICE service"
            exit 1
        fi
    else
        log_error "$WINGS_SERVICE service is not running, attempting to start..."
        systemctl start "$WINGS_SERVICE"
        if [ $? -eq 0 ]; then
            log "✓ Successfully started $WINGS_SERVICE service"
        else
            log_error "Failed to start $WINGS_SERVICE service"
            exit 1
        fi
    fi
    
    # Update state file with new modification time
    echo "$CURRENT_MTIME" > "$STATE_FILE"
    log "State file updated with new mtime: $CURRENT_MTIME"
else
    log "No certificate changes detected (mtime: $CURRENT_MTIME)"
fi

exit 0
