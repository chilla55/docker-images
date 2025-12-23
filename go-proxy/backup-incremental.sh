#!/bin/sh
set -e

BACKUP_DIR="${BACKUP_DIR:-/mnt/storagebox/backups/proxy}"
DB_PATH="${DB_PATH:-/data/proxy.db}"
DATE=$(date +%Y%m%d_%H%M%S)
HASH_FILE="/data/.db-hash"
mkdir -p "$BACKUP_DIR"

CURRENT_HASH=$(md5sum "$DB_PATH" | awk '{print $1}')
LAST_HASH=$(cat "$HASH_FILE" 2>/dev/null || true)

if [ "$CURRENT_HASH" != "$LAST_HASH" ]; then
  sqlite3 "$DB_PATH" "VACUUM INTO '$BACKUP_DIR/incr-$DATE.db'"
  gzip "$BACKUP_DIR/incr-$DATE.db"
  echo "$CURRENT_HASH" > "$HASH_FILE"
  # Keep last 168 hourly backups (7 days)
  ls -t "$BACKUP_DIR"/incr-*.db.gz 2>/dev/null | tail -n +169 | xargs -r rm -f
fi
