#!/bin/bash
# Connectivity checker for MariaDB nodes
# Runs on both Primary and Secondary to verify network state

set -e

ROLE="${REPLICATION_MODE}"
PRIMARY_HOST="${PRIMARY_HOST:-mariadb-primary}"
SECONDARY_HOST="${SECONDARY_HOST:-mariadb-secondary}"
MAXSCALE_HOST="${MAXSCALE_HOST:-maxscale}"
CHECK_INTERVAL="${CHECK_INTERVAL:-5}"

# Support Docker secrets - read password from file if _FILE variable is set
if [ -n "${MYSQL_ROOT_PASSWORD_FILE}" ] && [ -f "${MYSQL_ROOT_PASSWORD_FILE}" ]; then
    MYSQL_ROOT_PASSWORD="$(cat "${MYSQL_ROOT_PASSWORD_FILE}")"
fi

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] [$ROLE] $1"
}

# Check if a host is reachable
check_host() {
    local host=$1
    local port=${2:-3306}
    
    if timeout 3 nc -z "$host" "$port" 2>/dev/null; then
        return 0  # Reachable
    else
        return 1  # Unreachable
    fi
}

# Check MySQL/MariaDB is responding
check_mysql() {
    local host=$1
    
    if [ -n "${MYSQL_ROOT_PASSWORD}" ]; then
        if mariadb-admin ping -h "$host" -u root -p"${MYSQL_ROOT_PASSWORD}" --connect-timeout=3 --silent 2>/dev/null; then
            return 0
        fi
    else
        # Try without password for initial connection test
        if mariadb-admin ping -h "$host" --connect-timeout=3 --silent 2>/dev/null; then
            return 0
        fi
    fi
    return 1
}

# Set node to read-only mode
set_read_only() {
    log "Setting node to READ-ONLY mode (isolated from cluster)"
    mysql -u root -p"${MYSQL_ROOT_PASSWORD}" -e "SET GLOBAL read_only = ON;" 2>/dev/null || true
    mysql -u root -p"${MYSQL_ROOT_PASSWORD}" -e "SET GLOBAL super_read_only = ON;" 2>/dev/null || true
}

# Remove read-only mode
set_read_write() {
    log "Setting node to READ-WRITE mode (connected to cluster)"
    mysql -u root -p"${MYSQL_ROOT_PASSWORD}" -e "SET GLOBAL read_only = OFF;" 2>/dev/null || true
    mysql -u root -p"${MYSQL_ROOT_PASSWORD}" -e "SET GLOBAL super_read_only = OFF;" 2>/dev/null || true
}

# Primary node logic
check_primary() {
    local can_reach_secondary=0
    local can_reach_maxscale=0
    local consecutive_failures=0
    local startup_grace_seconds=120  # Allow 2 minutes for secondary to boot and connect
    local startup_time=$(date +%s)
    
    while true; do
        local current_time=$(date +%s)
        local elapsed=$((current_time - startup_time))
        local in_grace_period=0
        
        if [ $elapsed -lt $startup_grace_seconds ]; then
            in_grace_period=1
        fi
        
        # Check connectivity to Secondary
        if check_host "$SECONDARY_HOST" 3306; then
            can_reach_secondary=1
        else
            can_reach_secondary=0
            if [ $in_grace_period -eq 0 ]; then
                log "WARNING: Cannot reach Secondary at $SECONDARY_HOST"
            fi
        fi
        
        # Check connectivity to MaxScale (orchestrator)
        if check_host "$MAXSCALE_HOST" 4006; then
            can_reach_maxscale=1
        else
            can_reach_maxscale=0
            if [ $in_grace_period -eq 0 ]; then
                log "WARNING: Cannot reach MaxScale at $MAXSCALE_HOST"
            fi
        fi
        
        # Decision logic for Primary
        if [ $can_reach_secondary -eq 0 ] && [ $can_reach_maxscale -eq 0 ]; then
            # Isolated from both Secondary and MaxScale
            if [ $in_grace_period -eq 1 ]; then
                # During startup grace period, stay writable to allow secondary to replicate
                log "Startup grace period active ($elapsed/$startup_grace_seconds seconds). Staying read-write for secondary bootstrap."
                consecutive_failures=0
            else
                # Grace period expired, apply isolation logic
                consecutive_failures=$((consecutive_failures + 1))
                log "ISOLATED: Cannot reach Secondary OR MaxScale (attempt $consecutive_failures/3)"
                
                if [ $consecutive_failures -ge 3 ]; then
                    log "CRITICAL: Primary is isolated from cluster. Entering READ-ONLY mode to prevent split-brain."
                    set_read_only
                fi
            fi
        elif [ $can_reach_maxscale -eq 1 ] && [ $can_reach_secondary -eq 0 ]; then
            # Can reach MaxScale but not Secondary
            # Let MaxScale handle failover, stay operational
            log "Secondary unreachable, but MaxScale is available. Staying active."
            consecutive_failures=0
        else
            # Can reach at least Secondary or both
            log "Connectivity OK (Secondary: $can_reach_secondary, MaxScale: $can_reach_maxscale)"
            consecutive_failures=0
            
            # Ensure we're in read-write mode if we recovered
            if [ "${ENABLE_AUTO_RECOVERY}" = "true" ]; then
                set_read_write
            fi
        fi
        
        sleep "$CHECK_INTERVAL"
    done
}

