#!/bin/bash

REPO_URL="https://github.com/6th-Maroon-Division/Homepage.git"
APP_DIR="/app/repo"
MAINTENANCE_PAGE="/maintenance.html"
MAINTENANCE_ACTIVE="/tmp/maintenance_active"
STATUS_FILE="/tmp/status.json"
UPDATE_CHECK_INTERVAL="${UPDATE_CHECK_INTERVAL:-300}"  # Check every 5 minutes by default
MAINTENANCE_PORT="${MAINTENANCE_PORT:-$((PORT+1))}"  # Maintenance on web port + 1 by default
SERVICE_NAME="${SERVICE_NAME:-orbat}"  # Service name for registration
REGISTRY_HOST="${REGISTRY_HOST:-proxy}"  # go-proxy registry host
REGISTRY_PORT="${REGISTRY_PORT:-81}"    # go-proxy registry port (TCP)
DOMAINS="${DOMAINS:-orbat.chilla55.de}"  # Comma-separated domain list for routing
ROUTE_PATH="${ROUTE_PATH:-/}"            # Route path
BACKEND_HOST="${BACKEND_HOST:-orbat}"    # Hostname used in backend URL
BACKEND_URL="http://${BACKEND_HOST}:${PORT}"  # Backend URL for proxy routing
SESSION_ID=""
SESSION_FILE="/run/orbat-registry-session-id"
UPDATE_IN_PROGRESS=0

# Environment variable for extended info display (default: false)
SHOW_EXTENDED_INFO="${SHOW_EXTENDED_INFO:-false}"

echo "[Orbat] Starting entrypoint..."
echo "[Orbat] Service: $SERVICE_NAME, Port: $PORT, Maintenance: $MAINTENANCE_PORT"
echo "[Orbat] Auto-update check interval: ${UPDATE_CHECK_INTERVAL}s"

# Cleanup function
cleanup() {
    echo "[Orbat] Shutting down, closing persistent connection..."
    if [ -n "$REGISTRY_KEEPALIVE_PID" ]; then
        kill $REGISTRY_KEEPALIVE_PID 2>/dev/null || true
        wait $REGISTRY_KEEPALIVE_PID 2>/dev/null || true
    fi
    if [ -n "$SESSION_ID" ]; then
        send_registry_command "SHUTDOWN|$SESSION_ID" >/dev/null 2>&1
    fi
    if [ -n "$REGISTRY_FD_OPEN" ]; then
        exec 3>&-
    fi
}

trap cleanup EXIT INT TERM

# Construct DATABASE_URL from environment variables and secret
if [ -f "/run/secrets/database_password" ]; then
    DATABASE_PASSWORD=$(cat /run/secrets/database_password)
    export DATABASE_URL="postgresql://${DATABASE_USER}:${DATABASE_PASSWORD}@${DATABASE_HOST}:${DATABASE_PORT}/${DATABASE_NAME}?schema=${DATABASE_SCHEMA}"
    echo "[Orbat] Database URL constructed from environment and secrets"
else
    echo "[Orbat] Warning: database_password secret not found!"
fi

# Function to update status
update_status() {
    local step="$1"
    local message="$2"
    local progress="$3"
    local details="${4:-}"
    
    cat > "$STATUS_FILE" <<EOF
{
  "step": "$step",
  "message": "$message",
  "progress": $progress,
  "details": "$details",
  "timestamp": "$(date -Iseconds)",
  "showExtended": $SHOW_EXTENDED_INFO
}
EOF
}

# Registry helpers for go-proxy TCP service registry
open_registry_connection() {
    if [ -n "$REGISTRY_FD_OPEN" ]; then
        return 0
    fi

    if exec 3<>/dev/tcp/$REGISTRY_HOST/$REGISTRY_PORT; then
        REGISTRY_FD_OPEN=1
        echo "[Orbat] Connected to registry at ${REGISTRY_HOST}:${REGISTRY_PORT}"
        return 0
    fi

    echo "[Orbat] Failed to connect to registry at ${REGISTRY_HOST}:${REGISTRY_PORT}"
    return 1
}

send_registry_command() {
    local cmd="$1"
    if [ -z "$REGISTRY_FD_OPEN" ]; then
        return 1
    fi

    echo -ne "${cmd}\n" >&3

    local response=""
    local retries=0
    while [ $retries -lt 3 ]; do
        if read -t 5 -r response <&3 2>/dev/null; then
            if [ -n "$response" ]; then
                echo "$response"
                return 0
            fi
        fi
        retries=$((retries + 1))
    done

    echo ""
    return 1
}

