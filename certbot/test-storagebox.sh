#!/bin/bash
# ============================================================================
# Helper Script to Test CIFS Connection to Hetzner Storage Box
# ============================================================================
# Usage: ./test-storagebox.sh
# ============================================================================

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $*"
}

# Check if running as root (required for mount)
if [ "$EUID" -ne 0 ]; then
    log_error "This script must be run as root (for mounting)"
    echo "Please run: sudo $0"
    exit 1
fi

# Prompt for Storage Box details
echo "=========================================="
echo "Hetzner Storage Box CIFS Connection Test"
echo "=========================================="
echo ""

read -p "Storage Box hostname (e.g., u123456.your-storagebox.de): " STORAGE_HOST
read -p "Storage Box username (e.g., u123456): " STORAGE_USER
read -s -p "Storage Box password: " STORAGE_PASS
echo ""
read -p "Storage Box path (default: /backup): " STORAGE_PATH
STORAGE_PATH=${STORAGE_PATH:-/backup}

MOUNT_POINT="/mnt/storagebox-test"
REMOTE_PATH="//${STORAGE_HOST}${STORAGE_PATH}"

log_info "Testing connection to Storage Box..."
log_info "Remote: $REMOTE_PATH"
echo ""

# Check if cifs-utils is installed
if ! command -v mount.cifs &> /dev/null; then
    log_error "cifs-utils not installed!"
    echo "Install with: apt-get install cifs-utils (Debian/Ubuntu)"
    echo "            or: apk add cifs-utils (Alpine)"
    exit 1
fi

# Create mount point
mkdir -p "$MOUNT_POINT"

# Try to mount
log_info "Attempting to mount..."
if mount -t cifs "$REMOTE_PATH" "$MOUNT_POINT" \
    -o "username=${STORAGE_USER},password=${STORAGE_PASS},vers=3.0"; then
    
    log_info "✓ Successfully mounted Storage Box!"
    echo ""
    
    # List contents
    log_info "Contents of Storage Box:"
    ls -lah "$MOUNT_POINT" | head -n 20
    echo ""
    
    # Test write permissions
    TEST_FILE="$MOUNT_POINT/test-$(date +%s).txt"
    log_info "Testing write permissions..."
    
    if echo "test" > "$TEST_FILE" 2>/dev/null; then
        log_info "✓ Write test successful!"
        rm -f "$TEST_FILE"
    else
        log_warn "✗ Write test failed (may be read-only or permission issue)"
    fi
    
    # Unmount
    log_info "Unmounting..."
    umount "$MOUNT_POINT"
    rmdir "$MOUNT_POINT"
    
    echo ""
    log_info "=========================================="
    log_info "✓ All tests passed!"
    log_info "=========================================="
    echo ""
    log_info "Your Storage Box configuration:"
    echo "  STORAGE_BOX_HOST: $STORAGE_HOST"
    echo "  STORAGE_BOX_USER: $STORAGE_USER"
    echo "  STORAGE_BOX_PATH: $STORAGE_PATH"
    echo ""
    log_info "Add these to docker-compose.swarm.yml and create the password secret:"
    echo "  echo 'your-password' > storagebox.txt"
    echo "  docker secret create storagebox_password storagebox.txt"
    echo "  rm storagebox.txt  # Remove after creating secret"
    
else
    log_error "✗ Failed to mount Storage Box"
    rmdir "$MOUNT_POINT" 2>/dev/null || true
    echo ""
    log_info "Troubleshooting:"
    echo "  1. Verify hostname: $STORAGE_HOST"
    echo "  2. Verify username: $STORAGE_USER"
    echo "  3. Verify password is correct"
    echo "  4. Enable Samba/CIFS in Hetzner Robot:"
    echo "     https://robot.hetzner.com/storage"
    echo "  5. Check firewall allows SMB traffic"
    exit 1
fi
