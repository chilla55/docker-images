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
CLOUDFLARE_CREDENTIALS="${CLOUDFLARE_CREDENTIALS:-/run/secrets/cloudflare_credentials}"
RENEW_INTERVAL="${RENEW_INTERVAL:-12h}"

# Storage Box CIFS/Samba settings
STORAGE_BOX_ENABLED="${STORAGE_BOX_ENABLED:-true}"
STORAGE_BOX_HOST="${STORAGE_BOX_HOST:-u123456.your-storagebox.de}"
STORAGE_BOX_USER="${STORAGE_BOX_USER:-u123456}"
STORAGE_BOX_PASSWORD_FILE="${STORAGE_BOX_PASSWORD_FILE:-/run/secrets/storagebox_password}"
STORAGE_BOX_PATH="${STORAGE_BOX_PATH:-/certs}"
STORAGE_BOX_MOUNT_OPTIONS="${STORAGE_BOX_MOUNT_OPTIONS:-vers=3.0,uid=0,gid=1001,file_mode=0640,dir_mode=0750}"

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
    
    if [ -f "$CLOUDFLARE_CREDENTIALS" ]; then
        log_debug "Cloudflare credentials found"
        chmod 600 "$CLOUDFLARE_CREDENTIALS"
        log "Cloudflare credentials configured"
    else
        log_error "Cloudflare credentials not found at $CLOUDFLARE_CREDENTIALS"
        exit 1
    fi
}

mount_storage_box() {
    if [ "$STORAGE_BOX_ENABLED" != "true" ]; then
        log "Storage Box mounting is disabled, using local storage"
        return 0
    fi
    
    log "Mounting Hetzner Storage Box via CIFS..."
    
    if [ ! -f "$STORAGE_BOX_PASSWORD_FILE" ]; then
        log_error "Storage Box password file not found at $STORAGE_BOX_PASSWORD_FILE"
        log "Falling back to local storage"
        STORAGE_BOX_ENABLED="false"
        return 1
    fi
    
    local password=$(cat "$STORAGE_BOX_PASSWORD_FILE")
    local mount_point="/etc/letsencrypt"
    local remote_path="//${STORAGE_BOX_HOST}${STORAGE_BOX_PATH}"
    
    log_debug "Mounting $remote_path to $mount_point"
    
    # Check if already mounted
    if mountpoint -q "$mount_point"; then
        log "Storage Box already mounted at $mount_point"
        return 0
    fi
    
    # Create mount point if it doesn't exist
    mkdir -p "$mount_point"
    
    # Mount the Storage Box
    if mount -t cifs "$remote_path" "$mount_point" \
        -o "username=${STORAGE_BOX_USER},password=${password},${STORAGE_BOX_MOUNT_OPTIONS}"; then
        log "Successfully mounted Storage Box at $mount_point"
        log_debug "Mount options: $STORAGE_BOX_MOUNT_OPTIONS"
        return 0
    else
        log_error "Failed to mount Storage Box"
        log_error "Remote: $remote_path"
        log_error "Check credentials and network connectivity"
        log "Falling back to local storage"
        STORAGE_BOX_ENABLED="false"
        return 1
    fi
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
    
    for domain in "${domains_array[@]}"; do
        certbot_domains="$certbot_domains -d $domain"
    done
    
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
        --dns-cloudflare-credentials "$CLOUDFLARE_CREDENTIALS" \
        --email "$CERT_EMAIL" \
        --agree-tos \
        --non-interactive \
        --quiet \
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
    
    certbot renew \
        --dns-cloudflare \
        --dns-cloudflare-credentials "$CLOUDFLARE_CREDENTIALS" \
        --quiet \
        --non-interactive \
        --deploy-hook "/scripts/entrypoint.sh post-renew"
    
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
    log "=========================================="
    
    # Setup
    setup_cloudflare
    mount_storage_box
    
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
