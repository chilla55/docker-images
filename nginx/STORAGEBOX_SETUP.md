# Nginx Storage Box Configuration Summary

## Changes Implemented

### 1. **Docker Compose Configuration** (`docker-compose.swarm.yml`)
   - Updated to use **host bind mounts** instead of CIFS mounts inside the container
   - Added volumes section:
     ```yaml
     volumes:
       - /mnt/storagebox/certs:/etc/nginx/certs:ro
       - /mnt/storagebox/sites:/etc/nginx/sites-available:ro
     ```
   - Removed CIFS mount environment variables
   - Added sites watcher environment variables:
     ```
     SITES_WATCH_PATH: "/etc/nginx/sites-enabled"
     SITES_WATCH_INTERVAL: "30"
     SITES_WATCH_DEBUG: "0"
     ```

### 2. **Dockerfile Updates**
   - Added `COPY watch-sites-reload.sh /usr/local/bin/watch-sites-reload.sh`
   - Made the script executable

### 3. **New Script: `watch-sites-reload.sh`**
   - Monitors `/etc/nginx/sites-enabled` directory for changes
   - Checks every 30 seconds (configurable via `SITES_WATCH_INTERVAL`)
   - When site files change:
     - Validates nginx configuration with `nginx -t`
     - Reloads nginx if validation passes
     - Logs error and skips reload if validation fails
   - Configurable debug output via `SITES_WATCH_DEBUG=1`

### 4. **Entrypoint Script Updates** (`entrypoint.sh`)
   - Removed all CIFS mount logic (no longer needed with host bind mounts)
   - Added sites watcher startup:
     ```bash
     if [ -n "${SITES_WATCH_PATH:-}" ]; then
       /usr/local/bin/watch-sites-reload.sh &
     fi
     ```
   - Simplified cleanup function

## Storage Box Structure

The host machine should have the following structure at `/mnt/storagebox`:

```
/mnt/storagebox/
├── certs/              # SSL certificates mounted to /etc/nginx/certs
│   └── live/
│       └── example.com/
│           ├── fullchain.pem
│           ├── privkey.pem
│           └── ...
└── sites/              # Site configurations mounted to /etc/nginx/sites-available\n                        # Symlinks created in /etc/nginx/sites-enabled by watch-sites-reload.sh
    ├── example.com.conf
    ├── api.example.com.conf
    └── ...
```

## Features

### ✅ Auto-Reload on Site Changes
- Sites watcher detects file additions, modifications, and deletions
- Validates configuration before reloading
- Prevents nginx crashes from bad configs

### ✅ Cert Watcher Integration
- Existing cert watcher still works (if `CERT_WATCH_PATH` is set)
- Both systems work independently and concurrently

### ✅ Cloudflare Real IP
- Continues to work as before
- Updates automatically every 6 hours

### ✅ Host Bind Mounts
- Simpler setup than CIFS mounts inside container
- No password secrets needed for storagebox
- Host maintains storagebox CIFS mount

## Deployment Notes

1. **Ensure host mount exists**: The host must have `/mnt/storagebox/certs` and `/mnt/storagebox/sites` directories with CIFS mount already configured at the host level

2. **No more SYS_ADMIN/DAC_READ_SEARCH caps needed**: These can be removed since CIFS mounting is done on the host
   - However, they're left in place for now (harmless and future compatibility)

3. **Debug Mode**: Set `SITES_WATCH_DEBUG=1` in environment to enable detailed logging

4. **Interval Configuration**:
   - `SITES_WATCH_INTERVAL`: How often to check for changes (default: 30 seconds)
   - `CERT_WATCH_INTERVAL`: How often to check certs (default: 300 seconds)

## Testing

To test the auto-reload feature:

```bash
# Add a new site config
echo 'server { listen 80; server_name test.example.com; }' > /mnt/storagebox/sites/test.conf

# Watch the logs to see it reload automatically
docker logs <container-id>

# Remove the site
rm /mnt/storagebox/sites/test.conf

# Watch it reload again
```
