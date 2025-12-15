#!/bin/bash
set -e

echo "[backup-entrypoint] Starting MariaDB with intelligent backup capability"

# Check if this is a fresh installation and backups exist
BACKUP_DIR="/backups"
MYSQL_DATA_DIR="/var/lib/mysql"
AUTO_RESTORE="${BACKUP_AUTO_RESTORE:-true}"

# Auto-restore from backup if enabled and backups exist
if [ "${AUTO_RESTORE}" = "true" ]; then
    # Check if backup exists
    if [ -L "${BACKUP_DIR}/full/latest_full.sql.bz2" ] || [ -f "${BACKUP_DIR}/full/latest_full.sql.bz2" ]; then
        # Check if this is a fresh installation (no mysql data directory initialized)
        if [ ! -d "${MYSQL_DATA_DIR}/mysql" ]; then
            echo "[backup-entrypoint] Fresh installation detected with available backups"
            echo "[backup-entrypoint] Will restore after MariaDB initialization..."
            
            # Start MariaDB in background to initialize system databases
            docker-entrypoint.sh "$@" &
            MARIADB_PID=$!
            
            # Wait for MariaDB to be ready
            echo "[backup-entrypoint] Waiting for MariaDB to initialize..."
            MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password 2>/dev/null || echo "${MYSQL_ROOT_PASSWORD}")
            for i in {1..60}; do
                if mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &>/dev/null 2>&1; then
                    echo "[backup-entrypoint] MariaDB is ready"
                    break
                fi
                if [ $i -eq 60 ]; then
                    echo "[backup-entrypoint] ERROR: MariaDB failed to start within timeout"
                    kill $MARIADB_PID 2>/dev/null || true
                    exit 1
                fi
                echo "[backup-entrypoint] Waiting for MariaDB... ($i/60)"
                sleep 2
            done
            
            # Restore from backup
            echo "[backup-entrypoint] Restoring from latest backup..."
            if /usr/local/bin/backup-restore.sh restore-latest; then
                echo "[backup-entrypoint] Database restored successfully from backup!"
            else
                echo "[backup-entrypoint] WARNING: Backup restore failed, continuing with fresh database"
            fi
            
            # Keep MariaDB running in foreground
            wait $MARIADB_PID
            exit 0
        else
            echo "[backup-entrypoint] Database already initialized, skipping auto-restore"
        fi
    else
        echo "[backup-entrypoint] No backups found for auto-restore"
    fi
fi

# Setup cron if backups are enabled
if [ "${BACKUP_ENABLED}" = "true" ]; then
    echo "[backup-entrypoint] Configuring backup schedules"
    
    # Full backup schedule (default: Sunday 3 AM weekly)
    FULL_SCHEDULE="${BACKUP_FULL_SCHEDULE:-0 3 * * 0}"
    # Incremental backup schedule (default: every hour)
    INCREMENTAL_SCHEDULE="${BACKUP_INCREMENTAL_SCHEDULE:-0 * * * *}"
    
    # Create crontab
    cat > /etc/cron.d/mariadb-backup << EOF
# MariaDB Intelligent Backup Schedule
${FULL_SCHEDULE} root /usr/local/bin/backup-full.sh >> /var/log/mysql/backup-full.log 2>&1
${INCREMENTAL_SCHEDULE} root /usr/local/bin/backup-incremental.sh >> /var/log/mysql/backup-incremental.log 2>&1
EOF
    
    chmod 0644 /etc/cron.d/mariadb-backup
    
    # Ensure log directory exists
    mkdir -p /var/log/mysql
    
    # Start cron
    service cron start
    echo "[backup-entrypoint] Backup schedules configured"
    echo "  Full backups: ${FULL_SCHEDULE}"
    echo "  Incremental backups: ${INCREMENTAL_SCHEDULE}"
    echo "  Retention: ${BACKUP_RETENTION_DAYS:-30} days"
fi

# Execute original MariaDB entrypoint
exec docker-entrypoint.sh "$@"
