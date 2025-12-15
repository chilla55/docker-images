#!/bin/bash
set -e

BACKUP_DIR="/backups"
BACKUP_DATE=$(date +%Y%m%d_%H%M%S)
DIFFERENTIAL_DIR="${BACKUP_DIR}/differential"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-30}"
MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password 2>/dev/null || echo "${MYSQL_ROOT_PASSWORD}")
STATE_DIR="/var/lib/mysql-backup-state"

echo "[$(date)] Starting DIFFERENTIAL backup (mid-week)"

# Wait for MariaDB to be ready
until mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &>/dev/null; do
    echo "[$(date)] Waiting for MariaDB to be ready..."
    sleep 5
done

# Check if there's a full backup
if [ ! -f "${BACKUP_DIR}/full/latest_full.sql.bz2" ]; then
    echo "[$(date)] No full backup found. Running full backup first..."
    /usr/local/bin/backup-full.sh
    exit 0
fi

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

# Get last differential backup state
mkdir -p "${STATE_DIR}"
LAST_STATE_FILE="${STATE_DIR}/last_differential_state"
if [ -f "${LAST_STATE_FILE}" ]; then
    LAST_STATE=$(cat "${LAST_STATE_FILE}")
else
    LAST_STATE=""
fi

# Check if database has changed since last differential backup
if [ "${CURRENT_STATE}" = "${LAST_STATE}" ] && [ -n "${LAST_STATE}" ]; then
    echo "[$(date)] No changes detected since last differential backup. Skipping."
    echo "[$(date)] Current state: ${CURRENT_STATE}"
    exit 0
fi

echo "[$(date)] Changes detected! Proceeding with differential backup..."
echo "[$(date)] Previous state: ${LAST_STATE}"
echo "[$(date)] Current state: ${CURRENT_STATE}"

# Create differential backup directory
mkdir -p "${DIFFERENTIAL_DIR}"

# Get list of all databases (excluding system tables)
DATABASES=$(mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SHOW DATABASES;" | grep -Ev "(Database|information_schema|performance_schema|mysql|sys)")

# Perform differential backup of all databases
BACKUP_FILE="${DIFFERENTIAL_DIR}/${BACKUP_DATE}_differential.sql"
echo "[$(date)] Backing up databases: ${DATABASES}"

mysqldump -uroot -p"${MYSQL_ROOT_PASSWORD}" \
    --single-transaction \
    --routines \
    --triggers \
    --events \
    --flush-logs \
    --all-databases \
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
cat > "${DIFFERENTIAL_DIR}/${BACKUP_DATE}_metadata.txt" << EOF
backup_type=differential
backup_date=${BACKUP_DATE}
databases=${DATABASES}
timestamp=$(date -Iseconds)
EOF

# Create symbolic link to latest
ln -sfn "${BACKUP_FILE}" "${DIFFERENTIAL_DIR}/latest_differential.sql.bz2"

# Update state tracking
echo "${CURRENT_STATE}" > "${LAST_STATE_FILE}"
echo "${BACKUP_DATE}" > "${STATE_DIR}/last_differential_backup"

echo "[$(date)] Differential backup completed: ${BACKUP_FILE}"

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
