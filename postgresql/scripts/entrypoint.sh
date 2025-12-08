#!/bin/bash
set -eo pipefail

# Create log directory
mkdir -p /var/log/postgresql
chown -R postgres:postgres /var/log/postgresql

# Only configure if database is already initialized
# The docker-entrypoint.sh will handle initial setup
if [ -s "$PGDATA/PG_VERSION" ]; then
    if [ "${REPLICATION_MODE}" = "primary" ]; then
        echo "Configuring as PRIMARY server..."
        
        # Append replication settings if not already present
        if ! grep -q "Replication Settings" ${PGDATA}/postgresql.conf; then
            cat >> ${PGDATA}/postgresql.conf <<EOF

# Replication Settings
wal_level = replica
max_wal_senders = 10
max_replication_slots = 10
hot_standby = on
archive_mode = on
archive_command = 'test ! -f /var/lib/postgresql/archive/%f && cp %p /var/lib/postgresql/archive/%f'
EOF
        fi

        # Create archive directory
        mkdir -p /var/lib/postgresql/archive
        chown -R postgres:postgres /var/lib/postgresql/archive
        
    elif [ "${REPLICATION_MODE}" = "replica" ]; then
        echo "Configuring as REPLICA server..."
        
        # Wait for primary to be ready
        echo "Waiting for primary to be ready..."
        until pg_isready -h ${PRIMARY_HOST} -p ${PRIMARY_PORT} -U ${POSTGRES_USER:-postgres}; do
            echo "Primary not ready, waiting..."
            sleep 2
        done
        echo "Primary is ready!"
        
        # Check if this is first run (empty data directory)
        if [ ! -s "$PGDATA/PG_VERSION" ]; then
            echo "Initializing replica from primary..."
            rm -rf ${PGDATA}/*
            
            # Use pg_basebackup to clone from primary
            PGPASSWORD=${REPLICATION_PASSWORD} pg_basebackup \
                -h ${PRIMARY_HOST} \
                -p ${PRIMARY_PORT} \
                -U ${REPLICATION_USER} \
                -D ${PGDATA} \
                -Fp -Xs -P -R
            
            echo "Replica initialized successfully"
        fi
    fi
fi

# Start connectivity monitor in background after PostgreSQL starts
if [ "${ENABLE_CONNECTIVITY_MONITOR}" = "true" ]; then
    (
        # Wait for PostgreSQL to be ready
        sleep 10
        while ! pg_isready -h localhost -U ${POSTGRES_USER:-postgres} >/dev/null 2>&1; do
            sleep 2
        done
        
        echo "Starting connectivity monitor..."
        /usr/local/bin/check-connectivity.sh
    ) &
fi

# Execute the original PostgreSQL entrypoint
exec docker-entrypoint.sh "$@"
