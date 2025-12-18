#!/bin/sh
set -e

BACKUP_DIR="${BACKUP_DIR:-/mnt/storagebox/backups/proxy}"
RETENTION_DAYS=${RETENTION_DAYS:-35}

find "$BACKUP_DIR" -type f -name '*.db.gz' -mtime +$RETENTION_DAYS -print -delete 2>/dev/null || true
