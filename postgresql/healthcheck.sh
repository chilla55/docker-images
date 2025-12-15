#!/bin/bash
# PostgreSQL healthcheck script

POSTGRES_PASSWORD=$(cat /run/secrets/postgres_password 2>/dev/null || echo "${POSTGRES_PASSWORD}")

# Check if PostgreSQL is responding
if PGPASSWORD="${POSTGRES_PASSWORD}" psql -U postgres -c "SELECT 1" &>/dev/null; then
    exit 0
else
    exit 1
fi
