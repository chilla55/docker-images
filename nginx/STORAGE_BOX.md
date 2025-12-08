# Nginx with Hetzner Storage Box CIFS Mount

## Overview

Nginx has been updated to mount the Hetzner Storage Box directly via CIFS to read SSL certificates and site configurations. This allows nginx to share the same storage as certbot without needing Docker volumes.

## Features

- **Dual Mount Support**: Mount both certificates and site configurations from Storage Box
- **Independent Control**: Enable/disable each mount separately
- **Read-Only Access**: Nginx mounts both paths as read-only for security
- **Graceful Degradation**: Continues running even if mounts fail
- **Multi-Node Compatible**: Works across Docker Swarm nodes

## Changes Made

### 1. Dockerfile
- ✅ Added `cifs-utils` for CIFS/Samba mounting
- ✅ Added `bash` for enhanced entrypoint script
- ✅ Added `su-exec` for privilege dropping
- ✅ Created `/etc/nginx/certs` and `/etc/nginx/sites-enabled` directories
- ✅ Changed to run as `root` initially (required for mounting), then drops to `nginx` user

### 2. entrypoint.sh
- ✅ Added generic `mount_storage_box_path()` function
- ✅ Mounts both certificates and sites from Storage Box
- ✅ Graceful cleanup on shutdown (unmounts all paths)
- ✅ Drops privileges to `nginx` user after mounting
- ✅ Proper error handling if mounts fail

### 3. docker-compose.swarm.yml
- ✅ Added required capabilities: `SYS_ADMIN`, `DAC_READ_SEARCH`
- ✅ Added `/dev/fuse` device access
- ✅ Added Storage Box environment variables for both mounts
- ✅ Added `storagebox_password` secret
- ✅ Removed `certbot_certs` and `nginx_sites` volumes (no longer needed)
- ✅ Updated `CERT_WATCH_PATH` to point to mounted certs

## Configuration

### Environment Variables

Add these to your `docker-compose.swarm.yml`:

```yaml
environment:
  # Storage Box Connection (shared by both mounts)
  STORAGE_BOX_HOST: u123456.your-storagebox.de  # Your Storage Box hostname
  STORAGE_BOX_USER: u123456  # Your Storage Box username
  STORAGE_BOX_PASSWORD_FILE: /run/secrets/storagebox_password
  
  # Certificates Mount
  STORAGE_BOX_CERTS_ENABLED: "true"
  STORAGE_BOX_CERTS_PATH: /certs  # Must match certbot's path
  STORAGE_BOX_CERTS_MOUNT_POINT: /etc/nginx/certs
  STORAGE_BOX_CERTS_MOUNT_OPTIONS: "vers=3.0,uid=0,gid=101,file_mode=0640,dir_mode=0750,ro"
  
  # Site Configurations Mount
  STORAGE_BOX_SITES_ENABLED: "true"
  STORAGE_BOX_SITES_PATH: /sites  # Path on Storage Box for nginx site configs
  STORAGE_BOX_SITES_MOUNT_POINT: /etc/nginx/sites-enabled
  STORAGE_BOX_SITES_MOUNT_OPTIONS: "vers=3.0,uid=0,gid=101,file_mode=0640,dir_mode=0750,ro"
  
  # Certificate Watcher
  CERT_WATCH_PATH: "/etc/nginx/certs/live/example.com/fullchain.pem"  # Update with your domain
```

### Secret

Both certbot and nginx use the same secret:

```bash
echo "your-storage-box-password" > storagebox.txt
docker secret create storagebox_password storagebox.txt
rm storagebox.txt
```

## How It Works

```
Hetzner Storage Box (CIFS/Samba)
//u123456.your-storagebox.de/
            │
            ├─── /certs (Certificates)
            │    ├─ Mounted by certbot at /etc/letsencrypt (read-write)
            │    │  └─ Obtains and renews certificates
            │    └─ Mounted by nginx at /etc/nginx/certs (read-only)
            │       └─ Reads certificates for HTTPS
            │
            └─── /sites (Site Configurations)
                 └─ Mounted by nginx at /etc/nginx/sites-enabled (read-only)
                    └─ Loads nginx site configurations
```

### Startup Sequence

1. **Nginx container starts as root**
2. **Mounts Storage Box paths:**
   - `/certs` → `/etc/nginx/certs` (read-only)
   - `/sites` → `/etc/nginx/sites-enabled` (read-only)
3. **Starts background processes:**
   - Cloudflare IP updater
   - Certificate watcher (watches mounted cert file)
4. **Drops privileges** to `nginx` user using `su-exec`
5. **Starts nginx** process

### Certificate Updates

When certbot renews a certificate:

1. Certbot writes new cert to Storage Box mount
2. Nginx's certificate watcher detects the file change
3. Nginx automatically reloads configuration
4. Zero downtime SSL certificate updates!

## Nginx Configuration Example

In your nginx site config:

```nginx
server {
    listen 443 ssl http2;
    server_name example.com;
    
    # Certificates from Storage Box mount
    ssl_certificate /etc/nginx/certs/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/live/example.com/privkey.pem;
    
    # SSL Configuration
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    
    # Your location blocks...
    location / {
        proxy_pass http://backend;
    }
}
```

## Mount Options Explained

