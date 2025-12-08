#!/bin/bash
set -eo pipefail

# Create log directory
mkdir -p /var/log/mysql
chown -R mysql:mysql /var/log/mysql

# Determine which config to use based on replication mode
if [ "${REPLICATION_MODE}" = "master" ]; then
    echo "Configuring as MASTER server..."
    rm -f /etc/mysql/conf.d/mariadb-slave.cnf
    
    # Set server ID based on hostname/IP
    SERVER_ID=1
    sed -i "s/server-id.*/server-id = ${SERVER_ID}/" /etc/mysql/conf.d/mariadb-master.cnf
    
    # Start connectivity monitor in background
    if [ "${ENABLE_CONNECTIVITY_MONITOR}" = "true" ]; then
        echo "Starting connectivity monitor for Primary..."
        /usr/local/bin/check-connectivity.sh &
    fi
    
elif [ "${REPLICATION_MODE}" = "slave" ]; then
    echo "Configuring as SLAVE server..."
    rm -f /etc/mysql/conf.d/mariadb-master.cnf
    
    # Set server ID
    SERVER_ID=2
    sed -i "s/server-id.*/server-id = ${SERVER_ID}/" /etc/mysql/conf.d/mariadb-slave.cnf
    
    # Wait for master to be ready
    echo "Waiting for master to be ready..."
    until nc -z ${MASTER_HOST} ${MASTER_PORT}; do
        echo "Master not ready, waiting..."
        sleep 2
    done
    echo "Master is ready!"
    
    # Start connectivity monitor in background
    if [ "${ENABLE_CONNECTIVITY_MONITOR}" = "true" ]; then
        echo "Starting connectivity monitor for Secondary..."
        /usr/local/bin/check-connectivity.sh &
    fi
fi

# Execute the original MariaDB entrypoint
exec docker-entrypoint.sh "$@"
