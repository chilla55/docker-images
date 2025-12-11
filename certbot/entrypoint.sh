#!/bin/bash
# ============================================================================
# Certbot Entrypoint Script with Hetzner Storage Box (CIFS Mount)
# ============================================================================
# This script:
# 1. Mounts Hetzner Storage Box via CIFS/Samba to /etc/letsencrypt
# 2. Obtains/renews Let's Encrypt certificates using Cloudflare DNS
# 3. Certificates are directly written to Storage Box mount
# 4. Optionally signals nginx to reload via Docker API
# ============================================================================

set -e

# ──────────────────────────────────────────────────────────────────────────
# Configuration
# ──────────────────────────────────────────────────────────────────────────
CERT_EMAIL="${CERT_EMAIL:-admin@example.com}"
CERT_DOMAINS="${CERT_DOMAINS:-example.com,*.example.com}"
CLOUDFLARE_CREDENTIALS_FILE="${CLOUDFLARE_CREDENTIALS_FILE:-/run/secrets/cloudflare_api_token}"
RENEW_INTERVAL="${RENEW_INTERVAL:-12h}"
CERTBOT_DRY_RUN="${CERTBOT_DRY_RUN:-false}"

# Storage Box settings (SMB3 primary, SSHFS fallback with password)
STORAGE_BOX_ENABLED="${STORAGE_BOX_ENABLED:-true}"
STORAGE_BOX_HOST="${STORAGE_BOX_HOST:-u123456.your-storagebox.de}"
STORAGE_BOX_USER="${STORAGE_BOX_USER:-u123456}"
STORAGE_BOX_PASSWORD_FILE="${STORAGE_BOX_PASSWORD_FILE:-/run/secrets/storagebox_password}"
STORAGE_BOX_SSH_KEY_FILE="${STORAGE_BOX_SSH_KEY_FILE:-/run/secrets/storagebox_ssh_key}"
STORAGE_BOX_SSH_PORT="${STORAGE_BOX_SSH_PORT:-23}"
STORAGE_BOX_PATH="${STORAGE_BOX_PATH:-/backup}"
STORAGE_BOX_MOUNT_OPTIONS="${STORAGE_BOX_MOUNT_OPTIONS:-vers=3.0,seal,nodfs,noserverino,nounix,uid=0,gid=1001,file_mode=0640,dir_mode=0750}"
STORAGE_BOX_USE_SSHFS="${STORAGE_BOX_USE_SSHFS:-false}"

DEBUG="${DEBUG:-false}"

# ──────────────────────────────────────────────────────────────────────────
# Logging Functions
# ──────────────────────────────────────────────────────────────────────────
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

log_debug() {
    if [ "$DEBUG" = "true" ]; then
        echo "[$(date +'%Y-%m-%d %H:%M:%S')] [DEBUG] $*"
    fi
}

log_error() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] [ERROR] $*" >&2
}

# ──────────────────────────────────────────────────────────────────────────
# Setup Functions
# ──────────────────────────────────────────────────────────────────────────
setup_cloudflare() {
    log "Setting up Cloudflare credentials..."
    
    if [ ! -f "$CLOUDFLARE_CREDENTIALS_FILE" ]; then
        log_error "Cloudflare API token not found at $CLOUDFLARE_CREDENTIALS_FILE"
        exit 1
    fi
    
    # Create cloudflare.ini from token
    local cf_dir="/etc/letsencrypt/.secrets/certbot"
    mkdir -p "$cf_dir"
    
    local token=$(cat "$CLOUDFLARE_CREDENTIALS_FILE")
    cat > "$cf_dir/cloudflare.ini" <<EOF
dns_cloudflare_api_token = $token
EOF
    
    chmod 600 "$cf_dir/cloudflare.ini"
    log "Cloudflare credentials configured"
}

mount_storage_box() {
    if [ "$STORAGE_BOX_ENABLED" != "true" ]; then
        log "Storage Box mounting is disabled, using local storage"
        return 0
    fi
    
    local mount_point="/etc/letsencrypt"
    
    # Check if already mounted (bind-mount from host or direct mount)
    if mount | grep -q "on $mount_point type"; then
        log "Storage Box already mounted at $mount_point"
        return 0
    fi
    
    log "Storage Box mount not found at $mount_point, using local storage"
    return 0
}

