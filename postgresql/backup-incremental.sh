#!/bin/bash
set -e

BACKUP_DIR="/backups"
BACKUP_DATE=$(date +%Y%m%d_%H%M%S)
INCREMENTAL_DIR="${BACKUP_DIR}/incremental/${BACKUP_DATE}"
POSTGRES_PASSWORD=$(cat /run/secrets/postgres_password)

echo "[$(date)] Starting INCREMENTAL backup (WAL archive)"

# Check if there's a full backup
if [ ! -d "${BACKUP_DIR}/latest-full" ]; then
    echo "[$(date)] ERROR: No full backup found. Running full backup first."
    /usr/local/bin/backup-full.sh
    exit 0
fi

# Create incremental backup directory
mkdir -p "${INCREMENTAL_DIR}"

# Archive WAL files (incremental data)
# PostgreSQL WAL archiving should be enabled in postgresql.conf
rsync -a /var/lib/postgresql/data/pg_wal/ "${INCREMENTAL_DIR}/" --exclude "archive_status"

# Create marker file
echo "${BACKUP_DATE}" > "${INCREMENTAL_DIR}/backup_type.txt"
echo "incremental" >> "${INCREMENTAL_DIR}/backup_type.txt"

# Compress
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
