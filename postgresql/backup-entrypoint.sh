#!/bin/bash
set -e

echo "[backup-entrypoint] Starting PostgreSQL with intelligent backup capability"

# Check if this is a fresh installation and backups exist
BACKUP_DIR="/backups"
PGDATA="/var/lib/postgresql/data"
AUTO_RESTORE="${BACKUP_AUTO_RESTORE:-true}"

# Ensure log directory exists with correct permissions
mkdir -p /var/log/postgresql
chown -R postgres:postgres /var/log/postgresql
chmod 755 /var/log/postgresql

# Initialize PostgreSQL if needed
if [ ! -s "${PGDATA}/PG_VERSION" ]; then
    echo "[backup-entrypoint] Initializing PostgreSQL data directory..."
    
    # Get postgres password
    if [ -f /run/secrets/postgres_password ]; then
        POSTGRES_PASSWORD=$(cat /run/secrets/postgres_password)
    else
        POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-}"
    fi
    
    if [ -z "${POSTGRES_PASSWORD}" ]; then
        echo "[backup-entrypoint] ERROR: No postgres password provided"
        exit 1
    fi
    
    # Initialize database as postgres user
    su-exec postgres initdb -D "${PGDATA}" --auth=md5 --pwfile=<(echo "${POSTGRES_PASSWORD}")
    
    # Use provided config if available, otherwise use embedded config
    if [ -f /etc/postgresql/postgresql.conf ]; then
        echo "[backup-entrypoint] Using provided postgresql.conf"
        cp /etc/postgresql/postgresql.conf "${PGDATA}/postgresql.conf"
    else
        echo "[backup-entrypoint] Using default postgresql.conf"
        # Default config is already in place from initdb
    fi
    
    # Configure pg_hba.conf for network access
    cat > "${PGDATA}/pg_hba.conf" << EOF
# TYPE  DATABASE        USER            ADDRESS                 METHOD
local   all             all                                     peer
host    all             all             127.0.0.1/32            md5
host    all             all             ::1/128                 md5
host    all             all             0.0.0.0/0               md5
EOF

    # Auto-restore from backup if enabled and backups exist
    if [ "${AUTO_RESTORE}" = "true" ]; then
        if [ -L "${BACKUP_DIR}/full/latest_full.sql.bz2" ] || [ -f "${BACKUP_DIR}/full/latest_full.sql.bz2" ]; then
            echo "[backup-entrypoint] Fresh installation detected with available backups"
            echo "[backup-entrypoint] Will restore after PostgreSQL initialization..."
            
            # Start PostgreSQL in background temporarily
            su-exec postgres postgres -D "${PGDATA}" &
            POSTGRES_PID=$!
            
            # Wait for PostgreSQL to be ready
            echo "[backup-entrypoint] Waiting for PostgreSQL to initialize..."
            for i in {1..60}; do
                if su-exec postgres psql -U postgres -c "SELECT 1" &>/dev/null 2>&1; then
                    echo "[backup-entrypoint] PostgreSQL is ready"
                    break
                fi
                if [ $i -eq 60 ]; then
                    echo "[backup-entrypoint] ERROR: PostgreSQL failed to start within timeout"
                    kill $POSTGRES_PID 2>/dev/null || true
                    exit 1
                fi
                sleep 2
            done
            
            # Restore from backup
            echo "[backup-entrypoint] Restoring from latest backup..."
            if POSTGRES_PASSWORD="${POSTGRES_PASSWORD}" /usr/local/bin/backup-restore.sh restore-latest; then
                echo "[backup-entrypoint] Database restored successfully from backup!"
            else
                echo "[backup-entrypoint] WARNING: Backup restore failed, continuing with fresh database"
            fi
            
            # Shutdown temporary instance
            su-exec postgres pg_ctl -D "${PGDATA}" stop -m fast
            wait $POSTGRES_PID
        else
            echo "[backup-entrypoint] No backups found for auto-restore"
        fi
    fi
else
    echo "[backup-entrypoint] Database already initialized"
fi

# Setup cron if backups are enabled
if [ "${BACKUP_ENABLED}" = "true" ]; then
    echo "[backup-entrypoint] Configuring backup schedules"
    
    # Full backup schedule (default: Sunday 3 AM weekly)
    FULL_SCHEDULE="${BACKUP_FULL_SCHEDULE:-0 3 * * 0}"
    # Differential backup schedule (default: Wednesday 3 AM mid-week)
    DIFFERENTIAL_SCHEDULE="${BACKUP_DIFFERENTIAL_SCHEDULE:-0 3 * * 3}"
    # Incremental backup schedule (default: every hour)
    INCREMENTAL_SCHEDULE="${BACKUP_INCREMENTAL_SCHEDULE:-0 * * * *}"
    
    # Create crontab for root
    cat > /var/spool/cron/crontabs/root << EOF
# PostgreSQL Intelligent Backup Schedule
${FULL_SCHEDULE} /usr/local/bin/backup-full.sh >> /var/log/postgresql/backup-full.log 2>&1
${DIFFERENTIAL_SCHEDULE} /usr/local/bin/backup-differential.sh >> /var/log/postgresql/backup-differential.log 2>&1
${INCREMENTAL_SCHEDULE} /usr/local/bin/backup-incremental.sh >> /var/log/postgresql/backup-incremental.log 2>&1
EOF
    
    chmod 0600 /var/spool/cron/crontabs/root
    
    # Start crond in background
    crond -b -l 2
    echo "[backup-entrypoint] Backup schedules configured"
    echo "  Full backups: ${FULL_SCHEDULE}"
    echo "  Differential backups: ${DIFFERENTIAL_SCHEDULE}"
    echo "  Incremental backups: ${INCREMENTAL_SCHEDULE}"
    echo "  Retention: 14 days (full), 42 days (differential+full), 42+ days (full only)"
fi

# Execute PostgreSQL as postgres user
echo "[backup-entrypoint] Starting PostgreSQL..."
exec su-exec postgres postgres -D "${PGDATA}"
