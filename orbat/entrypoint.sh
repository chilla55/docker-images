#!/bin/bash
set -e

REPO_URL="https://github.com/6th-Maroon-Division/Homepage.git"
APP_DIR="/app"
MAINTENANCE_PAGE="/maintenance.html"
MAINTENANCE_ACTIVE="/tmp/maintenance_active"
STATUS_FILE="/tmp/status.json"
UPDATE_CHECK_INTERVAL="${UPDATE_CHECK_INTERVAL:-300}"  # Check every 5 minutes by default

# Environment variable for extended info display (default: false)
SHOW_EXTENDED_INFO="${SHOW_EXTENDED_INFO:-false}"

echo "[Orbat] Starting entrypoint..."
echo "[Orbat] Auto-update check interval: ${UPDATE_CHECK_INTERVAL}s"

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

# Function to show maintenance page
show_maintenance() {
    echo "[Orbat] Showing maintenance page..."
    touch "$MAINTENANCE_ACTIVE"
    update_status "initializing" "Starting maintenance mode" 0 "Preparing to update application"
    
    # Start a simple HTTP server showing maintenance and status API
    ( cd / && exec node -e "
        const http = require('http');
        const fs = require('fs');
        const server = http.createServer((req, res) => {
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
        server.listen($PORT);
        console.log('Maintenance server listening on port $PORT');
    " ) &
    MAINTENANCE_PID=$!
    sleep 2
}

# Function to stop maintenance page
stop_maintenance() {
    echo "[Orbat] Stopping maintenance page..."
    rm -f "$MAINTENANCE_ACTIVE"
    if [ ! -z "$MAINTENANCE_PID" ]; then
        kill $MAINTENANCE_PID 2>/dev/null || true
    fi
}

# Function to perform update
perform_update() {
    echo "[Orbat] Performing update..."
    cd "$APP_DIR"
    
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
    
    cd "$APP_DIR"
    
    # Clone into current directory (which is the mounted volume)
    git clone "$REPO_URL" .
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

# Start npm in background so we can track its PID
npm start &
NEXTJS_PID=$!

# Wait for Next.js process
wait $NEXTJS_PID
