#!/bin/bash
set -e

BACKUP_DIR="/backups"
BACKUP_DATE=$(date +%Y%m%d_%H%M%S)
INCREMENTAL_DIR="${BACKUP_DIR}/incremental"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password 2>/dev/null || echo "${MYSQL_ROOT_PASSWORD}")
STATE_DIR="/var/lib/mysql-backup-state"

echo "[$(date)] Starting INCREMENTAL backup (change-detection)"

# Wait for MariaDB to be ready
until mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &>/dev/null; do
    echo "[$(date)] Waiting for MariaDB to be ready..."
    sleep 5
done

# Check if there's a full backup
if [ ! -f "${BACKUP_DIR}/full/latest_full.sql.bz2" ]; then
    echo "[$(date)] No full backup found. Skipping incremental backup."
    echo "[$(date)] Run full backup first or wait for scheduled full backup."
    exit 0
fi

# Create incremental backup directory
mkdir -p "${INCREMENTAL_DIR}"
mkdir -p "${STATE_DIR}"

# Calculate current database state checksum
CURRENT_STATE=$(mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -N -B << 'EOF'
SELECT CONCAT(
    COALESCE(SUM(DATA_LENGTH + INDEX_LENGTH), 0), '|',
    COALESCE(MAX(UPDATE_TIME), 'never'), '|',
    COUNT(*)
) AS state
FROM information_schema.TABLES
WHERE TABLE_SCHEMA NOT IN ('information_schema', 'performance_schema', 'mysql', 'sys');
EOF
)

# Get last backup state
LAST_STATE_FILE="${STATE_DIR}/last_incremental_state"
if [ -f "${LAST_STATE_FILE}" ]; then
    LAST_STATE=$(cat "${LAST_STATE_FILE}")
else
    LAST_STATE=""
fi

# Check if database has changed
if [ "${CURRENT_STATE}" = "${LAST_STATE}" ]; then
    echo "[$(date)] No changes detected in database. Skipping backup."
    echo "[$(date)] Current state: ${CURRENT_STATE}"
    exit 0
fi

echo "[$(date)] Changes detected! Proceeding with incremental backup..."
echo "[$(date)] Previous state: ${LAST_STATE}"
echo "[$(date)] Current state: ${CURRENT_STATE}"

# Get list of databases with recent changes
CHANGED_DATABASES=$(mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -N -B << 'EOF'
SELECT DISTINCT TABLE_SCHEMA
FROM information_schema.TABLES
WHERE TABLE_SCHEMA NOT IN ('information_schema', 'performance_schema', 'mysql', 'sys')
AND (UPDATE_TIME IS NULL OR UPDATE_TIME >= DATE_SUB(NOW(), INTERVAL 2 HOUR))
ORDER BY TABLE_SCHEMA;
EOF
)

if [ -z "${CHANGED_DATABASES}" ]; then
    echo "[$(date)] No specific databases with recent changes found. Backing up all databases."
    CHANGED_DATABASES=$(mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -N -B -e "SHOW DATABASES;" | grep -Ev "(Database|information_schema|performance_schema|mysql|sys)")
fi

# Perform incremental backup of changed databases
BACKUP_FILE="${INCREMENTAL_DIR}/${BACKUP_DATE}_incremental.sql"
echo "[$(date)] Backing up databases: ${CHANGED_DATABASES}"

# Backup each changed database
for DB in ${CHANGED_DATABASES}; do
    echo "[$(date)] Backing up database: ${DB}"
    mysqldump -uroot -p"${MYSQL_ROOT_PASSWORD}" \
        --single-transaction \
        --routines \
        --triggers \
        --events \
        --quick \
        --lock-tables=false \
        --skip-comments \
        --databases "${DB}" \
        >> "${BACKUP_FILE}"
done

# Only create backup file if something was written
if [ -f "${BACKUP_FILE}" ] && [ -s "${BACKUP_FILE}" ]; then
    # Compress backup
    echo "[$(date)] Compressing backup..."
    bzip2 -f "${BACKUP_FILE}"
    BACKUP_FILE="${BACKUP_FILE}.bz2"
    
    # Calculate checksum
    sha256sum "${BACKUP_FILE}" > "${BACKUP_FILE}.sha256"
    
    # Store metadata
    cat > "${INCREMENTAL_DIR}/${BACKUP_DATE}_metadata.txt" << EOF
backup_type=incremental
backup_date=${BACKUP_DATE}
changed_databases=${CHANGED_DATABASES}
state_checksum=${CURRENT_STATE}
timestamp=$(date -Iseconds)
EOF
    
    # Update state tracking
    echo "${CURRENT_STATE}" > "${LAST_STATE_FILE}"
    echo "${BACKUP_DATE}" > "${STATE_DIR}/last_incremental_backup"
    
    # Create symbolic link to latest incremental
    ln -sfn "${BACKUP_FILE}" "${INCREMENTAL_DIR}/latest_incremental.sql.bz2"
    
    echo "[$(date)] Incremental backup completed: ${BACKUP_FILE}"
    
    # Verify backup
    BACKUP_SIZE=$(du -h "${BACKUP_FILE}" | cut -f1)
    echo "[$(date)] Backup size: ${BACKUP_SIZE}"
    
    # Verify checksum
    if sha256sum -c "${BACKUP_FILE}.sha256" &>/dev/null; then
        echo "[$(date)] Checksum verified successfully"
    else
        echo "[$(date)] WARNING: Checksum verification failed!"
        exit 1
    fi
    
    # Clean up old incremental backups
    find "${INCREMENTAL_DIR}" -name "*_incremental.sql.bz2" -mtime +${RETENTION_DAYS} -delete
    find "${INCREMENTAL_DIR}" -name "*_metadata.txt" -mtime +${RETENTION_DAYS} -delete
    find "${INCREMENTAL_DIR}" -name "*.sha256" -mtime +${RETENTION_DAYS} -delete
    
    exit 0
else
    echo "[$(date)] No data to backup (empty dump)"
    rm -f "${BACKUP_FILE}" 2>/dev/null || true
    exit 0
fi
