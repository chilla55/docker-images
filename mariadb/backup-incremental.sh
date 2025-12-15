#!/bin/bash
set -e

BACKUP_DIR="/backups"
BACKUP_DATE=$(date +%Y%m%d_%H%M%S)
INCREMENTAL_BASE="${BACKUP_DIR}/latest-full"
INCREMENTAL_DIR="${BACKUP_DIR}/incremental/${BACKUP_DATE}"
MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password)

echo "[$(date)] Starting INCREMENTAL backup"

# Check if there's a full backup to base on
if [ ! -d "${INCREMENTAL_BASE}" ]; then
    echo "[$(date)] ERROR: No full backup found. Running full backup first."
    /usr/local/bin/backup-full.sh
    exit 0
fi

# Find the latest incremental or use full as base
LAST_BACKUP="${INCREMENTAL_BASE}"
LATEST_INCREMENTAL=$(find "${BACKUP_DIR}/incremental" -maxdepth 1 -type d -name "20*" 2>/dev/null | sort -r | head -1)
if [ -n "${LATEST_INCREMENTAL}" ]; then
    LAST_BACKUP="${LATEST_INCREMENTAL}"
fi

# Create incremental backup directory
mkdir -p "${INCREMENTAL_DIR}"

# Perform incremental backup
mariabackup --backup \
    --target-dir="${INCREMENTAL_DIR}" \
    --incremental-basedir="${LAST_BACKUP}" \
    --user=root \
    --password="${MYSQL_ROOT_PASSWORD}" \
    --host=localhost

# Create marker file
echo "${BACKUP_DATE}" > "${INCREMENTAL_DIR}/backup_type.txt"
echo "incremental" >> "${INCREMENTAL_DIR}/backup_type.txt"
echo "base: $(basename ${LAST_BACKUP})" >> "${INCREMENTAL_DIR}/backup_type.txt"

# Compress the incremental backup
cd "${BACKUP_DIR}/incremental"
tar -czf "${BACKUP_DATE}.tar.gz" "${BACKUP_DATE}"
rm -rf "${BACKUP_DATE}"

echo "[$(date)] Incremental backup completed: ${BACKUP_DATE}.tar.gz"

# Verify
if [ -f "${BACKUP_DIR}/incremental/${BACKUP_DATE}.tar.gz" ]; then
    BACKUP_SIZE=$(du -h "${BACKUP_DIR}/incremental/${BACKUP_DATE}.tar.gz" | cut -f1)
    echo "[$(date)] Backup size: ${BACKUP_SIZE}"
    exit 0
else
    echo "[$(date)] ERROR: Backup file not found!"
    exit 1
fi
