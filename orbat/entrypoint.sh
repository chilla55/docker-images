#!/bin/bash
set -e

REPO_URL="https://github.com/6th-Maroon-Division/Homepage.git"
APP_DIR="/app/repo"
MAINTENANCE_PAGE="/maintenance.html"
MAINTENANCE_ACTIVE="/tmp/maintenance_active"
STATUS_FILE="/tmp/status.json"
MAINTENANCE_APPROVED="/tmp/maintenance_approved"
SWITCHBACK_APPROVED="/tmp/switchback_approved"
UPDATE_CHECK_INTERVAL="${UPDATE_CHECK_INTERVAL:-300}"  # Check every 5 minutes by default
MAINTENANCE_PORT="${MAINTENANCE_PORT:-3001}"  # Maintenance on separate port
NGINX_HOST="${NGINX_HOST:-nginx_nginx}"  # Nginx service name for notifications
SERVICE_NAME="${SERVICE_NAME:-orbat}"  # Service name for registration

# Environment variable for extended info display (default: false)
SHOW_EXTENDED_INFO="${SHOW_EXTENDED_INFO:-false}"

echo "[Orbat] Starting entrypoint..."
echo "[Orbat] Service: $SERVICE_NAME, Port: $PORT, Maintenance: $MAINTENANCE_PORT"
echo "[Orbat] Auto-update check interval: ${UPDATE_CHECK_INTERVAL}s"

