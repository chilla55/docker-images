#!/bin/bash
set -e

echo "[backup-entrypoint] Starting PostgreSQL with backup capability"

# Setup cron if backups are enabled
if [ "${BACKUP_ENABLED}" = "true" ]; then
    echo "[backup-entrypoint] Configuring backup schedules"
    
    # Full backup schedule (default: Sunday 3 AM)
    FULL_SCHEDULE="${BACKUP_FULL_SCHEDULE:-0 3 * * 0}"
    INCREMENTAL_SCHEDULE="${BACKUP_INCREMENTAL_SCHEDULE:-0 3 * * 1-6}"
    
    # Create crontab
    cat > /etc/cron.d/postgresql-backup << EOF
# PostgreSQL Backup Schedule
${FULL_SCHEDULE} postgres /usr/local/bin/backup-full.sh >> /var/log/postgresql/backup-full.log 2>&1
${INCREMENTAL_SCHEDULE} postgres /usr/local/bin/backup-incremental.sh >> /var/log/postgresql/backup-incremental.log 2>&1
EOF
    
    chmod 0644 /etc/cron.d/postgresql-backup
    
    # Start cron
    service cron start
    echo "[backup-entrypoint] Backup schedules configured"
    echo "  Full backups: ${FULL_SCHEDULE}"
    echo "  Incremental backups: ${INCREMENTAL_SCHEDULE}"
fi

# Execute original PostgreSQL entrypoint
exec docker-entrypoint.sh "$@"