```
vers=3.0         - Use SMB 3.0 protocol
uid=0            - Files owned by root
gid=101          - Files in nginx group (nginx user is gid 101)
file_mode=0640   - Files: rw-r----- (owner read/write, group read)
dir_mode=0750    - Dirs: rwxr-x--- (owner rwx, group rx)
ro               - Read-only mount (nginx only reads, certbot writes)
```

## Managing Site Configurations

### Directory Structure on Storage Box

Create the following structure on your Storage Box:

```
/sites/
  ├── example.com.conf
  ├── api.example.com.conf
  └── blog.example.com.conf
```

### Uploading Configurations

Use SFTP, WebDAV, or CIFS to upload site configs to `/sites/` on your Storage Box:

**Via SFTP:**
```bash
sftp u123456@u123456.your-storagebox.de
cd sites
put example.com.conf
```

**Via scp:**
```bash
scp example.com.conf u123456@u123456.your-storagebox.de:sites/
```

**Via WebDAV:**
```bash
curl -u u123456:password -T example.com.conf \
  https://u123456.your-storagebox.de/sites/example.com.conf
```

### Example Site Configuration

`/sites/example.com.conf`:
```nginx
server {
    listen 443 ssl http2;
    server_name example.com;
    
    # Certificates from Storage Box
    ssl_certificate /etc/nginx/certs/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/live/example.com/privkey.pem;
    ssl_trusted_certificate /etc/nginx/certs/live/example.com/chain.pem;
    
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers on;
    
    # Your location blocks...
    location / {
        proxy_pass http://backend;
    }
}
```

### Reloading After Config Changes

Nginx automatically detects certificate changes via the certificate watcher, but site config changes require a manual reload:

```bash
# Reload nginx to pick up new/changed site configs
docker service update --force nginx_nginx

# Or exec into container and reload
docker exec $(docker ps -qf "name=nginx") nginx -s reload
```

## Deployment

### 1. Prepare Storage Box

Create directories on your Storage Box:

```bash
# Via SFTP
sftp u123456@u123456.your-storagebox.de
mkdir certs  # Will be used by certbot
mkdir sites  # For nginx site configurations
exit
```

### 2. Upload Site Configurations

Upload your nginx site configs to the `/sites/` directory.

### 3. Update Configuration

Edit `nginx/docker-compose.swarm.yml`:
- Set `STORAGE_BOX_HOST` to your Storage Box hostname
- Set `STORAGE_BOX_USER` to your Storage Box username
- Set `STORAGE_BOX_CERTS_ENABLED` to `"true"`
- Set `STORAGE_BOX_SITES_ENABLED` to `"true"`
- Set `CERT_WATCH_PATH` to your certificate path

### 4. Create Secret (if not already created)

```bash
echo "your-storage-box-password" > storagebox.txt
docker secret create storagebox_password storagebox.txt
rm storagebox.txt
```

### 5. Deploy Nginx

```bash
cd nginx/
docker stack deploy -c docker-compose.swarm.yml nginx
```

### 6. Verify

Check logs:

```bash
docker service logs -f nginx_nginx
```

You should see:

```
[2025-12-07 18:00:00] [storage-box-certs] Mounting //u123456.your-storagebox.de/certs to /etc/nginx/certs
[2025-12-07 18:00:00] [storage-box-certs] Successfully mounted at /etc/nginx/certs
[2025-12-07 18:00:01] [storage-box-sites] Mounting //u123456.your-storagebox.de/sites to /etc/nginx/sites-enabled
[2025-12-07 18:00:01] [storage-box-sites] Successfully mounted at /etc/nginx/sites-enabled
[entrypoint] Starting cert watcher for: /etc/nginx/certs/live/example.com/fullchain.pem
[entrypoint] Starting nginx as user nginx
```

## Troubleshooting

### Mount fails

```bash
# Check if Storage Box is accessible
docker exec $(docker ps -qf "name=nginx") mount | grep cifs

# Check credentials
docker secret inspect storagebox_password

# Test mount manually
docker exec -it $(docker ps -qf "name=nginx") bash
mount -t cifs //u123456.your-storagebox.de/certs /mnt/test \
  -o username=u123456,password=your-password,vers=3.0
```

### Site configs not loading

```bash
# Check if sites are mounted
docker exec $(docker ps -qf "name=nginx") ls -la /etc/nginx/sites-enabled/

# Verify mount
docker exec $(docker ps -qf "name=nginx") mount | grep sites-enabled
```

### Certificates not found

```bash
# List mounted certificates
docker exec $(docker ps -qf "name=nginx") ls -la /etc/nginx/certs/live/

# Check if certbot has created certs
docker exec $(docker ps -qf "name=certbot") ls -la /etc/letsencrypt/live/
```

### Permission denied

- Check `STORAGE_BOX_MOUNT_OPTIONS` has correct gid (101 for nginx user)
- Ensure nginx user is in the correct group

## Benefits

- ✅ **No shared volumes** - Direct CIFS mount
- ✅ **Read-only for nginx** - More secure
- ✅ **Real-time updates** - Certificate changes immediately visible
- ✅ **Automatic reload** - Certificate watcher handles nginx reload
- ✅ **Same password** - Both certbot and nginx use same Storage Box secret
- ✅ **Multi-node compatible** - Works across Docker Swarm nodes