mount_storage_box_smb3() {
    log_debug "Attempting SMB3 mount..."
    
    if [ ! -f "$STORAGE_BOX_PASSWORD_FILE" ]; then
        log_debug "Storage Box password file not found, skipping SMB3"
        return 1
    fi
    
    local password=$(cat "$STORAGE_BOX_PASSWORD_FILE")
    local mount_point="/etc/letsencrypt"
    local remote_path="//${STORAGE_BOX_HOST}${STORAGE_BOX_PATH}"
    
    # Check if already mounted
    if mount | grep -q "on $mount_point type smb3"; then
        log "Storage Box already mounted at $mount_point (SMB3)"
        return 0
    fi
    
    mkdir -p "$mount_point"
    
    # Try SMB3 - will likely fail in Docker but worth attempting
    if mount -t smb3 "$remote_path" "$mount_point" \
        -o "username=${STORAGE_BOX_USER},password=${password},${STORAGE_BOX_MOUNT_OPTIONS}" 2>/dev/null; then
        log "Successfully mounted Storage Box at $mount_point via SMB3"
        return 0
    fi
    
    log_debug "SMB3 mount failed (expected in Docker containers)"
    return 1
}

mount_storage_box_sshfs() {
    log "Mounting Hetzner Storage Box via SSHFS (port $STORAGE_BOX_SSH_PORT)..."
    
    if [ ! -f "$STORAGE_BOX_PASSWORD_FILE" ]; then
        log_debug "Storage Box password file not found, skipping SSHFS"
        return 1
    fi
    
    # Try to load FUSE module (required for SSHFS in containers)
    if ! grep -q "^fuse" /proc/filesystems 2>/dev/null; then
        log_debug "Loading FUSE kernel module..."
        modprobe fuse 2>/dev/null || true
    fi
    
    # Check if FUSE is available
    if ! grep -q "^fuse" /proc/filesystems 2>/dev/null; then
        log_debug "FUSE kernel module not available, skipping SSHFS"
        return 1
    fi
    
    local password=$(cat "$STORAGE_BOX_PASSWORD_FILE")
    local mount_point="/etc/letsencrypt"
    local remote_path="${STORAGE_BOX_USER}@${STORAGE_BOX_HOST}:${STORAGE_BOX_PATH}"
    
    # Check if already mounted
    if mount | grep -q "on $mount_point type fuse.sshfs"; then
        log "Storage Box already mounted at $mount_point (SSHFS)"
        return 0
    fi
    
    mkdir -p "$mount_point"
    
    # Mount via SSHFS with password (using sshpass)
    # Simplified options: just allow_other and auto_unmount
    if command -v sshpass &> /dev/null; then
        if sshpass -p "$password" sshfs -p "$STORAGE_BOX_SSH_PORT" \
            -o "StrictHostKeyChecking=accept-new,allow_other,auto_unmount" \
            "$remote_path" "$mount_point" 2>/dev/null; then
            log "Successfully mounted Storage Box at $mount_point via SSHFS"
            return 0
        fi
    fi
    
    log_debug "Failed to mount Storage Box via SSHFS"
    return 1
}

mount_storage_box_cifs() {
    log_debug "Attempting CIFS mount..."
    
    if [ ! -f "$STORAGE_BOX_PASSWORD_FILE" ]; then
        log_debug "Storage Box password file not found, skipping CIFS"
        return 1
    fi
    
    local password=$(cat "$STORAGE_BOX_PASSWORD_FILE")
    local mount_point="/etc/letsencrypt"
    local remote_path="//${STORAGE_BOX_HOST}${STORAGE_BOX_PATH}"
    
    # Check if already mounted
    if mount | grep -q "on $mount_point type cifs"; then
        log "Storage Box already mounted at $mount_point (CIFS)"
        return 0
    fi
    
    mkdir -p "$mount_point"
    
    # Try CIFS - will likely fail in Docker but worth attempting
    if mount -t cifs "$remote_path" "$mount_point" \
        -o "user=${STORAGE_BOX_USER},password=${password},${STORAGE_BOX_MOUNT_OPTIONS}" 2>/dev/null; then
        log "Successfully mounted Storage Box at $mount_point via CIFS"
        return 0
    fi
    
    log_debug "CIFS mount failed (expected in Docker containers)"
    return 1
}

unmount_storage_box() {
    if [ "$STORAGE_BOX_ENABLED" = "true" ]; then
        log "Unmounting Storage Box..."
        umount /etc/letsencrypt 2>/dev/null || true
    fi
}

