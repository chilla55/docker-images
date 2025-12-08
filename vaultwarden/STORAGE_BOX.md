# Vaultwarden Storage Box Integration

## Overview

Vaultwarden RSA private keys are now automatically backed up to Hetzner Storage Box for persistence across container restarts and node failures.

## How It Works

### Architecture

```
Container Startup
     │
     ├─► Mount Storage Box via CIFS to /data/keys
     │
     ├─► Check if rsa_key.pem exists on Storage Box
     │   │
     │   ├─► YES: Symlink existing key from /data/keys/ to /data/
     │   │
     │   └─► NO: Start Vaultwarden, generate keys
     │           │
     │           └─► Post-start script:
     │               - Wait for key generation
     │               - Copy keys to Storage Box
     │               - Replace with symlinks
     │
     └─► Vaultwarden uses RSA keys from /data/
```

### Files

1. **entrypoint.sh**: 
   - Mounts Storage Box CIFS volume
   - Creates symlinks if keys exist
   - Launches post-start script if keys need generation

2. **post-start.sh**:
   - Waits up to 60s for Vaultwarden to generate RSA keys
   - Copies `rsa_key.pem` and `rsa_key.pub.pem` to Storage Box
   - Sets correct permissions (600 for private, 644 for public)
   - Replaces original files with symlinks

3. **healthcheck.sh**:
   - Checks `/alive` endpoint via wget

## Storage Box Structure

```
/backup/vaultwarden/
└── keys/
    ├── rsa_key.pem        (600 - private key)
    └── rsa_key.pub.pem    (644 - public key)
```

## Configuration

### Environment Variables (docker-compose.swarm.yml)

```yaml
STORAGE_BOX_KEYS_ENABLED: true
STORAGE_BOX_HOST: u123456.your-storagebox.de
STORAGE_BOX_USER: u123456
STORAGE_BOX_PASSWORD_FILE: /run/secrets/vaultwarden_storagebox_password
STORAGE_BOX_KEYS_PATH: /vaultwarden/keys
STORAGE_BOX_KEYS_MOUNT_POINT: /data/keys
STORAGE_BOX_KEYS_MOUNT_OPTIONS: vers=3.0,uid=0,gid=0,file_mode=0600,dir_mode=0700
```

### Capabilities Required

```yaml
cap_add:
  - SYS_ADMIN       # Required for CIFS mount
  - DAC_READ_SEARCH # Bypass file read permission checks
devices:
  - /dev/fuse       # FUSE filesystem device
```

### Secrets

```yaml
vaultwarden_storagebox_password:
  external: true
  name: storagebox_password  # Shared with nginx/certbot
```

## Benefits

1. **Disaster Recovery**: RSA keys survive node failure
2. **Centralized Backup**: All keys in one Storage Box location
3. **Easy Migration**: Move Vaultwarden to different node - keys restored automatically
4. **Consistent Security**: Same CIFS encryption as nginx/certbot (SMB 3.0)
5. **Automatic**: Zero manual intervention required

## Security

- **CIFS Encryption**: SMB 3.0 (`vers=3.0`)
- **File Permissions**: Private key `0600`, public key `0644`
- **Directory Permissions**: `0700` (owner only)
- **Password Security**: Stored as Docker secret, never in plaintext
- **Network Encryption**: overlay networks use `--opt encrypted=true`

## Testing

### Manual Test

```bash
# Build image
cd vaultwarden
make build

# Deploy (requires secrets)
docker stack deploy -c docker-compose.swarm.yml vaultwarden

# Verify mount
docker exec $(docker ps -q -f name=vaultwarden) mount | grep vaultwarden
# Expected: //u123456.your-storagebox.de/backup/vaultwarden/keys on /data/keys type cifs

# Check keys
docker exec $(docker ps -q -f name=vaultwarden) ls -la /data/
# Expected: rsa_key.pem -> /data/keys/rsa_key.pem
```

### Verify Storage Box

```bash
# SSH to Storage Box
ssh u123456@u123456.your-storagebox.de

# Check keys exist
ls -la /backup/vaultwarden/keys/
# Expected:
# -rw------- 1 u123456 u123456 1675 date rsa_key.pem
# -rw-r--r-- 1 u123456 u123456  451 date rsa_key.pub.pem
```

## Troubleshooting

### Mount Failed

```bash
# Check logs
docker service logs vaultwarden_vaultwarden | grep -i mount

# Common issues:
# 1. Wrong credentials
# 2. CIFS not installed (should be in image)
# 3. SYS_ADMIN capability missing
# 4. /dev/fuse device not available
```

### Keys Not Generated

```bash
# Check Vaultwarden logs
docker service logs vaultwarden_vaultwarden | grep -i rsa

# Check post-start script
docker exec $(docker ps -q -f name=vaultwarden) cat /usr/local/bin/post-start.sh
```

### Permission Denied

```bash
# Verify capabilities
docker inspect $(docker ps -q -f name=vaultwarden) | jq '.[0].HostConfig.CapAdd'
# Expected: ["SYS_ADMIN", "DAC_READ_SEARCH"]

# Verify device
docker inspect $(docker ps -q -f name=vaultwarden) | jq '.[0].HostConfig.Devices'
# Expected: [{"PathOnHost": "/dev/fuse", ...}]
```

## Migration Guide

### Moving to New Node

1. Deploy Vaultwarden on new node:
```bash
docker node update --label-add vaultwarden.node=node2 <new-node>
docker service update --constraint-add 'node.labels.vaultwarden.node==node2' vaultwarden_vaultwarden
```

2. Keys automatically restored from Storage Box on startup

### Recovery from Backup

If Storage Box keys lost:

1. Stop Vaultwarden
2. Upload backup keys to Storage Box:
```bash
scp rsa_key.* u123456@u123456.your-storagebox.de:/backup/vaultwarden/keys/
```
3. Restart Vaultwarden - will use restored keys

## Implementation Notes

- Uses same Storage Box secret as nginx/certbot (shared `storagebox_password`)
- CIFS mount happens before Vaultwarden starts
- Post-start script runs in background (doesn't block startup)
- Symlinks allow Vaultwarden to see keys at expected `/data/` location
- 60-second timeout for key generation (should happen in <5s)

## Version

Initial implementation: v1.0.0
