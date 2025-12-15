#!/bin/bash
set -e

BACKUP_DIR="/backups"
BACKUP_DATE=$(date +%Y%m%d_%H%M%S)
INCREMENTAL_DIR="${BACKUP_DIR}/incremental"
POSTGRES_PASSWORD=$(cat /run/secrets/postgres_password 2>/dev/null || echo "${POSTGRES_PASSWORD}")
STATE_DIR="/var/lib/postgresql-backup-state"

echo "[$(date)] Starting INCREMENTAL backup (change-detection)"

# Wait for PostgreSQL to be ready
until PGPASSWORD="${POSTGRES_PASSWORD}" psql -U postgres -c "SELECT 1" &>/dev/null; do
    echo "[$(date)] Waiting for PostgreSQL to be ready..."
    sleep 5
done

# Check if there's a full backup
if [ ! -f "${BACKUP_DIR}/full/latest_full.sql.bz2" ]; then
    echo "[$(date)] No full backup found. Running full backup first..."
    /usr/local/bin/backup-full.sh
    if [ $? -ne 0 ]; then
        echo "[$(date)] ERROR: Full backup failed. Cannot proceed with incremental backup."
        exit 1
    fi
    echo "[$(date)] Full backup completed. Skipping incremental backup this time."
    exit 0
fi

# Create incremental backup directory
mkdir -p "${INCREMENTAL_DIR}"
mkdir -p "${STATE_DIR}"

# Calculate current database state checksum
CURRENT_STATE=$(PGPASSWORD="${POSTGRES_PASSWORD}" psql -U postgres -t -c "
SELECT CONCAT(
    COALESCE(SUM(pg_database_size(datname)), 0)::text, '|',
    COALESCE(MAX(stats_reset)::text, 'never'), '|',
    COUNT(*)::text
) 
FROM pg_stat_database 
WHERE datname NOT IN ('template0', 'template1');
")

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

# Get list of databases
DATABASES=$(PGPASSWORD="${POSTGRES_PASSWORD}" psql -U postgres -t -c "SELECT datname FROM pg_database WHERE datistemplate = false AND datname != 'postgres';" | grep -v '^$')

# Perform incremental backup of all user databases
BACKUP_FILE="${INCREMENTAL_DIR}/${BACKUP_DATE}_incremental.sql"
echo "[$(date)] Backing up databases: ${DATABASES}"

for DB in ${DATABASES}; do
    echo "[$(date)] Backing up database: ${DB}"
    PGPASSWORD="${POSTGRES_PASSWORD}" pg_dump -U postgres \
        --clean \
        --if-exists \
        --create \
        "${DB}" \
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
databases=${DATABASES}
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
    
    exit 0
else
    echo "[$(date)] No data to backup (empty dump)"
    rm -f "${BACKUP_FILE}" 2>/dev/null || true
    exit 0
fi
