#!/bin/sh
set -e

BACKUP_DIR="${BACKUP_DIR:-/mnt/storagebox/backups/proxy}"
DB_PATH="${DB_PATH:-/data/proxy.db}"
DATE=$(date +%Y%m%d_%H%M%S)
mkdir -p "$BACKUP_DIR"

sqlite3 "$DB_PATH" "VACUUM INTO '$BACKUP_DIR/full-$DATE.db'"
 gzip "$BACKUP_DIR/full-$DATE.db"

# Keep last 4 weekly full backups
ls -t "$BACKUP_DIR"/full-*.db.gz 2>/dev/null | tail -n +5 | xargs -r rm -f
