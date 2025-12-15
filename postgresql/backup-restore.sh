#!/bin/bash
set -e

BACKUP_DIR="/backups"
RESTORE_DIR="/var/lib/postgresql/data"
POSTGRES_PASSWORD=$(cat /run/secrets/postgres_password)

echo "==================================="
echo "PostgreSQL Backup Restore Utility"
echo "==================================="
echo ""

# Function to list backups
list_backups() {
    echo "Available Full Backups:"
    find "${BACKUP_DIR}/full" -name "*.tar.gz" -type f | sort -r | nl
    echo ""
    echo "Available Incremental Backups:"
    find "${BACKUP_DIR}/incremental" -name "*.tar.gz" -type f | sort -r | nl
    echo ""
}

# Function to restore
restore_backup() {
    local FULL_BACKUP=$1
    local RESTORE_TEMP="/tmp/restore"
    
    echo "[$(date)] Starting restore process"
    echo "[$(date)] Full backup: ${FULL_BACKUP}"
    
    # Stop PostgreSQL
    echo "[$(date)] Stopping PostgreSQL..."
    pg_ctl stop -D "${RESTORE_DIR}" -m fast || true
    sleep 5
    
    # Backup current data
    if [ -d "${RESTORE_DIR}" ]; then
        echo "[$(date)] Backing up current data"
        mv "${RESTORE_DIR}" "${RESTORE_DIR}.backup.$(date +%Y%m%d_%H%M%S)"
    fi
    
    # Extract full backup
    mkdir -p "${RESTORE_TEMP}"
    echo "[$(date)] Extracting full backup..."
    tar -xzf "${FULL_BACKUP}" -C "${RESTORE_TEMP}"
    
    # Find base.tar.gz and extract
    BASE_TAR=$(find "${RESTORE_TEMP}" -name "base.tar.gz" | head -1)
    if [ -n "${BASE_TAR}" ]; then
        mkdir -p "${RESTORE_DIR}"
        tar -xzf "${BASE_TAR}" -C "${RESTORE_DIR}"
    fi
    
    # Apply WAL files if any
    FULL_DATE=$(basename "${FULL_BACKUP}" .tar.gz)
    INCREMENTALS=$(find "${BACKUP_DIR}/incremental" -name "*.tar.gz" -newer "${FULL_BACKUP}" | sort)
    
    if [ -n "${INCREMENTALS}" ]; then
        echo "[$(date)] Applying WAL files..."
        mkdir -p "${RESTORE_DIR}/pg_wal"
        for INCR in ${INCREMENTALS}; do
            echo "[$(date)] Applying: $(basename ${INCR})"
            tar -xzf "${INCR}" -C /tmp/
            INCR_DIR=$(find /tmp -maxdepth 1 -type d -name "20*" | head -1)
            rsync -a "${INCR_DIR}/" "${RESTORE_DIR}/pg_wal/"
            rm -rf "${INCR_DIR}"
        done
    fi
    
    # Fix permissions
    chown -R postgres:postgres "${RESTORE_DIR}"
    chmod 700 "${RESTORE_DIR}"
    
    # Cleanup
    rm -rf "${RESTORE_TEMP}"
    
    echo "[$(date)] Restore completed successfully!"
    echo "[$(date)] Start PostgreSQL to complete recovery"
    echo ""
    echo "To start PostgreSQL, run: pg_ctl start -D ${RESTORE_DIR}"
}

# Main
if [ $# -eq 0 ]; then
    list_backups
    echo "Usage: $0 <full-backup-path>"
    echo "Example: $0 /backups/full/20250115_030000.tar.gz"
    exit 0
fi

restore_backup "$1"
