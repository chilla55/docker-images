#!/bin/bash
# Connectivity checker for PostgreSQL nodes

set -e

ROLE="${REPLICATION_MODE}"
PRIMARY_HOST="${PRIMARY_HOST:-postgresql-primary}"
SECONDARY_HOST="${SECONDARY_HOST:-postgresql-secondary}"
PGPOOL_HOST="${PGPOOL_HOST:-pgpool}"
CHECK_INTERVAL="${CHECK_INTERVAL:-3}"
FAILURE_THRESHOLD="${FAILURE_THRESHOLD:-3}"

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] [$ROLE] $1"
}

check_host() {
    local host=$1
    local port=${2:-5432}
    
    if timeout 3 nc -z "$host" "$port" 2>/dev/null; then
        return 0
    else
        return 1
    fi
}

set_read_only() {
    log "Setting database to READ-ONLY mode (isolated from cluster)"
    psql -U ${POSTGRES_USER:-postgres} -c "ALTER SYSTEM SET default_transaction_read_only = on;" 2>/dev/null || true
    psql -U ${POSTGRES_USER:-postgres} -c "SELECT pg_reload_conf();" 2>/dev/null || true
}

set_read_write() {
    log "Setting database to READ-WRITE mode (connected to cluster)"
    psql -U ${POSTGRES_USER:-postgres} -c "ALTER SYSTEM SET default_transaction_read_only = off;" 2>/dev/null || true
    psql -U ${POSTGRES_USER:-postgres} -c "SELECT pg_reload_conf();" 2>/dev/null || true
}
check_primary() {
    local can_reach_secondary=0
    local can_reach_pgpool=0
    local consecutive_failures=0
    local consecutive_pgpool_failures=0
    
    while true; do
        local secondary_ok=0
        local pgpool_ok=0
        
        if check_host "$SECONDARY_HOST" 5432; then
            secondary_ok=1
        fi
        
        if check_host "$PGPOOL_HOST" 9999; then
            pgpool_ok=1
            consecutive_pgpool_failures=0
        else
            consecutive_pgpool_failures=$((consecutive_pgpool_failures + 1))
        fi
        
        # Check if secondary has been promoted (no longer in recovery mode)
        if [ $secondary_ok -eq 1 ]; then
            local secondary_is_replica=$(PGPASSWORD="${REPLICATION_PASSWORD}" psql -h "$SECONDARY_HOST" -U "${REPLICATION_USER}" -p 5432 -t -c "SELECT pg_is_in_recovery();" 2>/dev/null | tr -d '[:space:]')
            if [ "$secondary_is_replica" = "f" ]; then
                log "CRITICAL: Secondary has been promoted to primary! Going READ-ONLY immediately."
                set_read_only
                sleep "$CHECK_INTERVAL"
                continue
            fi
        fi
        
        # Primary is healthy if it can reach EITHER secondary OR pgpool
        if [ $secondary_ok -eq 1 ] || [ $pgpool_ok -eq 1 ]; then
            consecutive_failures=0
            
            # Warn if PgPool is unreachable for extended period
            if [ $consecutive_pgpool_failures -ge 6 ]; then
                log "WARNING: PgPool unreachable for $((consecutive_pgpool_failures * CHECK_INTERVAL))s - failover capability degraded"
            fi
            
            log "Connectivity OK (Secondary: $secondary_ok, PgPool: $pgpool_ok)"
            
            if [ "${ENABLE_AUTO_RECOVERY}" = "true" ]; then
                set_read_write
            fi
        else
            consecutive_failures=$((consecutive_failures + 1))
            log "WARNING: Cannot reach Secondary OR PgPool (attempt $consecutive_failures/$FAILURE_THRESHOLD)"
            
            if [ $consecutive_failures -ge $FAILURE_THRESHOLD ]; then
                log "CRITICAL: Primary isolated from cluster. Entering READ-ONLY mode."
                set_read_only
            fi
        fi
        
        sleep "$CHECK_INTERVAL"
    done
}

check_secondary() {
    local can_reach_primary=0
    local consecutive_primary_failures=0
    
    while true; do
        if check_host "$PRIMARY_HOST" 5432; then
            can_reach_primary=1
            consecutive_primary_failures=0
            log "Connectivity OK (Primary reachable)"
        else
            can_reach_primary=0
            consecutive_primary_failures=$((consecutive_primary_failures + 1))
            log "WARNING: Cannot reach Primary (attempt $consecutive_primary_failures/$FAILURE_THRESHOLD)"
            
            if [ $consecutive_primary_failures -ge $FAILURE_THRESHOLD ]; then
                log "PRIMARY FAILURE CONFIRMED: Waiting for PgPool to promote this replica..."
            fi
        fi
        
        sleep "$CHECK_INTERVAL"
    done
}

# Main
log "Starting connectivity monitor for $ROLE node"

# Wait for PostgreSQL to be ready
while ! pg_isready -h localhost -U ${POSTGRES_USER:-postgres} >/dev/null 2>&1; do
    log "Waiting for PostgreSQL to be ready..."
    sleep 2
done

log "PostgreSQL is ready, starting connectivity checks"

if [ "$ROLE" = "primary" ]; then
    check_primary
elif [ "$ROLE" = "replica" ]; then
    check_secondary
else
    log "ERROR: Unknown role $ROLE"
    exit 1
fi
