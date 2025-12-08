# Certbot with Hetzner Storage Box - CIFS/Samba Direct Mount

## Summary of Changes

This implementation has been **simplified** from the original rclone-based sync approach to use **direct CIFS/Samba mounting** of the Hetzner Storage Box.

### Why CIFS Mount is Better

**Original Approach (rclone):**
- ❌ Complex setup with rclone configuration
- ❌ Requires sync process after each renewal
- ❌ Potential sync delays
- ❌ Need to handle sync failures
- ❌ Nginx reads from local volume, certbot syncs to Storage Box

**New Approach (CIFS mount):**
- ✅ Simple CIFS mount - just hostname, username, password
- ✅ No sync needed - direct write to Storage Box
- ✅ Real-time updates visible to all containers
- ✅ Simpler error handling
- ✅ Both certbot and nginx can read from same mounted Storage Box

## How It Works

```
┌─────────────────────────────────────────────────────────────┐
│  Hetzner Storage Box (Samba/CIFS share)                     │
│  //u123456.your-storagebox.de/certs/                        │
│                                                              │
│  /live/example.com/                                          │
│  ├── fullchain.pem                                           │
│  ├── privkey.pem                                             │
│  ├── cert.pem                                                │
│  └── chain.pem                                               │
└─────────────────────────────────────────────────────────────┘
                        ▲                    ▲
                        │                    │
                  (CIFS mount)         (CIFS mount)
                        │                    │
         ┌──────────────┴──────┐   ┌────────┴──────────┐
         │  Certbot Container  │   │  Nginx Container  │
         │  /etc/letsencrypt   │   │  /etc/nginx/certs │
         │  (writes certs)     │   │  (reads certs)    │
         └─────────────────────┘   └───────────────────┘
```

## Key Changes Made

### 1. Dockerfile
- **Removed**: `rclone` package
- **Added**: `cifs-utils` package for CIFS/Samba mounting
- **Added**: `/mnt/storagebox` directory creation

### 2. entrypoint.sh
- **Removed**: All rclone sync functions (`sync_to_storage_box`, `sync_from_storage_box`, `setup_rclone`)
- **Added**: `mount_storage_box()` function using CIFS mount
- **Added**: `unmount_storage_box()` cleanup function
- **Simplified**: No sync needed in renewal loop - certs written directly to mount
- **Changed**: Mount happens at startup, /etc/letsencrypt becomes the CIFS mount point

### 3. docker-compose.swarm.yml
- **Removed**: `rclone_config` secret
- **Removed**: `certbot_data` volume (no local storage needed)
- **Added**: `storagebox_password` secret
- **Added**: `SYS_ADMIN` and `DAC_READ_SEARCH` capabilities (required for CIFS mount)
- **Added**: `/dev/fuse` device access
- **Added**: Storage Box configuration environment variables:
  - `STORAGE_BOX_HOST`
  - `STORAGE_BOX_USER`
  - `STORAGE_BOX_PASSWORD_FILE`
  - `STORAGE_BOX_PATH`
  - `STORAGE_BOX_MOUNT_OPTIONS`

### 4. Setup Scripts
- **Removed**: `rclone.conf.example`
- **Removed**: rclone password encryption instructions
- **Added**: `storagebox.txt.example` (plain password file)
- **Modified**: `setup.sh` now creates `storagebox_password` secret instead of `rclone_config`
- **Modified**: `test-storagebox.sh` now tests CIFS mount instead of rclone connection

### 5. Configuration
**Before (rclone.conf):**
```ini
[hetzner-storagebox]
type = sftp
host = u123456.your-storagebox.de
user = u123456
port = 23
pass = ENCRYPTED_PASSWORD
shell_type = unix
```

**After (docker-compose.swarm.yml):**
```yaml
environment:
  STORAGE_BOX_HOST: u123456.your-storagebox.de
  STORAGE_BOX_USER: u123456
  STORAGE_BOX_PASSWORD_FILE: /run/secrets/storagebox_password
  STORAGE_BOX_PATH: /certs
```

## Setup Process

### Old Way (rclone):
1. Create Cloudflare credentials
2. Create rclone config with encrypted password
3. Test rclone connection
4. Create both secrets
5. Deploy

### New Way (CIFS):
1. Create Cloudflare credentials
2. Create plain password file
3. Test CIFS mount (sudo required)
4. Create both secrets
5. Deploy

## Next Step: Configure Nginx

Nginx will also mount the same Storage Box to read certificates. This will be configured separately after certbot is working.

**Nginx will:**
- Mount the same Storage Box via CIFS
- Read certificates from `/etc/nginx/certs` (same remote path)
- No need for shared Docker volumes
- Each container has its own CIFS mount to the same storage

## Benefits

1. **Simplicity**: No rclone config, no password encryption, no sync
2. **Real-time**: Certificate updates immediately visible
3. **Reliability**: No sync failures to handle
4. **Native**: Uses standard Linux CIFS mounting
5. **Transparent**: Both containers see the same files
6. **Hetzner Native**: Samba/CIFS is natively supported by all Storage Boxes

## Requirements

- Hetzner Storage Box with Samba/CIFS enabled (in Robot panel)
- Docker Swarm with manager node (for SYS_ADMIN capability)
- Network access from Docker host to Storage Box (port 445/tcp for SMB)

## Testing

Before deploying, test the CIFS connection:

```bash
sudo ./test-storagebox.sh
```

This will:
1. Prompt for Storage Box credentials
2. Mount the Storage Box
3. Test read/write operations
4. Unmount and cleanup
5. Show configuration to use in docker-compose.swarm.yml
