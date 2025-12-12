#!/bin/bash
set -e

# This script runs only on the primary during initialization
# It creates the replication user if it doesn't exist

if [ "${REPLICATION_MODE}" = "primary" ]; then
    # Read replication password from secret file
    if [ -n "$REPLICATION_PASSWORD_FILE" ] && [ -f "$REPLICATION_PASSWORD_FILE" ]; then
        REPLICATION_PASSWORD="$(cat "$REPLICATION_PASSWORD_FILE")"
    fi

    if [ -n "$REPLICATION_PASSWORD" ] && [ -n "$REPLICATION_USER" ]; then
        psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
            DO \$\$
            BEGIN
                IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '${REPLICATION_USER}') THEN
                    CREATE ROLE ${REPLICATION_USER} WITH REPLICATION LOGIN PASSWORD '${REPLICATION_PASSWORD}';
                    RAISE NOTICE 'Replication user ${REPLICATION_USER} created';
                ELSE
                    RAISE NOTICE 'Replication user ${REPLICATION_USER} already exists';
                END IF;
            END
            \$\$;
EOSQL
    fi
fi
