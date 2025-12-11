# Certbot Cleanup Summary

## Changes Made

### ✅ Removed (Container Mounting Approach)
- All mount functions (`mount_storage_box_smb3`, `mount_storage_box_sshfs`, `mount_storage_box_cifs`, `unmount_storage_box`)
- Storage Box environment variables (host, user, path, SSH port, mount options)
- `storagebox_password` secret (not needed in container)
- Docker capabilities (`SYS_ADMIN`, `SYS_MODULE`, `DAC_READ_SEARCH`)
- `apparmor:unconfined` security option
- Mount-related packages from Dockerfile (`cifs-utils`, `sshfs`, `sshpass`)
- Obsolete documentation files

### ✅ Kept (Host Mount Approach)
- **Cloudflare DNS** integration
- **Dry-run mode** (`CERTBOT_DRY_RUN`)
- **Certificate issuance/renewal** logic
- **Health check**
- **Host bind-mount**: `/mnt/storagebox/certs` → `/etc/letsencrypt`

### ✅ Simplified
- `entrypoint.sh`: 398 lines → 268 lines (-130 lines, -33%)
- Only `verify_storage_mount()` function remains (checks if mount exists)
- Dockerfile: Only bash + curl (removed mount tools)
- docker-compose.yml: No special capabilities needed

## Architecture

```
Host (srv2):
  └─ /mnt/storagebox ← mounted via fstab (SMB3)
       └─ /certs ← certificate storage
            └─ bind-mounted into container

Container (certbot):
  └─ /etc/letsencrypt ← bind-mount from host
       └─ certbot writes here
```

## Benefits

1. **Simpler** - No container-level mount complexity
2. **More reliable** - Host mounts are stable, no Docker permission issues
3. **Secure** - No elevated capabilities needed
4. **Maintainable** - Less code, easier to debug
5. **Persistent** - fstab ensures mount survives reboots

## Files

- `setup-storagebox-manual.sh` - Setup Storage Box on each host
- `FSTAB_SETUP.md` - fstab configuration guide
- All mount logic removed from container
