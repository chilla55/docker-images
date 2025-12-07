#!/bin/bash
# ============================================================================
# Pterodactyl Panel Entrypoint Script
# ============================================================================
# Handles environment setup, secret loading, validation, and initialization
# ============================================================================

set -e  # Exit on error
set -u  # Exit on undefined variable
set -o pipefail  # Exit on pipe failure

# ──────────────────────────────────────────────────────────────────────────
# Configuration
# ──────────────────────────────────────────────────────────────────────────
APP_DIR="/var/www/pterodactyl"
ENV_FILE="${APP_DIR}/.env"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# ──────────────────────────────────────────────────────────────────────────
# Helper Functions
# ──────────────────────────────────────────────────────────────────────────
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

die() {
    log_error "$1"
    exit 1
}

# ──────────────────────────────────────────────────────────────────────────
# Load Secrets from Files
# ──────────────────────────────────────────────────────────────────────────
load_secret() {
    local var_name="$1"
    local file_var="${var_name}_FILE"
    local file_path="${!file_var:-}"
    
    if [[ -n "$file_path" ]] && [[ -f "$file_path" ]]; then
        local value
        value=$(cat "$file_path" | tr -d '[:space:]')
        
        if [[ -z "$value" ]]; then
            log_warn "Secret file $file_path is empty"
            return 1
        fi
        
        export "${var_name}=${value}"
        log_info "Loaded ${var_name} from ${file_path}"
        return 0
    fi
    
    return 1
}

# ──────────────────────────────────────────────────────────────────────────
# Main Entrypoint Logic
# ──────────────────────────────────────────────────────────────────────────
log_info "Starting Pterodactyl Panel ${PANEL_VERSION:-unknown}"
cd "$APP_DIR" || die "Failed to change to ${APP_DIR}"

# ──────────────────────────────────────────────────────────────────────────
# Step 1: Load Secrets
# ──────────────────────────────────────────────────────────────────────────
log_info "Loading secrets from files..."

load_secret "APP_KEY" || log_warn "APP_KEY not loaded from file (will check environment)"
load_secret "DB_PASSWORD" || log_warn "DB_PASSWORD not loaded from file"
load_secret "REDIS_PASSWORD" || log_warn "REDIS_PASSWORD not loaded from file"
load_secret "MAIL_PASSWORD" || log_warn "MAIL_PASSWORD not loaded from file"

# ──────────────────────────────────────────────────────────────────────────
# Step 2: Validate Required Environment Variables
# ──────────────────────────────────────────────────────────────────────────
log_info "Validating environment configuration..."

REQUIRED_VARS=(
    "APP_ENV"
    "APP_URL"
    "DB_HOST"
    "DB_DATABASE"
    "DB_USERNAME"
    "DB_PASSWORD"
)

for var in "${REQUIRED_VARS[@]}"; do
    if [[ -z "${!var:-}" ]]; then
        die "Required environment variable ${var} is not set"
    fi
done

# Validate APP_KEY format (must be base64: prefixed)
if [[ -n "${APP_KEY:-}" ]]; then
    if [[ ! "$APP_KEY" =~ ^base64: ]]; then
        die "APP_KEY must start with 'base64:' prefix"
    fi
else
    log_warn "APP_KEY is not set - this must be generated before first run"
fi

# ──────────────────────────────────────────────────────────────────────────
# Step 3: Generate .env File
# ──────────────────────────────────────────────────────────────────────────
log_info "Generating .env configuration..."

cat > "$ENV_FILE" <<EOF
# ============================================================================
# Pterodactyl Panel Environment Configuration
# Generated at: $(date -u +"%Y-%m-%d %H:%M:%S UTC")
# ============================================================================

# ──────────────────────────────────────────────────────────────────────────
# Application Settings
# ──────────────────────────────────────────────────────────────────────────
APP_ENV=${APP_ENV:-production}
APP_DEBUG=${APP_DEBUG:-false}
APP_KEY=${APP_KEY:-}
APP_URL=${APP_URL}
APP_TIMEZONE=${APP_TIMEZONE:-UTC}
APP_LOCALE=${APP_LOCALE:-en}

# ──────────────────────────────────────────────────────────────────────────
# Cache & Session Settings
# ──────────────────────────────────────────────────────────────────────────
CACHE_DRIVER=${CACHE_DRIVER:-redis}
SESSION_DRIVER=${SESSION_DRIVER:-redis}
QUEUE_CONNECTION=${QUEUE_CONNECTION:-redis}

# ──────────────────────────────────────────────────────────────────────────
# Database Settings
# ──────────────────────────────────────────────────────────────────────────
DB_CONNECTION=${DB_CONNECTION:-mysql}
DB_HOST=${DB_HOST}
DB_PORT=${DB_PORT:-3306}
DB_DATABASE=${DB_DATABASE}
DB_USERNAME=${DB_USERNAME}
DB_PASSWORD=${DB_PASSWORD}

# ──────────────────────────────────────────────────────────────────────────
# Redis Settings
# ──────────────────────────────────────────────────────────────────────────
REDIS_HOST=${REDIS_HOST:-redis}
REDIS_PORT=${REDIS_PORT:-6379}
REDIS_PASSWORD=${REDIS_PASSWORD:-}

