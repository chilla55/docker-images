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
    if [ "${ENABLE_CONNECTIVITY_MONITOR}" = "true" ]; then
        log "Starting connectivity monitor for Primary..."
        /usr/local/bin/check-connectivity.sh &
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
        NC_OUT=$(nc -vz -w3 ${MASTER_HOST} ${MASTER_PORT} 2>&1) || NC_STATUS=$?
        if [ -z "${NC_STATUS:-}" ]; then NC_STATUS=0; fi
        log "nc result (code=${NC_STATUS}): ${NC_OUT}"
        if [ "${NC_STATUS}" -eq 0 ]; then
            log "Master is ready!"
            break
        fi
        log "Master not ready, waiting..."
        NC_STATUS=
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
