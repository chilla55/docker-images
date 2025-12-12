# Nginx Folder Cleanup Summary

## Date: December 12, 2025

### Files Removed
- **STORAGE_BOX.md** - Outdated documentation describing old CIFS mount approach (3.2KB)

### Files Updated

#### 1. README.md
**Changes:**
- Added "Sites Watcher" to feature list
- Added "Host Bind Mounts" to feature list
- Added "Prerequisites" section with storagebox mount paths
- Updated network name from `proxy` to `web-net` (aligned with actual setup)
- Added reference to STORAGEBOX_SETUP.md for detailed configuration
- Removed outdated `CF_REALIP_MAX_FAILS` from environment variables table
- Added new environment variables:
  - `SITES_WATCH_PATH`
  - `SITES_WATCH_INTERVAL`
  - `SITES_WATCH_DEBUG`
- Updated "Site Configurations" section to reflect bind mounts from host
- Simplified "Health Checks" section
- Removed incomplete architecture diagram
- Added new "Watchers" section explaining both certificate and sites watchers

#### 2. docker-compose.swarm.yml
**Changes:**
- Removed entire `secrets:` section (storagebox_password no longer needed)
- Updated comment about CIFS mounts (now uses bind mounts instead)
- Removed `cap_add` (SYS_ADMIN, DAC_READ_SEARCH) - not needed for bind mounts
- Removed `devices` (no /dev/fuse needed for bind mounts)
- Removed `CF_REALIP_MAX_FAILS` environment variable
- Removed `secrets:` reference from service definition
- Added clear comments about bind mount functionality

#### 3. .dockerignore
**Changes:**
- Maintained existing ignores
- Added `.editorconfig` to ignore list
- Added `.prettierrc` to ignore list
- Added section comments for clarity

#### 4. Dockerfile
**No changes needed** - Already correctly includes watch-sites-reload.sh

#### 5. entrypoint.sh
**No changes needed** - Already correctly set up to start both watchers

### Files Created (Previously in this session)
- **STORAGEBOX_SETUP.md** - New comprehensive guide for storagebox configuration with bind mounts
- **watch-sites-reload.sh** - New script for monitoring site configuration changes

### Cleanup Results

**Before cleanup:**
- 13 files in nginx folder
- Duplicate/conflicting documentation (STORAGE_BOX.md and STORAGEBOX_SETUP.md)
- Outdated references to CIFS mounting in multiple files
- Unused environment variables and secrets

**After cleanup:**
- 12 files in nginx folder
- Single source of truth for storagebox setup (STORAGEBOX_SETUP.md)
- All documentation consistent with bind mount approach
- Removed unused capabilities and secrets
- Cleaner, more maintainable configuration

### Key Architectural Changes Reflected
1. **From CIFS to Bind Mounts**: No longer mounting CIFS inside container; host handles storagebox mounting
2. **From Static to Dynamic**: Sites watcher enables automatic nginx reload when configurations change
3. **Simplified Security**: Removed unnecessary capabilities (SYS_ADMIN, DAC_READ_SEARCH) since CIFS mounting is host-level
4. **Better Error Handling**: Sites watcher validates configs before reloading, preventing crashes

### Testing Recommendations
1. Verify storagebox is properly mounted on host at `/mnt/storagebox`
2. Test certificate watcher with `CERT_WATCH_PATH` set
3. Test sites watcher by adding/modifying/removing site configs in `/mnt/storagebox/sites`
4. Monitor container logs during deployment to verify all watchers start correctly
