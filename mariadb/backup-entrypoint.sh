#!/bin/bash
set -e

echo "[backup-entrypoint] Starting MariaDB with backup capability"

# Setup cron if backups are enabled
if [ "${BACKUP_ENABLED}" = "true" ]; then
    echo "[backup-entrypoint] Configuring backup schedules"
    
    # Full backup schedule (default: Sunday 2 AM)
    FULL_SCHEDULE="${BACKUP_FULL_SCHEDULE:-0 2 * * 0}"
    INCREMENTAL_SCHEDULE="${BACKUP_INCREMENTAL_SCHEDULE:-0 2 * * 1-6}"
    
    # Create crontab
    cat > /etc/cron.d/mariadb-backup << EOF
# MariaDB Backup Schedule
${FULL_SCHEDULE} root /usr/local/bin/backup-full.sh >> /var/log/mysql/backup-full.log 2>&1
${INCREMENTAL_SCHEDULE} root /usr/local/bin/backup-incremental.sh >> /var/log/mysql/backup-incremental.log 2>&1
EOF
    
    chmod 0644 /etc/cron.d/mariadb-backup
    
    # Start cron
    service cron start
    echo "[backup-entrypoint] Backup schedules configured"
    echo "  Full backups: ${FULL_SCHEDULE}"
    echo "  Incremental backups: ${INCREMENTAL_SCHEDULE}"
fi

# Execute original MariaDB entrypoint
# Note: MariaDB 11.7 uses 'mariadbd' not 'mysqld'
exec docker-entrypoint.sh mariadbd