# ──────────────────────────────────────────────────────────────────────────
# Mail Settings
# ──────────────────────────────────────────────────────────────────────────
MAIL_MAILER=${MAIL_MAILER:-smtp}
MAIL_HOST=${MAIL_HOST:-smtp.example.com}
MAIL_PORT=${MAIL_PORT:-587}
MAIL_USERNAME=${MAIL_USERNAME:-}
MAIL_PASSWORD=${MAIL_PASSWORD:-}
MAIL_ENCRYPTION=${MAIL_ENCRYPTION:-tls}
MAIL_FROM_ADDRESS=${MAIL_FROM_ADDRESS:-noreply@example.com}
MAIL_FROM_NAME="${MAIL_FROM_NAME:-Pterodactyl Panel}"

# ──────────────────────────────────────────────────────────────────────────
# Hashids Settings (for obfuscating server IDs)
# ──────────────────────────────────────────────────────────────────────────
HASHIDS_SALT=${HASHIDS_SALT:-}
HASHIDS_LENGTH=${HASHIDS_LENGTH:-8}

# ──────────────────────────────────────────────────────────────────────────
# Trusted Proxies (nginx reverse proxy)
# ──────────────────────────────────────────────────────────────────────────
TRUSTED_PROXIES=${TRUSTED_PROXIES:-*}
EOF

chown www-data:www-data "$ENV_FILE"
chmod 600 "$ENV_FILE"
log_info ".env file generated successfully"

# ──────────────────────────────────────────────────────────────────────────
# Step 4: Set Proper Permissions
# ──────────────────────────────────────────────────────────────────────────
log_info "Setting file permissions..."

chown -R www-data:www-data "$APP_DIR"
chmod -R 755 "${APP_DIR}/storage" "${APP_DIR}/bootstrap/cache"

# ──────────────────────────────────────────────────────────────────────────
# Step 5: Wait for Database
# ──────────────────────────────────────────────────────────────────────────
log_info "Waiting for database connection..."

max_attempts=30
attempt=0

while [ $attempt -lt $max_attempts ]; do
    if mariadb -h"${DB_HOST}" -P"${DB_PORT}" -u"${DB_USERNAME}" -p"${DB_PASSWORD}" -e "SELECT 1" >/dev/null 2>&1; then
        log_info "Database connection successful"
        break
    fi
    
    attempt=$((attempt + 1))
    log_warn "Database not ready, attempt ${attempt}/${max_attempts}..."
    sleep 2
done

if [ $attempt -eq $max_attempts ]; then
    die "Failed to connect to database after ${max_attempts} attempts"
fi

# ──────────────────────────────────────────────────────────────────────────
# Step 6: Clear and Optimize Caches
# ──────────────────────────────────────────────────────────────────────────
log_info "Optimizing application caches..."

php artisan config:clear || log_warn "Failed to clear config cache"
php artisan cache:clear || log_warn "Failed to clear application cache"
php artisan view:clear || log_warn "Failed to clear view cache"

# Generate optimized config cache for production
if [[ "${APP_ENV}" == "production" ]]; then
    php artisan config:cache || log_warn "Failed to cache config"
    php artisan route:cache || log_warn "Failed to cache routes"
    php artisan view:cache || log_warn "Failed to cache views"
fi

# ──────────────────────────────────────────────────────────────────────────
# Step 7: Run Database Migrations (if enabled)
# ──────────────────────────────────────────────────────────────────────────
if [[ "${RUN_MIGRATIONS_ON_START:-false}" == "true" ]]; then
    log_info "Running database migrations..."
    
    if php artisan migrate --force; then
        log_info "Migrations completed successfully"
    else
        log_error "Migration failed - container will continue but may not work correctly"
    fi
else
    log_info "Skipping migrations (RUN_MIGRATIONS_ON_START not set to true)"
fi

# ──────────────────────────────────────────────────────────────────────────
# Step 8: Database Seeding (if enabled and first run)
# ──────────────────────────────────────────────────────────────────────────
if [[ "${RUN_SEED_ON_START:-false}" == "true" ]]; then
    log_info "Running database seeders..."
    
    if php artisan db:seed --force; then
        log_info "Database seeding completed"
    else
        log_warn "Database seeding failed - this is normal if database is already seeded"
    fi
fi

# ──────────────────────────────────────────────────────────────────────────
# Step 9: Display Startup Summary
# ──────────────────────────────────────────────────────────────────────────
log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "Pterodactyl Panel Container Ready"
log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
log_info "Version:     ${PANEL_VERSION:-unknown}"
log_info "Environment: ${APP_ENV}"
log_info "URL:         ${APP_URL}"
log_info "Database:    ${DB_USERNAME}@${DB_HOST}:${DB_PORT}/${DB_DATABASE}"
log_info "Redis:       ${REDIS_HOST}:${REDIS_PORT}"
log_info "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# ──────────────────────────────────────────────────────────────────────────
# Step 10: Execute Main Process
# ──────────────────────────────────────────────────────────────────────────
log_info "Starting supervisord..."
exec "$@"


