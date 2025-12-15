#!/bin/bash
set -e

BACKUP_DIR="/backups"
BACKUP_DATE=$(date +%Y%m%d_%H%M%S)
FULL_BACKUP_DIR="${BACKUP_DIR}/full"
POSTGRES_PASSWORD=$(cat /run/secrets/postgres_password 2>/dev/null || echo "${POSTGRES_PASSWORD}")

echo "[$(date)] Starting FULL backup"

# Wait for PostgreSQL to be ready
until PGPASSWORD="${POSTGRES_PASSWORD}" psql -U postgres -c "SELECT 1" &>/dev/null; do
    echo "[$(date)] Waiting for PostgreSQL to be ready..."
    sleep 5
done

# Create backup directory
mkdir -p "${FULL_BACKUP_DIR}"

# Get list of all databases (excluding templates)
DATABASES=$(PGPASSWORD="${POSTGRES_PASSWORD}" psql -U postgres -t -c "SELECT datname FROM pg_database WHERE datistemplate = false AND datname != 'postgres';" | grep -v '^$')

# Perform full backup of all databases
BACKUP_FILE="${FULL_BACKUP_DIR}/${BACKUP_DATE}_full.sql"
echo "[$(date)] Backing up all databases..."

PGPASSWORD="${POSTGRES_PASSWORD}" pg_dumpall -U postgres \
    --clean \
    --if-exists \
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
echo "${BACKUP_DATE}" > /var/lib/postgresql-backup-state/last_full_backup

echo "[$(date)] Full backup completed: ${BACKUP_FILE}"

# Clean up old backups with tiered retention strategy
echo "[$(date)] Cleaning up backups with tiered retention"

# Tier 1: Keep incrementals for 14 days (full granularity)
find "${BACKUP_DIR}/incremental" -name "*.sql.bz2" -mtime +14 -delete 2>/dev/null || true
find "${BACKUP_DIR}/incremental" -name "*_metadata.txt" -mtime +14 -delete 2>/dev/null || true
find "${BACKUP_DIR}/incremental" -name "*.sha256" -mtime +14 -delete 2>/dev/null || true
echo "[$(date)] Cleaned up incrementals older than 14 days"

# Tier 2: Keep differential backups for 42 days (twice-weekly recovery)
find "${BACKUP_DIR}/differential" -name "*.sql.bz2" -mtime +42 -delete 2>/dev/null || true
find "${BACKUP_DIR}/differential" -name "*_metadata.txt" -mtime +42 -delete 2>/dev/null || true
find "${BACKUP_DIR}/differential" -name "*.sha256" -mtime +42 -delete 2>/dev/null || true
echo "[$(date)] Cleaned up differentials older than 42 days"

# Tier 3: Keep full backups for 42 days (weekly recovery)
# Always keep minimum 2 full backups for safety
FULL_BACKUP_COUNT=$(find "${BACKUP_DIR}/full" -name "*_full.sql.bz2" -type f 2>/dev/null | wc -l)
if [ "${FULL_BACKUP_COUNT}" -gt 2 ]; then
    find "${BACKUP_DIR}/full" -name "*_full.sql.bz2" -mtime +42 -delete
    find "${BACKUP_DIR}/full" -name "*_metadata.txt" -mtime +42 -delete
    find "${BACKUP_DIR}/full" -name "*.sha256" -mtime +42 -delete
    echo "[$(date)] Cleaned up full backups older than 42 days (keeping minimum 2 backups)"
else
    echo "[$(date)] Keeping all full backups (count: ${FULL_BACKUP_COUNT}, minimum: 2)"
fi

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
