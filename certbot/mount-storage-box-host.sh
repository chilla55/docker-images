#!/bin/bash
# ============================================================================
# Mount Hetzner Storage Box on Docker Host
# ============================================================================
# Purpose: Mount Storage Box on host so containers can bind-mount it
# Run on: Docker host (srv2)
# Usage: ./mount-storage-box-host.sh <password>
# ============================================================================

set -e

# Configuration
STORAGE_BOX_HOST="u515899.your-storagebox.de"
STORAGE_BOX_USER="u515899"
STORAGE_BOX_PATH="/backup"
HOST_MOUNT_POINT="/mnt/storagebox"
PASSWORD="${1:-}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $1"
}

log_error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] [ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] [WARN]${NC} $1"
}

# ──────────────────────────────────────────────────────────────────────────
# Validation
# ──────────────────────────────────────────────────────────────────────────

if [ -z "$PASSWORD" ]; then
    log_error "Password required"
    echo "Usage: $0 <storage_box_password>"
    exit 1
fi

if ! command -v mount &> /dev/null; then
    log_error "mount command not found"
    exit 1
fi

# ──────────────────────────────────────────────────────────────────────────
# Setup
# ──────────────────────────────────────────────────────────────────────────

log "Storage Box Host Mount Setup"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log "Host: $STORAGE_BOX_HOST"
log "User: $STORAGE_BOX_USER"
log "Path: $STORAGE_BOX_PATH"
log "Mount point: $HOST_MOUNT_POINT"
log "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# Create mount point
if [ ! -d "$HOST_MOUNT_POINT" ]; then
    log "Creating mount point: $HOST_MOUNT_POINT"
    mkdir -p "$HOST_MOUNT_POINT"
fi

# ──────────────────────────────────────────────────────────────────────────
# Check if already mounted
# ──────────────────────────────────────────────────────────────────────────

if mount | grep -q "on $HOST_MOUNT_POINT type"; then
    log "Already mounted at $HOST_MOUNT_POINT"
    mount | grep "on $HOST_MOUNT_POINT type"
    exit 0
fi

# ──────────────────────────────────────────────────────────────────────────
# Try CIFS mount (SMB3 fallback available)
# ============================================================================

log "Attempting CIFS mount..."

MOUNT_OPTIONS="vers=3.0,seal,nodfs,noserverino,nounix,uid=0,gid=0,file_mode=0755,dir_mode=0755"
REMOTE_PATH="//${STORAGE_BOX_HOST}${STORAGE_BOX_PATH}"

if mount -t cifs "$REMOTE_PATH" "$HOST_MOUNT_POINT" \
    -o "user=${STORAGE_BOX_USER},password=${PASSWORD},${MOUNT_OPTIONS}"; then
    log "✓ Successfully mounted Storage Box via CIFS"
    mount | grep "on $HOST_MOUNT_POINT type"
    exit 0
fi

log_warn "CIFS mount failed, trying SMB3..."

if mount -t smb3 "$REMOTE_PATH" "$HOST_MOUNT_POINT" \
    -o "username=${STORAGE_BOX_USER},password=${PASSWORD},${MOUNT_OPTIONS}"; then
    log "✓ Successfully mounted Storage Box via SMB3"
    mount | grep "on $HOST_MOUNT_POINT type"
    exit 0
fi

log_error "Failed to mount Storage Box (CIFS and SMB3 both failed)"
log_error "Check:"
log_error "  1. Network connectivity: ping $STORAGE_BOX_HOST"
log_error "  2. Credentials: user=$STORAGE_BOX_USER"
log_error "  3. Kernel modules: modprobe cifs cifs_arc4_transform"
log_error "  4. Host dmesg: dmesg | tail -30"

exit 1
