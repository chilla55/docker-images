#!/bin/bash
set -e

echo "[backup-entrypoint] Starting MariaDB with intelligent backup capability"

# Check if this is a fresh installation and backups exist
BACKUP_DIR="/backups"
MYSQL_DATA_DIR="/var/lib/mysql"
AUTO_RESTORE="${BACKUP_AUTO_RESTORE:-true}"

# Initialize MariaDB if needed
if [ ! -d "${MYSQL_DATA_DIR}/mysql" ]; then
    echo "[backup-entrypoint] Initializing MariaDB data directory..."
    
    # Get root password
    if [ -f /run/secrets/mysql_root_password ]; then
        MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password)
    else
        MYSQL_ROOT_PASSWORD="${MYSQL_ROOT_PASSWORD:-}"
    fi
    
    if [ -z "${MYSQL_ROOT_PASSWORD}" ]; then
        echo "[backup-entrypoint] ERROR: No root password provided"
        exit 1
    fi
    
    # Initialize database
    mysql_install_db --user=mysql --datadir="${MYSQL_DATA_DIR}" --auth-root-authentication-method=normal
    
    # Auto-restore from backup if enabled and backups exist
    if [ "${AUTO_RESTORE}" = "true" ]; then
        if [ -L "${BACKUP_DIR}/full/latest_full.sql.bz2" ] || [ -f "${BACKUP_DIR}/full/latest_full.sql.bz2" ]; then
            echo "[backup-entrypoint] Fresh installation detected with available backups"
            echo "[backup-entrypoint] Will restore after MariaDB initialization..."
            
            # Start MariaDB in background temporarily
            mariadbd --user=mysql --datadir="${MYSQL_DATA_DIR}" --skip-networking --socket=/tmp/mysql_init.sock &
            MARIADB_PID=$!
            
            # Wait for MariaDB to be ready
            echo "[backup-entrypoint] Waiting for MariaDB to initialize..."
            for i in {1..60}; do
                if mysql --socket=/tmp/mysql_init.sock -e "SELECT 1" &>/dev/null 2>&1; then
                    echo "[backup-entrypoint] MariaDB is ready"
                    break
                fi
                if [ $i -eq 60 ]; then
                    echo "[backup-entrypoint] ERROR: MariaDB failed to start within timeout"
                    kill $MARIADB_PID 2>/dev/null || true
                    exit 1
                fi
                sleep 2
            done
            
            # Set root password
            mysql --socket=/tmp/mysql_init.sock << EOF
ALTER USER 'root'@'localhost' IDENTIFIED BY '${MYSQL_ROOT_PASSWORD}';
DELETE FROM mysql.user WHERE User='';
DELETE FROM mysql.user WHERE User='root' AND Host NOT IN ('localhost', '127.0.0.1', '::1');
DROP DATABASE IF EXISTS test;
DELETE FROM mysql.db WHERE Db='test' OR Db='test\\_%';
FLUSH PRIVILEGES;
EOF
            
            # Restore from backup
            echo "[backup-entrypoint] Restoring from latest backup..."
            if MYSQL_ROOT_PASSWORD="${MYSQL_ROOT_PASSWORD}" /usr/local/bin/backup-restore.sh restore-latest; then
                echo "[backup-entrypoint] Database restored successfully from backup!"
            else
                echo "[backup-entrypoint] WARNING: Backup restore failed, continuing with fresh database"
            fi
            
            # Shutdown temporary instance
            mysqladmin --socket=/tmp/mysql_init.sock -uroot -p"${MYSQL_ROOT_PASSWORD}" shutdown
            wait $MARIADB_PID
        else
            echo "[backup-entrypoint] No backups found for auto-restore"
            
            # Start MariaDB temporarily to set root password
            mariadbd --user=mysql --datadir="${MYSQL_DATA_DIR}" --skip-networking --socket=/tmp/mysql_init.sock &
            MARIADB_PID=$!
            
            # Wait for MariaDB
            for i in {1..30}; do
                if mysql --socket=/tmp/mysql_init.sock -e "SELECT 1" &>/dev/null 2>&1; then
                    break
                fi
                sleep 1
            done
            
            # Set root password and secure installation
            mysql --socket=/tmp/mysql_init.sock << EOF
ALTER USER 'root'@'localhost' IDENTIFIED BY '${MYSQL_ROOT_PASSWORD}';
CREATE USER IF NOT EXISTS 'root'@'%' IDENTIFIED BY '${MYSQL_ROOT_PASSWORD}';
GRANT ALL PRIVILEGES ON *.* TO 'root'@'%' WITH GRANT OPTION;
DELETE FROM mysql.user WHERE User='';
DROP DATABASE IF EXISTS test;
DELETE FROM mysql.db WHERE Db='test' OR Db='test\\_%';
FLUSH PRIVILEGES;
EOF
            
            # Shutdown temporary instance
            mysqladmin --socket=/tmp/mysql_init.sock -uroot -p"${MYSQL_ROOT_PASSWORD}" shutdown
            wait $MARIADB_PID
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
    # Incremental backup schedule (default: every hour)
    INCREMENTAL_SCHEDULE="${BACKUP_INCREMENTAL_SCHEDULE:-0 * * * *}"
    
    # Create crontab for root
    cat > /var/spool/cron/crontabs/root << EOF
# MariaDB Intelligent Backup Schedule
${FULL_SCHEDULE} /usr/local/bin/backup-full.sh >> /var/log/mysql/backup-full.log 2>&1
${INCREMENTAL_SCHEDULE} /usr/local/bin/backup-incremental.sh >> /var/log/mysql/backup-incremental.log 2>&1
EOF
    
    chmod 0600 /var/spool/cron/crontabs/root
    
    # Ensure log directory exists
    mkdir -p /var/log/mysql
    chown mysql:mysql /var/log/mysql
    
    # Start crond in background
    crond -b -l 2
    echo "[backup-entrypoint] Backup schedules configured"
    echo "  Full backups: ${FULL_SCHEDULE}"
    echo "  Incremental backups: ${INCREMENTAL_SCHEDULE}"
    echo "  Retention: ${BACKUP_RETENTION_DAYS:-30} days"
fi

# Execute MariaDB
echo "[backup-entrypoint] Starting MariaDB..."
exec "$@"
