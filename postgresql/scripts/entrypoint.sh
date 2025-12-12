#!/bin/bash
set -eo pipefail

# Load secrets from *_FILE paths if provided
# Avoid exporting POSTGRES_PASSWORD when POSTGRES_PASSWORD_FILE is set,
# since the official postgres entrypoint treats them as mutually exclusive.
if [ -n "$REPLICATION_PASSWORD_FILE" ] && [ -f "$REPLICATION_PASSWORD_FILE" ]; then
    export REPLICATION_PASSWORD="$(cat "$REPLICATION_PASSWORD_FILE")"
fi

# Create log directory
mkdir -p /var/log/postgresql
chown -R postgres:postgres /var/log/postgresql

# Generate SSL certificates signed by root CA on first run
if [ ! -f /var/lib/postgresql/server.crt ] || [ ! -f /var/lib/postgresql/server.key ]; then
    echo "Generating SSL certificate for $(hostname)..."
    
    # Determine hostname for certificate
    SERVER_HOSTNAME=$(hostname)
    
    # Generate server private key
    openssl genrsa -out /var/lib/postgresql/server.key 2048
    
    # Create certificate signing request (CSR)
    openssl req -new -key /var/lib/postgresql/server.key \
        -out /var/lib/postgresql/server.csr \
        -subj "/C=DE/ST=State/L=City/O=Docker/CN=${SERVER_HOSTNAME}"
    
    # Sign certificate with root CA (mounted via /mnt/storagebox/rootca)
    if [ -f /var/lib/postgresql/rootca/ca-cert.pem ] && [ -f /var/lib/postgresql/rootca/ca-key.pem ]; then
        # Copy CA files to writable location temporarily for serial file generation
        cp /var/lib/postgresql/rootca/ca-cert.pem /tmp/ca-cert.pem
        cp /var/lib/postgresql/rootca/ca-key.pem /tmp/ca-key.pem
        
        openssl x509 -req -days 3650 \
            -in /var/lib/postgresql/server.csr \
            -CA /tmp/ca-cert.pem \
            -CAkey /tmp/ca-key.pem \
            -CAcreateserial \
            -out /var/lib/postgresql/server.crt
        
        # Clean up temp files
        rm -f /tmp/ca-cert.pem /tmp/ca-key.pem /tmp/ca-cert.srl
        
        echo "✓ SSL certificate signed by root CA"
    else
        echo "WARNING: Root CA not found, generating self-signed cert instead"
        openssl x509 -req -days 3650 -in /var/lib/postgresql/server.csr \
            -signkey /var/lib/postgresql/server.key \
            -out /var/lib/postgresql/server.crt
    fi
    
    # Clean up CSR
    rm -f /var/lib/postgresql/server.csr
    
    # Set proper permissions
    chmod 600 /var/lib/postgresql/server.key /var/lib/postgresql/server.crt
    chown postgres:postgres /var/lib/postgresql/server.key /var/lib/postgresql/server.crt
fi

## Configure primary or initialize replica BEFORE launching postgres
if [ "${REPLICATION_MODE}" = "primary" ]; then
    echo "Configuring as PRIMARY server..."

    # Create archive directory (used by archive_command)
    mkdir -p /var/lib/postgresql/archive
    chown -R postgres:postgres /var/lib/postgresql/archive

    # Append replication settings if database already initialized
    if [ -s "$PGDATA/PG_VERSION" ]; then
        if ! grep -q "Replication Settings" ${PGDATA}/postgresql.conf 2>/dev/null; then
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
    fi

elif [ "${REPLICATION_MODE}" = "replica" ]; then
    echo "Configuring as REPLICA server..."

    # Wait for primary to be ready before attempting basebackup
    echo "Waiting for primary to be ready (host=${PRIMARY_HOST} port=${PRIMARY_PORT})..."
    while true; do
        # Use /dev/tcp with extended timeout (10 sec) to check if port is open
        if timeout 10 bash -c "echo > /dev/tcp/${PRIMARY_HOST}/${PRIMARY_PORT}" 2>/dev/null; then
            echo "Primary is ready!"
            break
        else
            echo "Primary not ready, waiting..."
        fi
        sleep 2
    done

    # Initialize replica if data directory is empty (first run)
    if [ ! -s "$PGDATA/PG_VERSION" ]; then
        echo "Initializing replica from primary..."
        rm -rf ${PGDATA}/*

        PGPASSWORD=${REPLICATION_PASSWORD} pg_basebackup \
            -h ${PRIMARY_HOST} \
            -p ${PRIMARY_PORT} \
            -U ${REPLICATION_USER} \
            -D ${PGDATA} \
            -Fp -Xs -P -R

        echo "Replica initialized successfully"
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

# Always use the same config file path and pg_hba.conf from /etc/postgresql
PG_ARGS="-c hba_file=/etc/postgresql/pg_hba.conf -c config_file=/etc/postgresql/postgresql.conf"

# Execute the original PostgreSQL entrypoint with explicit config paths
exec docker-entrypoint.sh postgres $PG_ARGS