# Cleanup function
cleanup() {
    echo "[Orbat] Shutting down, closing persistent connection..."
    if [ ! -z "$REGISTRATION_PID" ]; then
        kill $REGISTRATION_PID 2>/dev/null || true
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

# Register with nginx and maintain persistent connection
register_with_nginx() {
    echo "[Orbat] Registering with nginx and establishing persistent connection..."
    
    local nginx_port=$((PORT + 1))
    # Format: REGISTER|service_name|hostname|service_port|maintenance_port
    local message="REGISTER|$SERVICE_NAME|$(hostname)|$PORT|$MAINTENANCE_PORT"
    
    # Keep connection open in background - when this dies, nginx knows service is down
    ( 
        echo "$message" | nc "$NGINX_HOST" "$nginx_port" 2>/dev/null
        # Connection stays open until nginx or this process dies
    ) &
    
    REGISTRATION_PID=$!
    echo "[Orbat] Registered with nginx (persistent connection PID: $REGISTRATION_PID)"
    
    # Give it a moment to establish
    sleep 2
}

# Request maintenance mode from nginx via TCP (handshake step 1)
request_maintenance_mode() {
    echo "[Orbat] Requesting maintenance mode from nginx via TCP..."
    rm -f "$MAINTENANCE_APPROVED"
    
    local nginx_port=$((PORT + 1))
    # Send: MAINT_ENTER|hostname:port|maintenance_port
    local message="MAINT_ENTER|$(hostname):$PORT|$MAINTENANCE_PORT"
    
    if echo "$message" | timeout 5 nc "$NGINX_HOST" "$nginx_port" 2>/dev/null | grep -q "ACK"; then
        echo "[Orbat] Maintenance mode request sent to nginx"
        
        # Wait for nginx approval (it will connect to our port and send MAINT_APPROVED)
        local wait_count=0
        while [ $wait_count -lt 30 ]; do
            if [ -f "$MAINTENANCE_APPROVED" ]; then
                echo "[Orbat] Maintenance mode approved by nginx"
                return 0
            fi
            sleep 1
            wait_count=$((wait_count + 1))
        done
        
        echo "[Orbat] Warning: No approval received from nginx, proceeding anyway"
        return 1
    else
        echo "[Orbat] Could not contact nginx, proceeding with local maintenance"
        return 1
    fi
}

# Notify nginx that main service is ready via TCP (handshake step 1)
notify_service_ready() {
    echo "[Orbat] Notifying nginx that main service is ready via TCP..."
    rm -f "$SWITCHBACK_APPROVED"
    
    local nginx_port=$((PORT + 1))
    # Send: MAINT_EXIT|hostname:port
    local message="MAINT_EXIT|$(hostname):$PORT"
    
    if echo "$message" | timeout 5 nc "$NGINX_HOST" "$nginx_port" 2>/dev/null | grep -q "ACK"; then
        echo "[Orbat] Ready notification sent to nginx"
        
        # Wait for nginx to confirm switchback
        local wait_count=0
        while [ $wait_count -lt 30 ]; do
            if [ -f "$SWITCHBACK_APPROVED" ]; then
                echo "[Orbat] Switchback approved by nginx"
                return 0
            fi
            sleep 1
            wait_count=$((wait_count + 1))
        done
        
        echo "[Orbat] Warning: No switchback confirmation from nginx"
        return 1
    else
        echo "[Orbat] Could not contact nginx for switchback"
        return 1
    fi
}

# Function to show maintenance page on separate port with handshake API
show_maintenance() {
    echo "[Orbat] Starting maintenance server on port $MAINTENANCE_PORT..."
    touch "$MAINTENANCE_ACTIVE"
    update_status "initializing" "Starting maintenance mode" 0 "Preparing to update application"
    
    # Start TCP listener for handshake + HTTP server for maintenance page
    ( cd / && exec node -e "
        const http = require('http');
        const net = require('net');
        const fs = require('fs');
        
        // TCP listener for handshake protocol
        const tcpServer = net.createServer((socket) => {
            socket.on('data', (data) => {
                const message = data.toString().trim();
                console.log('[Maintenance] TCP message:', message);
                
                if (message === 'MAINT_APPROVED') {
                    console.log('[Maintenance] Nginx approved maintenance mode');
                    fs.writeFileSync('$MAINTENANCE_APPROVED', Date.now().toString());
                    socket.write('ACK\n');
                } else if (message === 'SWITCHBACK_APPROVED') {
                    console.log('[Maintenance] Nginx approved switchback to main service');
                    fs.writeFileSync('$SWITCHBACK_APPROVED', Date.now().toString());
                    socket.write('ACK\n');
                    setTimeout(() => process.exit(0), 1000);
                }
                socket.end();
            });
        });
        tcpServer.listen($MAINTENANCE_PORT);
        console.log('TCP handshake listener on port $MAINTENANCE_PORT');
        
        // HTTP server for maintenance page and status API
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
        httpServer.listen($MAINTENANCE_PORT + 1000);  // HTTP on different port to avoid conflict
        console.log('HTTP maintenance server on port ' + ($MAINTENANCE_PORT + 1000));
    " ) &
    MAINTENANCE_PID=$!
    sleep 2
    echo "[Orbat] Maintenance server ready on port $MAINTENANCE_PORT (PID: $MAINTENANCE_PID)"
}

# Function to stop maintenance and switch back to main service
stop_maintenance() {
    echo "[Orbat] Stopping maintenance and switching back to main service..."
    rm -f "$MAINTENANCE_ACTIVE"
    
    # Notify nginx that we're ready and wait for approval
    notify_service_ready
    
    # Maintenance server will exit on switchback approval
    if [ ! -z "$MAINTENANCE_PID" ]; then
        wait $MAINTENANCE_PID 2>/dev/null || true
        echo "[Orbat] Maintenance server stopped"
    fi
}

# Function to perform update
perform_update() {
    echo "[Orbat] Performing update..."
    cd "$APP_DIR"
    
    # Request maintenance mode from nginx first
    request_maintenance_mode
    
    # Start maintenance server
    show_maintenance
    
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
    
    # Stop maintenance
    update_status "finalizing" "Finalizing update" 95 "Restarting Next.js server"
    stop_maintenance
    
    echo "[Orbat] Update completed - restarting application..."
    
    # Kill the current Next.js process to restart it
    if [ ! -z "$NEXTJS_PID" ]; then
        kill $NEXTJS_PID 2>/dev/null || true
        wait $NEXTJS_PID 2>/dev/null || true
    fi
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
    
    git clone "$REPO_URL" "$APP_DIR"
    cd "$APP_DIR"
else
    echo "[Orbat] Checking for updates..."
    cd "$APP_DIR"
    
    # Check if there are updates
    update_status "checking" "Checking for updates" 2 "Fetching latest commits from GitHub"
    git fetch origin main
    LOCAL=$(git rev-parse HEAD)
    REMOTE=$(git rev-parse origin/main)
    
    if [ "$LOCAL" != "$REMOTE" ]; then
        echo "[Orbat] Updates detected - pulling changes..."
        show_maintenance
        update_status "pulling" "Pulling latest changes" 10 "Downloading updated code from repository"
        git pull origin main
    else
        echo "[Orbat] No updates found"
        update_status "starting" "No updates needed" 95 "Starting application with current version"
    fi
fi

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

# Stop maintenance if it was running
update_status "finalizing" "Finalizing startup" 95 "Preparing to start Next.js server"
stop_maintenance

# Start update checker in background
start_update_checker

# Start the application
echo "[Orbat] Starting Next.js application..."
update_status "running" "Application running" 100 "Service is now available"

# Register with nginx (persistent connection)
register_with_nginx

# Start npm in background so we can track its PID
npm start &
NEXTJS_PID=$!

# Wait for Next.js process
wait $NEXTJS_PID
