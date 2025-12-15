#!/bin/bash
set -e

BACKUP_DIR="/backups"
BACKUP_DATE=$(date +%Y%m%d_%H%M%S)
FULL_BACKUP_DIR="${BACKUP_DIR}/full/${BACKUP_DATE}"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
POSTGRES_PASSWORD=$(cat /run/secrets/postgres_password)

echo "[$(date)] Starting FULL backup"

# Create backup directory
mkdir -p "${FULL_BACKUP_DIR}"

# Perform full backup using pg_basebackup
PGPASSWORD="${POSTGRES_PASSWORD}" pg_basebackup \
    -h localhost \
    -U postgres \
    -D "${FULL_BACKUP_DIR}" \
    -Ft \
    -z \
    -P \
    -X stream

# Create marker file
echo "${BACKUP_DATE}" > "${FULL_BACKUP_DIR}/backup_type.txt"
echo "full" >> "${FULL_BACKUP_DIR}/backup_type.txt"

# Create symbolic link to latest
ln -sfn "${FULL_BACKUP_DIR}" "${BACKUP_DIR}/latest-full"

# Compress with additional gzip
cd "${BACKUP_DIR}/full"
tar -czf "${BACKUP_DATE}.tar.gz" "${BACKUP_DATE}"
rm -rf "${BACKUP_DATE}"

echo "[$(date)] Full backup completed: ${BACKUP_DATE}.tar.gz"

# Clean up old backups
find "${BACKUP_DIR}/full" -name "*.tar.gz" -mtime +${RETENTION_DAYS} -delete
find "${BACKUP_DIR}/incremental" -type d -mtime +${RETENTION_DAYS} -exec rm -rf {} + 2>/dev/null || true

echo "[$(date)] Backup rotation completed (retention: ${RETENTION_DAYS} days)"

# Verify backup
if [ -f "${BACKUP_DIR}/full/${BACKUP_DATE}.tar.gz" ]; then
    BACKUP_SIZE=$(du -h "${BACKUP_DIR}/full/${BACKUP_DATE}.tar.gz" | cut -f1)
    echo "[$(date)] Backup size: ${BACKUP_SIZE}"
    exit 0
else
    echo "[$(date)] ERROR: Backup file not found!"
    exit 1
fi