# Secondary node logic
check_secondary() {
    local can_reach_primary=0
    local can_reach_maxscale=0
    local consecutive_primary_failures=0
    
    while true; do
        # Check connectivity to Primary
        if check_host "$PRIMARY_HOST" 3306; then
            can_reach_primary=1
            consecutive_primary_failures=0
        else
            can_reach_primary=0
            consecutive_primary_failures=$((consecutive_primary_failures + 1))
            log "WARNING: Cannot reach Primary at $PRIMARY_HOST (attempt $consecutive_primary_failures)"
        fi
        
        # Check connectivity to MaxScale
        if check_host "$MAXSCALE_HOST" 4006; then
            can_reach_maxscale=1
        else
            can_reach_maxscale=0
            log "WARNING: Cannot reach MaxScale at $MAXSCALE_HOST"
        fi
        
        # Decision logic for Secondary
        if [ $can_reach_primary -eq 0 ] && [ $can_reach_maxscale -eq 1 ]; then
            # Primary is down but can reach MaxScale
            if [ $consecutive_primary_failures -ge 3 ]; then
                log "PRIMARY FAILURE CONFIRMED: Cannot reach Primary, but MaxScale is reachable."
                log "Waiting for MaxScale to promote this node to Primary..."
                # MaxScale will handle promotion
            fi
        elif [ $can_reach_primary -eq 0 ] && [ $can_reach_maxscale -eq 0 ]; then
            # Isolated from both
            log "ISOLATED: Cannot reach Primary OR MaxScale. Staying in READ-ONLY mode."
            set_read_only
        else
            # Can reach Primary (normal operation)
            log "Connectivity OK (Primary: $can_reach_primary, MaxScale: $can_reach_maxscale)"
        fi
        
        sleep "$CHECK_INTERVAL"
    done
}

# Main
log "Starting connectivity monitor for $ROLE node"

# Wait longer for MariaDB to be fully ready (give it time to start)
log "Waiting for MariaDB to be ready..."
sleep 15  # Initial delay to let MariaDB start after initialization

# Now wait for actual connectivity with proper authentication
MAX_WAIT=120
WAITED=0
while true; do
    if [ $WAITED -ge $MAX_WAIT ]; then
        log "ERROR: MariaDB did not become ready within ${MAX_WAIT}s, exiting monitor"
        exit 1
    fi
    
    # Try to connect with root password if available
    if [ -n "${MYSQL_ROOT_PASSWORD}" ]; then
        if mariadb-admin ping -h localhost -u root -p"${MYSQL_ROOT_PASSWORD}" --connect-timeout=3 --silent 2>/dev/null; then
            break
        fi
    else
        # Fallback: just check if socket/port is responsive
        if nc -z localhost 3306 2>/dev/null; then
            sleep 2  # Give it a moment more
            if mariadb-admin ping -h localhost --connect-timeout=3 --silent 2>/dev/null; then
                break
            fi
        fi
    fi
    
    log "Still waiting for MariaDB to be ready... (${WAITED}s/${MAX_WAIT}s)"
    sleep 5
    WAITED=$((WAITED + 5))
done

log "MariaDB is ready, starting connectivity checks"

# Run appropriate checker based on role
if [ "$ROLE" = "master" ]; then
    check_primary
elif [ "$ROLE" = "slave" ]; then
    check_secondary
else
    log "ERROR: Unknown role $ROLE"
    exit 1
fi