# ──────────────────────────────────────────────────────────────────────────
# Certificate Functions
# ──────────────────────────────────────────────────────────────────────────
obtain_certificate() {
    local domains_array=(${CERT_DOMAINS//,/ })
    local certbot_domains=""
    local staging_args=()
    
    for domain in "${domains_array[@]}"; do
        certbot_domains="$certbot_domains -d $domain"
    done

    if [ "$CERTBOT_DRY_RUN" = "true" ]; then
        staging_args+=(--test-cert)
    fi
    
    log "Checking if certificate exists for: ${CERT_DOMAINS}"
    
    # Check if certificate already exists
    local first_domain="${domains_array[0]}"
    # Remove wildcard asterisk for directory name
    first_domain="${first_domain#\*.}"
    
    if [ -d "/etc/letsencrypt/live/$first_domain" ]; then
        log "Certificate already exists for $first_domain"
        return 0
    fi
    
    log "Obtaining new certificate for: ${CERT_DOMAINS}"
    certbot certonly \
        --dns-cloudflare \
        --dns-cloudflare-credentials /etc/letsencrypt/.secrets/certbot/cloudflare.ini \
        --email "$CERT_EMAIL" \
        --agree-tos \
        --non-interactive \
        --quiet \
        "${staging_args[@]}" \
        $certbot_domains
    
    if [ $? -eq 0 ]; then
        log "Successfully obtained certificate"
        fix_permissions
        return 0
    else
        log_error "Failed to obtain certificate"
        return 1
    fi
}

renew_certificates() {
    log "Checking for certificate renewals..."
    local renew_args=(
        --dns-cloudflare
        --dns-cloudflare-credentials /etc/letsencrypt/.secrets/certbot/cloudflare.ini
        --quiet
        --non-interactive
        --deploy-hook "/scripts/entrypoint.sh post-renew"
    )

    if [ "$CERTBOT_DRY_RUN" = "true" ]; then
        renew_args+=(--dry-run --test-cert)
    fi
    
    certbot renew "${renew_args[@]}"
    
    local exit_code=$?
    
    if [ $exit_code -eq 0 ]; then
        log "Certificate renewal check completed"
        fix_permissions
    else
        log_error "Certificate renewal failed with exit code: $exit_code"
    fi
    
    return $exit_code
}

fix_permissions() {
    log_debug "Fixing certificate permissions for nginx (gid 1001)"
    if [ -d "/etc/letsencrypt/live" ]; then
        chgrp -R 1001 /etc/letsencrypt/live 2>/dev/null || true
        chmod -R g+r /etc/letsencrypt/live 2>/dev/null || true
        chgrp -R 1001 /etc/letsencrypt/archive 2>/dev/null || true
        chmod -R g+r /etc/letsencrypt/archive 2>/dev/null || true
    fi
}

# ──────────────────────────────────────────────────────────────────────────
# Main Functions
# ──────────────────────────────────────────────────────────────────────────
post_renew_hook() {
    log "Running post-renew hook..."
    fix_permissions
    log "Certificates updated - nginx will auto-reload via certificate watcher"
}

convert_interval_to_seconds() {
    local interval=$1
    local number=$(echo "$interval" | grep -oE '[0-9]+')
    local unit=$(echo "$interval" | grep -oE '[a-zA-Z]+')
    
    case $unit in
        s) echo "$number" ;;
        m) echo $((number * 60)) ;;
        h) echo $((number * 3600)) ;;
        d) echo $((number * 86400)) ;;
        *) echo "43200" ;; # Default to 12 hours
    esac
}

cleanup() {
    log "Received termination signal, cleaning up..."
    unmount_storage_box
    exit 0
}

main() {
    log "=========================================="
    log "Certbot with Hetzner Storage Box (CIFS)"
    log "=========================================="
    log "Email: $CERT_EMAIL"
    log "Domains: $CERT_DOMAINS"
    log "Renew Interval: $RENEW_INTERVAL"
    log "Storage Box Enabled: $STORAGE_BOX_ENABLED"
    if [ "$STORAGE_BOX_ENABLED" = "true" ]; then
        log "Storage Box: //${STORAGE_BOX_HOST}${STORAGE_BOX_PATH}"
    fi
    log "Dry Run (staging only): $CERTBOT_DRY_RUN"
    log "=========================================="
    
    # Setup
    setup_cloudflare
    mount_storage_box || true

    # Optional dry-run to just validate mounting/credentials without hitting ACME
    if [ "${SKIP_CERTS:-false}" = "true" ]; then
        log "SKIP_CERTS=true: skipping certificate obtain/renew loop."
        # Keep container alive so you can inspect the mount
        tail -f /dev/null
    fi
    
    # Obtain certificate if it doesn't exist
    obtain_certificate
    
    # Convert interval to seconds
    local sleep_seconds=$(convert_interval_to_seconds "$RENEW_INTERVAL")
    log "Will check for renewals every ${sleep_seconds} seconds (${RENEW_INTERVAL})"
    
    # Renewal loop
    log "Starting renewal loop..."
    trap cleanup TERM INT
    
    while :; do
        sleep "$sleep_seconds" &
        wait $!
        
        renew_certificates
    done
}

# ──────────────────────────────────────────────────────────────────────────
# Entry Point
# ──────────────────────────────────────────────────────────────────────────
case "${1:-}" in
    post-renew)
        post_renew_hook
        ;;
    *)
        main
        ;;
esac
