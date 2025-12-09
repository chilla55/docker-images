#!/bin/bash
set -eo pipefail

# Create log directory
mkdir -p /var/log/mysql
chown -R mysql:mysql /var/log/mysql

# Helper to log with timestamp when stdout/stderr are quiet
log() {
    echo "$(date -Iseconds) $1" | tee -a /var/log/mysql/replication.log
}

# Determine which config to use based on replication mode
if [ "${REPLICATION_MODE}" = "master" ]; then
    log "Configuring as MASTER server..."
    rm -f /etc/mysql/conf.d/mariadb-slave.cnf
    
    # Set server ID based on hostname/IP
    SERVER_ID=1
    sed -i "s/server-id.*/server-id = ${SERVER_ID}/" /etc/mysql/conf.d/mariadb-master.cnf
    
    # Start connectivity monitor in background
    # Delay startup by 90 seconds to allow secondary to bootstrap and connect during grace period
    if [ "${ENABLE_CONNECTIVITY_MONITOR}" = "true" ]; then
        echo "Starting connectivity monitor for Primary (delayed 90 seconds for secondary bootstrap)..."
        (sleep 90 && /usr/local/bin/check-connectivity.sh) &
    fi
    
elif [ "${REPLICATION_MODE}" = "slave" ]; then
    log "Configuring as SLAVE server..."
    rm -f /etc/mysql/conf.d/mariadb-master.cnf
    
    # Set server ID
    SERVER_ID=2
    sed -i "s/server-id.*/server-id = ${SERVER_ID}/" /etc/mysql/conf.d/mariadb-slave.cnf
    
    # Wait for master to be ready
    log "Waiting for master to be ready (host=${MASTER_HOST} port=${MASTER_PORT})..."
    while true; do
        RESOLVED=$(getent hosts ${MASTER_HOST} | awk '{print $1}' | paste -sd ',' - || true)
        log "Master DNS: ${MASTER_HOST} -> ${RESOLVED:-<unresolved>}"
        
        # Try to ping the master with mariadb-admin (app-level check, more reliable than nc)
        if mariadb-admin ping -h "${MASTER_HOST}" -P "${MASTER_PORT}" --connect-timeout=3 --silent 2>/dev/null; then
            log "Master is ready!"
            break
        else
            log "Master not ready (ping failed), waiting..."
        fi
        sleep 2
    done
    
    # Start connectivity monitor in background
    if [ "${ENABLE_CONNECTIVITY_MONITOR}" = "true" ]; then
        log "Starting connectivity monitor for Secondary..."
        /usr/local/bin/check-connectivity.sh &
    fi
fi

# Execute the original MariaDB entrypoint with logging note
log "Starting MariaDB entrypoint"
exec /usr/local/bin/docker-entrypoint.sh "$@"
