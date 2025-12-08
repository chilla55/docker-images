#!/bin/bash
set -eu

: "${CF_REALIP_AUTO:=1}"
: "${CF_REALIP_INTERVAL:=21600}"  # 6h
: "${CF_REALIP_STATUS:=/var/cache/nginx/cf-realip.status.json}"

# Storage Box CIFS mount settings for certificates
: "${STORAGE_BOX_CERTS_ENABLED:=false}"
: "${STORAGE_BOX_HOST:=}"
: "${STORAGE_BOX_USER:=}"
: "${STORAGE_BOX_PASSWORD_FILE:=/run/secrets/storagebox_password}"
: "${STORAGE_BOX_CERTS_PATH:=/certs}"
: "${STORAGE_BOX_CERTS_MOUNT_POINT:=/etc/nginx/certs}"
: "${STORAGE_BOX_CERTS_MOUNT_OPTIONS:=vers=3.0,uid=0,gid=101,file_mode=0640,dir_mode=0750,ro}"

# Storage Box CIFS mount settings for site configurations
: "${STORAGE_BOX_SITES_ENABLED:=false}"
: "${STORAGE_BOX_SITES_PATH:=/sites}"
: "${STORAGE_BOX_SITES_MOUNT_POINT:=/etc/nginx/sites-enabled}"
: "${STORAGE_BOX_SITES_MOUNT_OPTIONS:=vers=3.0,uid=0,gid=101,file_mode=0640,dir_mode=0750,ro}"

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

# ──────────────────────────────────────────────────────────────────────────
# Mount Storage Box path (generic function)
# ──────────────────────────────────────────────────────────────────────────
mount_storage_box_path() {
    local enabled=$1
    local remote_path=$2
    local mount_point=$3
    local mount_options=$4
    local name=$5
    
    if [ "$enabled" != "true" ]; then
        log "[storage-box-$name] Mounting disabled, skipping"
        return 0
    fi
    
    if [ -z "$STORAGE_BOX_HOST" ] || [ -z "$STORAGE_BOX_USER" ]; then
        log "[storage-box-$name] ERROR: STORAGE_BOX_HOST and STORAGE_BOX_USER must be set"
        log "[storage-box-$name] Skipping mount, $name won't be available"
        return 1
    fi
    
    if [ ! -f "$STORAGE_BOX_PASSWORD_FILE" ]; then
        log "[storage-box-$name] ERROR: Password file not found at $STORAGE_BOX_PASSWORD_FILE"
        log "[storage-box-$name] Skipping mount, $name won't be available"
        return 1
    fi
    
    local password=$(cat "$STORAGE_BOX_PASSWORD_FILE")
    local full_remote_path="//${STORAGE_BOX_HOST}${remote_path}"
    
    log "[storage-box-$name] Mounting $full_remote_path to $mount_point"
    
    # Check if already mounted
    if mountpoint -q "$mount_point" 2>/dev/null; then
        log "[storage-box-$name] Already mounted at $mount_point"
        return 0
    fi
    
    # Create mount point if it doesn't exist
    mkdir -p "$mount_point"
    
    # Mount the Storage Box
    if mount -t cifs "$full_remote_path" "$mount_point" \
        -o "username=${STORAGE_BOX_USER},password=${password},${mount_options}"; then
        log "[storage-box-$name] Successfully mounted at $mount_point"
        return 0
    else
        log "[storage-box-$name] ERROR: Failed to mount $full_remote_path"
        log "[storage-box-$name] Skipping mount, $name won't be available"
        return 1
    fi
}

# Cleanup function for graceful shutdown
cleanup() {
    log "[entrypoint] Shutting down..."
    if [ "$STORAGE_BOX_CERTS_ENABLED" = "true" ]; then
        log "[storage-box-certs] Unmounting..."
        umount "$STORAGE_BOX_CERTS_MOUNT_POINT" 2>/dev/null || true
    fi
    if [ "$STORAGE_BOX_SITES_ENABLED" = "true" ]; then
        log "[storage-box-sites] Unmounting..."
        umount "$STORAGE_BOX_SITES_MOUNT_POINT" 2>/dev/null || true
    fi
    exit 0
}

trap cleanup TERM INT

# ──────────────────────────────────────────────────────────────────────────
# Mount Storage Box paths
# ──────────────────────────────────────────────────────────────────────────
mount_storage_box_path \
    "$STORAGE_BOX_CERTS_ENABLED" \
    "$STORAGE_BOX_CERTS_PATH" \
    "$STORAGE_BOX_CERTS_MOUNT_POINT" \
    "$STORAGE_BOX_CERTS_MOUNT_OPTIONS" \
    "certs" || true

mount_storage_box_path \
    "$STORAGE_BOX_SITES_ENABLED" \
    "$STORAGE_BOX_SITES_PATH" \
    "$STORAGE_BOX_SITES_MOUNT_POINT" \
    "$STORAGE_BOX_SITES_MOUNT_OPTIONS" \
    "sites" || true

# Ensure cache dir + status file exist before first run
mkdir -p "$(dirname "$CF_REALIP_STATUS")"
[ -f "$CF_REALIP_STATUS" ] || : > "$CF_REALIP_STATUS"

run_once() { /usr/local/bin/update-cf-ips.sh || echo "[cloudflare-realip] update failed"; }

# Cloudflare updater loop
if [ "$CF_REALIP_AUTO" = "1" ] || [ "$CF_REALIP_AUTO" = "true" ]; then
  run_once || true
  ( while :; do sleep "$CF_REALIP_INTERVAL"; run_once || true; done ) &
else
  echo "[cloudflare-realip] Auto-update disabled."
fi

# Cert watcher loop (only if path is set)
if [ -n "${CERT_WATCH_PATH:-}" ]; then
  echo "[entrypoint] Starting cert watcher for: $CERT_WATCH_PATH"
  /usr/local/bin/watch-cert-reload.sh &
else
  echo "[entrypoint] CERT_WATCH_PATH not set, cert watcher disabled."
fi

# Fix permissions before dropping to nginx user
chown -R nginx:nginx /var/cache/nginx /etc/nginx/logs 2>/dev/null || true

# Drop privileges and run nginx (PID 1)
log "[entrypoint] Starting nginx as user nginx"
exec su-exec nginx /usr/sbin/nginx -g 'daemon off;'