register_with_proxy() {
    echo "[Orbat] Registering service with go-proxy registry..."
    if ! open_registry_connection; then
        echo "[Orbat] Registry connection failed; skipping registration"
        return 1
    fi

    # If we have a previous session ID, try to reconnect first
    if [ -z "$SESSION_ID" ] && [ -f "$SESSION_FILE" ]; then
        SESSION_ID="$(cat "$SESSION_FILE" 2>/dev/null || echo "")"
    fi

    local response=""
    if [ -n "$SESSION_ID" ]; then
        response="$(send_registry_command "RECONNECT|$SESSION_ID")"
        if [ "$response" = "OK" ]; then
            echo "[Orbat] Reconnected to registry with session: $SESSION_ID"
        else
            echo "[Orbat] Reconnect failed (${response:-no response}); re-registering"
            SESSION_ID=""
        fi
    fi

    if [ -z "$SESSION_ID" ]; then
        local register_cmd="REGISTER|$SERVICE_NAME|$BACKEND_HOST|$PORT|$MAINTENANCE_PORT"
        response="$(send_registry_command "$register_cmd")"
        if [[ "$response" =~ ^ACK\|(.*)$ ]]; then
            SESSION_ID="${BASH_REMATCH[1]}"
            echo "[Orbat] Registered with session: $SESSION_ID"
            echo -n "$SESSION_ID" > "$SESSION_FILE" 2>/dev/null || true
        else
            echo "[Orbat] Registry registration failed: $response"
            return 1
        fi
    fi

    local domains_clean=${DOMAINS// /}
    local route_cmd="ROUTE|$SESSION_ID|$domains_clean|$ROUTE_PATH|$BACKEND_URL"
    response="$(send_registry_command "$route_cmd")"
    echo "[Orbat] Route registration: ${response:-no response}" 

    # Apply a couple of sensible defaults
    send_registry_command "OPTIONS|$SESSION_ID|timeout|60s" >/dev/null 2>&1
    send_registry_command "OPTIONS|$SESSION_ID|compression|true" >/dev/null 2>&1
    send_registry_command "OPTIONS|$SESSION_ID|http2|true" >/dev/null 2>&1

    return 0
}

# Background process to keep registry connection alive by reading from socket
maintain_registry_connection() {
    echo "[Orbat] Registry keepalive monitor started"
    while true; do
        if [ -z "$REGISTRY_FD_OPEN" ] || [ -z "$SESSION_ID" ]; then
            sleep 5
            continue
        fi
        
        # Just keep reading from the socket to detect disconnection
        # The socket should stay open as long as we keep it open
        if ! read -t 60 -r line <&3 2>/dev/null; then
            if [ $? -eq 124 ]; then
                # Timeout reading - connection is still alive
                :
            else
                # Error or EOF - connection closed
                echo "[Orbat] Registry connection lost, attempting to reconnect..."
                REGISTRY_FD_OPEN=""
                if open_registry_connection; then
                    if [ -n "$SESSION_ID" ]; then
                        local resp="$(send_registry_command "RECONNECT|$SESSION_ID")"
                        if [ "$resp" = "OK" ]; then
                            echo "[Orbat] Reconnected to registry"
                        else
                            echo "[Orbat] Reconnect failed, will re-register"
                            SESSION_ID=""
                        fi
                    fi
                fi
            fi
        fi
        sleep 5
    done
}

enter_proxy_maintenance() {
    if [ -z "$SESSION_ID" ]; then
        return 0
    fi

    local response="$(send_registry_command "MAINT_ENTER|$SESSION_ID")"
    echo "[Orbat] Proxy maintenance enter: ${response:-no response}"
}

exit_proxy_maintenance() {
    if [ -z "$SESSION_ID" ]; then
        return 0
    fi

    local attempt=0
    local response=""
    while [ $attempt -lt 3 ]; do
        response="$(send_registry_command "MAINT_EXIT|$SESSION_ID")"
        if [ "$response" = "MAINT_OK" ]; then
            echo "[Orbat] Proxy maintenance exit acknowledged"
            return 0
        fi
        attempt=$((attempt + 1))
        echo "[Orbat] Proxy maintenance exit retry $attempt (response: ${response:-no response})"
        sleep 1
    done

    echo "[Orbat] Warning: no MAINT_OK after exit attempts (last: ${response:-none})"
    return 0
}

# Function to show maintenance page on separate port with handshake API
show_maintenance() {
    echo "[Orbat] Starting maintenance server on port $MAINTENANCE_PORT..."
    touch "$MAINTENANCE_ACTIVE"
    update_status "initializing" "Starting maintenance mode" 0 "Preparing to update application"
    
    # Start HTTP server for maintenance page and status API
    ( cd / && exec node -e "
        const http = require('http');
        const fs = require('fs');
        
        const httpServer = http.createServer((req, res) => {
            // Status API endpoint
            if (req.url === '/api/status') {
                res.writeHead(200, {'Content-Type': 'application/json', 'Access-Control-Allow-Origin': '*'});
                try {
                    const status = fs.readFileSync('$STATUS_FILE', 'utf8');
                    res.end(status);
                } catch (e) {
                    res.end('{\"step\":\"unknown\",\"message\":\"Initializing...\",\"progress\":0,\"showExtended\":false}');
                }
                return;
            }
            
            // Maintenance page
            if (fs.existsSync('$MAINTENANCE_ACTIVE')) {
                res.writeHead(503, {'Content-Type': 'text/html'});
                res.end(fs.readFileSync('$MAINTENANCE_PAGE', 'utf8'));
            } else {
                res.writeHead(404);
                res.end('Service starting...');
            }
        });
        httpServer.listen($MAINTENANCE_PORT);
        console.log('HTTP maintenance server on port ' + $MAINTENANCE_PORT);
    " ) &
    MAINTENANCE_PID=$!
    sleep 2
    echo "[Orbat] Maintenance server ready on port $MAINTENANCE_PORT (PID: $MAINTENANCE_PID)"
}

# Function to stop maintenance and switch back to main service
stop_maintenance() {
    echo "[Orbat] Stopping maintenance and switching back to main service..."
    rm -f "$MAINTENANCE_ACTIVE"
    if [ -n "$MAINTENANCE_PID" ]; then
        kill $MAINTENANCE_PID 2>/dev/null || true
        wait $MAINTENANCE_PID 2>/dev/null || true
        echo "[Orbat] Maintenance server stopped"
    fi
}

# Function to perform update
perform_update() {
    echo "[Orbat] Performing update..."
    cd "$APP_DIR"
    
    UPDATE_IN_PROGRESS=1
    # Start maintenance server then inform proxy to route to it
    show_maintenance
    enter_proxy_maintenance
    
    update_status "pulling" "Pulling latest changes" 10 "Downloading updated code from repository"
    git pull origin main
    
    # Install dependencies
    echo "[Orbat] Installing dependencies..."
    update_status "dependencies" "Installing dependencies" 25 "Running npm ci to install packages"
    npm ci --production=false
    
    # Generate Prisma client
    echo "[Orbat] Generating Prisma client..."
    update_status "prisma-generate" "Generating Prisma client" 45 "Creating database client from schema"
    npx prisma generate
    
    # Run database migrations
    echo "[Orbat] Running database migrations..."
    update_status "migrations" "Running database migrations" 60 "Applying schema changes to database"
    npx prisma migrate deploy
    
    # Build Next.js app
    echo "[Orbat] Building Next.js application..."
    update_status "building" "Building Next.js application" 75 "Compiling TypeScript and optimizing assets"
    npm run build
    
    update_status "finalizing" "Finalizing update" 95 "Restarting Next.js server"

    echo "[Orbat] Update completed - restarting application..."

    # Stop current Next.js process; app loop will restart after update flag clears
    if [ -n "$NEXTJS_PID" ]; then
        kill $NEXTJS_PID 2>/dev/null || true
        wait $NEXTJS_PID 2>/dev/null || true
    fi

    # Tell proxy to switch back to main, then stop maintenance server
    exit_proxy_maintenance
    stop_maintenance
    UPDATE_IN_PROGRESS=0
}

# Background update checker
start_update_checker() {
    echo "[Orbat] Starting background update checker..."
    (
        while true; do
            sleep $UPDATE_CHECK_INTERVAL
            
            echo "[Orbat] Checking for updates (periodic check)..."
            cd "$APP_DIR"
            git fetch origin main 2>/dev/null || continue
            
            LOCAL=$(git rev-parse HEAD 2>/dev/null || echo "")
            REMOTE=$(git rev-parse origin/main 2>/dev/null || echo "")
            
            if [ ! -z "$LOCAL" ] && [ ! -z "$REMOTE" ] && [ "$LOCAL" != "$REMOTE" ]; then
                echo "[Orbat] Updates detected! Triggering update..."
                perform_update
            fi
        done
    ) &
    UPDATE_CHECKER_PID=$!
    echo "[Orbat] Update checker running with PID $UPDATE_CHECKER_PID"
}

# Check if this is first run or update
if [ ! -d "$APP_DIR/.git" ]; then
    echo "[Orbat] First run - cloning repository..."
    update_status "cloning" "Cloning repository from GitHub" 5 "Getting latest code from 6th-Maroon-Division/Homepage"
    
    if ! git clone "$REPO_URL" "$APP_DIR"; then
        echo "[Orbat] ERROR: Failed to clone repository"
        exit_proxy_maintenance
        stop_maintenance
        sleep 30
        exit 1
    fi
    cd "$APP_DIR" || { echo "[Orbat] ERROR: Failed to cd into $APP_DIR"; exit_proxy_maintenance; stop_maintenance; exit 1; }
else
    echo "[Orbat] Checking for updates..."
    cd "$APP_DIR" || { echo "[Orbat] ERROR: Failed to cd into $APP_DIR"; exit_proxy_maintenance; stop_maintenance; exit 1; }
    
    # Check if there are updates
    update_status "checking" "Checking for updates" 10 "Fetching latest commits from GitHub"
    if ! git fetch origin main 2>/dev/null; then
        echo "[Orbat] WARNING: git fetch failed, continuing with local version"
    else
        LOCAL=$(git rev-parse HEAD 2>/dev/null)
        REMOTE=$(git rev-parse origin/main 2>/dev/null)
        
        if [ ! -z "$LOCAL" ] && [ ! -z "$REMOTE" ] && [ "$LOCAL" != "$REMOTE" ]; then
            echo "[Orbat] Updates detected - pulling changes..."
            update_status "pulling" "Pulling latest changes" 15 "Downloading updated code from repository"
            if ! git pull origin main 2>&1 | tee -a /tmp/orbat.log; then
                echo "[Orbat] WARNING: git pull failed, continuing with current version"
            fi
        else
            echo "[Orbat] No updates found"
            update_status "ready" "Ready to start" 20 "Preparing to build and run application"
        fi
    fi
fi

# Install dependencies
echo "[Orbat] Installing dependencies..."
update_status "dependencies" "Installing dependencies" 30 "Running npm ci to install packages"
if ! npm ci --production=false 2>&1 | tee -a /tmp/orbat.log; then
    echo "[Orbat] ERROR: npm ci failed"
    exit_proxy_maintenance
    stop_maintenance
    sleep 30
    exit 1
fi

# Generate Prisma client
echo "[Orbat] Generating Prisma client..."
update_status "prisma-generate" "Generating Prisma client" 45 "Creating database client from schema"
if ! npx prisma generate 2>&1 | tee -a /tmp/orbat.log; then
    echo "[Orbat] ERROR: Prisma generate failed"
    exit_proxy_maintenance
    stop_maintenance
    sleep 30
    exit 1
fi

# Run database migrations
echo "[Orbat] Running database migrations..."
update_status "migrations" "Running database migrations" 60 "Applying schema changes to database"
if ! npx prisma migrate deploy 2>&1 | tee -a /tmp/orbat.log; then
    echo "[Orbat] ERROR: Prisma migrate deploy failed"
    exit_proxy_maintenance
    stop_maintenance
    sleep 30
    exit 1
fi

# Build Next.js app
echo "[Orbat] Building Next.js application..."
update_status "building" "Building Next.js application" 75 "Compiling TypeScript and optimizing assets"
if ! npm run build 2>&1 | tee -a /tmp/orbat.log; then
    echo "[Orbat] ERROR: npm run build failed"
    exit_proxy_maintenance
    stop_maintenance
    sleep 30
    exit 1
fi

# Exit maintenance mode and stop progress page
echo "[Orbat] Startup complete, switching to main service..."
update_status "startup-complete" "Startup complete" 95 "Switching to main application service"
exit_proxy_maintenance
stop_maintenance

run_app_loop() {
    echo "[Orbat] Starting Next.js supervisor loop..."
    while true; do
        # Defer starting app while updates are in progress
        while [ "$UPDATE_IN_PROGRESS" -eq 1 ]; do sleep 1; done
        echo "[Orbat] Launching Next.js..."
        update_status "running" "Application running" 100 "Service is now available"
        npm start
        echo "[Orbat] Next.js exited (code: $?), restarting in 5s..."
        sleep 5
    done
}

# Start update checker in background
start_update_checker

# Show maintenance page and register early
echo "[Orbat] Entering startup maintenance mode..."
show_maintenance
update_status "startup" "Starting up" 5 "Registering with proxy and initializing services"

# Register with go-proxy (persistent TCP registry)
if ! register_with_proxy; then
    echo "[Orbat] Warning: proxy registration failed"
fi

# Start registry connection keepalive
maintain_registry_connection &
REGISTRY_KEEPALIVE_PID=$!
echo "[Orbat] Registry keepalive monitor started (PID: $REGISTRY_KEEPALIVE_PID)"

# Enter maintenance mode to show progress page
enter_proxy_maintenance
