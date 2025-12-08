#!/bin/bash

# Check if PostgreSQL is running
if ! pg_isready -h localhost -U ${POSTGRES_USER:-postgres} >/dev/null 2>&1; then
    echo "PostgreSQL is not responding"
    exit 1
fi

# Check replication status if replica
if [ "${REPLICATION_MODE}" = "replica" ]; then
    RECOVERY_STATUS=$(psql -U ${POSTGRES_USER:-postgres} -t -c "SELECT pg_is_in_recovery();" 2>/dev/null | tr -d ' ')
    
    if [ "$RECOVERY_STATUS" != "t" ]; then
        echo "Replica is not in recovery mode"
        exit 1
    fi
    
    # Check replication lag
    LAG=$(psql -U ${POSTGRES_USER:-postgres} -t -c "SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp()));" 2>/dev/null | tr -d ' ')
    
    if [ -n "$LAG" ] && [ $(echo "$LAG > 60" | bc) -eq 1 ]; then
        echo "Replication lag is too high: ${LAG}s"
        exit 1
    fi
fi

echo "PostgreSQL is healthy"
exit 0
