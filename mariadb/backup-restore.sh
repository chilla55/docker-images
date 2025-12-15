#!/bin/bash
set -e

BACKUP_DIR="/backups"
RESTORE_DIR="/var/lib/mysql"
MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password)

echo "==================================="
echo "MariaDB Backup Restore Utility"
echo "==================================="
echo ""

# Function to list available backups
list_backups() {
    echo "Available Full Backups:"
    find "${BACKUP_DIR}/full" -name "*.tar.gz" -type f | sort -r | nl
    echo ""
    echo "Available Incremental Backups:"
    find "${BACKUP_DIR}/incremental" -name "*.tar.gz" -type f | sort -r | nl
    echo ""
}

# Function to restore from backup
restore_backup() {
    local FULL_BACKUP=$1
    local RESTORE_TEMP="/tmp/restore"
    
    echo "[$(date)] Starting restore process"
    echo "[$(date)] Full backup: ${FULL_BACKUP}"
    
    # Stop MariaDB
    echo "[$(date)] Stopping MariaDB..."
    mysqladmin -u root -p"${MYSQL_ROOT_PASSWORD}" shutdown || true
    sleep 5
    
    # Backup current data (just in case)
    if [ -d "${RESTORE_DIR}" ]; then
        echo "[$(date)] Backing up current data to ${RESTORE_DIR}.backup"
        mv "${RESTORE_DIR}" "${RESTORE_DIR}.backup.$(date +%Y%m%d_%H%M%S)"
    fi
    
    # Extract full backup
    mkdir -p "${RESTORE_TEMP}/full"
    echo "[$(date)] Extracting full backup..."
    tar -xzf "${FULL_BACKUP}" -C "${RESTORE_TEMP}/full"
    FULL_DIR=$(find "${RESTORE_TEMP}/full" -maxdepth 1 -type d -name "20*" | head -1)
    
    # Apply incremental backups in order
    echo "[$(date)] Looking for incremental backups..."
    FULL_DATE=$(basename "${FULL_BACKUP}" .tar.gz)
    INCREMENTALS=$(find "${BACKUP_DIR}/incremental" -name "*.tar.gz" -newer "${FULL_BACKUP}" | sort)
    
    for INCR in ${INCREMENTALS}; do
        echo "[$(date)] Applying incremental: $(basename ${INCR})"
        mkdir -p "${RESTORE_TEMP}/incr"
        tar -xzf "${INCR}" -C "${RESTORE_TEMP}/incr"
        INCR_DIR=$(find "${RESTORE_TEMP}/incr" -maxdepth 1 -type d -name "20*" | head -1)
        
        # Apply incremental to base
        mariabackup --prepare \
            --target-dir="${FULL_DIR}" \
            --incremental-dir="${INCR_DIR}"
        
        rm -rf "${RESTORE_TEMP}/incr"
    done
    
    # Final prepare
    echo "[$(date)] Final preparation of backup..."
    mariabackup --prepare --target-dir="${FULL_DIR}"
    
    # Copy to data directory
    echo "[$(date)] Copying data to ${RESTORE_DIR}..."
    mariabackup --copy-back --target-dir="${FULL_DIR}"
    
    # Fix permissions
    chown -R mysql:mysql "${RESTORE_DIR}"
    
    # Cleanup
    rm -rf "${RESTORE_TEMP}"
    
    echo "[$(date)] Restore completed successfully!"
    echo "[$(date)] Start MariaDB to complete recovery"
    echo ""
    echo "To start MariaDB, run: mysqld"
}

# Main menu
if [ $# -eq 0 ]; then
    list_backups
    echo "Usage: $0 <full-backup-path>"
    echo "Example: $0 /backups/full/20250115_020000.tar.gz"
    exit 0
fi

restore_backup "$1"
