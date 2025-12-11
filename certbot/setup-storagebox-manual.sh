#!/bin/bash
# ============================================================================
# Manual Storage Box fstab Setup
# ============================================================================
# Usage: ./setup-storagebox-manual.sh <hostname> <username> <password>
# Example: ./setup-storagebox-manual.sh srv2 u515899-sub1.your-storagebox.de u515899-sub1
# ============================================================================

set -e

if [ $# -ne 3 ]; then
    echo "Usage: $0 <hostname> <storagebox_host> <storagebox_user>"
    echo ""
    echo "Example:"
    echo "  $0 srv2 u515899-sub1.your-storagebox.de u515899-sub1"
    echo ""
    echo "This will:"
    echo "  1. Prompt for password (won't be echoed)"
    echo "  2. Create /root/.storagebox-creds on the server"
    echo "  3. Add fstab entry"
    echo "  4. Mount via 'mount -a'"
    exit 1
fi

HOSTNAME="$1"
STORAGE_BOX_HOST="$2"
STORAGE_BOX_USER="$3"
MOUNT_POINT="/mnt/storagebox"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} $1"
}

log_error() {
    echo -e "${RED}[$(date '+%H:%M:%S')] [ERROR]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[$(date '+%H:%M:%S')] [WARN]${NC} $1"
}

echo "Storage Box Manual Setup"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Server: $HOSTNAME.chilla55.de"
echo "Storage Box Host: $STORAGE_BOX_HOST"
echo "Storage Box User: $STORAGE_BOX_USER"
echo "Mount Point: $MOUNT_POINT"
echo ""

# Get password
read -sp "Enter Storage Box password: " PASSWORD
echo ""

# Step 1: Create mount point
log "Creating mount point..."
ssh root@${HOSTNAME}.chilla55.de "mkdir -p ${MOUNT_POINT}" || {
    log_error "Failed to create mount point"
    exit 1
}

# Step 2: Create credentials file
log "Creating credentials file..."
ssh root@${HOSTNAME}.chilla55.de "cat > /root/.storagebox-creds << 'EOF'
username=${STORAGE_BOX_USER}
password=${PASSWORD}
EOF
chmod 600 /root/.storagebox-creds" || {
    log_error "Failed to create credentials file"
    exit 1
}

# Step 3: Check if already in fstab
if ssh root@${HOSTNAME}.chilla55.de "grep -q 'storagebox' /etc/fstab" 2>/dev/null; then
    log_warn "Entry already in fstab, skipping..."
else
    # Step 3: Add to fstab
    log "Adding to fstab..."
    ssh root@${HOSTNAME}.chilla55.de "cat >> /etc/fstab << 'EOF'

# Hetzner Storage Box (${STORAGE_BOX_USER})
//${STORAGE_BOX_HOST}/${STORAGE_BOX_USER} ${MOUNT_POINT} smb3 credentials=/root/.storagebox-creds,vers=3.0,seal,nodfs,noserverino,nounix,uid=0,gid=0,file_mode=0755,dir_mode=0755,x-systemd.automount 0 0
EOF" || {
        log_error "Failed to update fstab"
        exit 1
    }
fi

# Step 4: Mount
log "Mounting..."
ssh root@${HOSTNAME}.chilla55.de "mount -a" 2>&1 | grep -v "^$" || true

# Step 5: Verify
log "Verifying..."
if ssh root@${HOSTNAME}.chilla55.de "mount | grep -q '${MOUNT_POINT}'" 2>/dev/null; then
    log "✓ Successfully mounted on ${HOSTNAME}"
    ssh root@${HOSTNAME}.chilla55.de "mount | grep '${MOUNT_POINT}'" | sed 's/^/  /'
else
    log_warn "Mount may have failed - check manually:"
    echo "  ssh root@${HOSTNAME}.chilla55.de 'mount | grep storagebox'"
    echo "  ssh root@${HOSTNAME}.chilla55.de 'dmesg | tail -20'"
fi

echo ""
log "✓ Setup complete for ${HOSTNAME}"
