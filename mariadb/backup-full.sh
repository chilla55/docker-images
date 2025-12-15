#!/bin/bash
set -e

BACKUP_DIR="/backups"
BACKUP_DATE=$(date +%Y%m%d_%H%M%S)
FULL_BACKUP_DIR="${BACKUP_DIR}/full/${BACKUP_DATE}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password)

echo "[$(date)] Starting FULL backup"

# Create backup directory
mkdir -p "${FULL_BACKUP_DIR}"

# Perform full backup using mariabackup
mariabackup --backup \
    --target-dir="${FULL_BACKUP_DIR}" \
    --user=root \
    --password="${MYSQL_ROOT_PASSWORD}" \
    --host=localhost

# Prepare the backup (makes it consistent)
mariabackup --prepare \
    --target-dir="${FULL_BACKUP_DIR}"

# Create a marker file indicating this is a full backup
echo "${BACKUP_DATE}" > "${FULL_BACKUP_DIR}/backup_type.txt"
echo "full" >> "${FULL_BACKUP_DIR}/backup_type.txt"

# Create a symbolic link to latest full backup
ln -sfn "${FULL_BACKUP_DIR}" "${BACKUP_DIR}/latest-full"

# Compress the backup
cd "${BACKUP_DIR}/full"
tar -czf "${BACKUP_DATE}.tar.gz" "${BACKUP_DATE}"
rm -rf "${BACKUP_DATE}"

echo "[$(date)] Full backup completed: ${BACKUP_DATE}.tar.gz"

# Clean up old backups
find "${BACKUP_DIR}/full" -name "*.tar.gz" -mtime +${RETENTION_DAYS} -delete
find "${BACKUP_DIR}/incremental" -type d -mtime +${RETENTION_DAYS} -exec rm -rf {} + 2>/dev/null || true

echo "[$(date)] Backup rotation completed (retention: ${RETENTION_DAYS} days)"

# Verify backup integrity
if [ -f "${BACKUP_DIR}/full/${BACKUP_DATE}.tar.gz" ]; then
    BACKUP_SIZE=$(du -h "${BACKUP_DIR}/full/${BACKUP_DATE}.tar.gz" | cut -f1)
    echo "[$(date)] Backup size: ${BACKUP_SIZE}"
    exit 0
else
    echo "[$(date)] ERROR: Backup file not found!"
    exit 1
fi
