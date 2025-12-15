#!/bin/bash
set -e

BACKUP_DIR="/backups"
BACKUP_DATE=$(date +%Y%m%d_%H%M%S)
FULL_BACKUP_DIR="${BACKUP_DIR}/full"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password 2>/dev/null || echo "${MYSQL_ROOT_PASSWORD}")

echo "[$(date)] Starting FULL backup"

# Wait for MariaDB to be ready
until mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &>/dev/null; do
    echo "[$(date)] Waiting for MariaDB to be ready..."
    sleep 5
done

# Create backup directory
mkdir -p "${FULL_BACKUP_DIR}"

# Get list of all databases (excluding system tables)
DATABASES=$(mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SHOW DATABASES;" | grep -Ev "(Database|information_schema|performance_schema|mysql|sys)")

# Perform full backup of all databases
BACKUP_FILE="${FULL_BACKUP_DIR}/${BACKUP_DATE}_full.sql"
echo "[$(date)] Backing up databases: ${DATABASES}"

mysqldump -uroot -p"${MYSQL_ROOT_PASSWORD}" \
    --single-transaction \
    --routines \
    --triggers \
    --events \
    --flush-logs \
    --master-data=2 \
    --all-databases \
    --add-drop-database \
    --quick \
    --lock-tables=false \
    > "${BACKUP_FILE}"

# Compress backup
echo "[$(date)] Compressing backup..."
bzip2 -f "${BACKUP_FILE}"
BACKUP_FILE="${BACKUP_FILE}.bz2"

# Calculate checksum
sha256sum "${BACKUP_FILE}" > "${BACKUP_FILE}.sha256"

# Store metadata
cat > "${FULL_BACKUP_DIR}/${BACKUP_DATE}_metadata.txt" << EOF
backup_type=full
backup_date=${BACKUP_DATE}
databases=${DATABASES}
timestamp=$(date -Iseconds)
EOF

# Create symbolic link to latest
ln -sfn "${BACKUP_FILE}" "${FULL_BACKUP_DIR}/latest_full.sql.bz2"

# Update last backup state for incremental tracking
echo "${BACKUP_DATE}" > /var/lib/mysql-backup-state/last_full_backup
mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "FLUSH LOGS;" 2>/dev/null || true

echo "[$(date)] Full backup completed: ${BACKUP_FILE}"

# Clean up old backups
echo "[$(date)] Cleaning up backups older than ${RETENTION_DAYS} days"
find "${BACKUP_DIR}/full" -name "*_full.sql.bz2" -mtime +${RETENTION_DAYS} -delete
find "${BACKUP_DIR}/full" -name "*_metadata.txt" -mtime +${RETENTION_DAYS} -delete
find "${BACKUP_DIR}/full" -name "*.sha256" -mtime +${RETENTION_DAYS} -delete
find "${BACKUP_DIR}/incremental" -name "*.sql.bz2" -mtime +${RETENTION_DAYS} -delete 2>/dev/null || true

echo "[$(date)] Backup rotation completed"

# Verify backup
if [ -f "${BACKUP_FILE}" ]; then
    BACKUP_SIZE=$(du -h "${BACKUP_FILE}" | cut -f1)
    echo "[$(date)] Backup size: ${BACKUP_SIZE}"
    
    # Verify checksum
    if sha256sum -c "${BACKUP_FILE}.sha256" &>/dev/null; then
        echo "[$(date)] Checksum verified successfully"
    else
        echo "[$(date)] WARNING: Checksum verification failed!"
        exit 1
    fi
    
    exit 0
else
    echo "[$(date)] ERROR: Backup file not found!"
    exit 1
fi
