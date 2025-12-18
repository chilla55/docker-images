#!/bin/sh
set -e

BACKUP_DIR="${BACKUP_DIR:-/mnt/storagebox/backups/proxy}"
DB_PATH="${DB_PATH:-/data/proxy.db}"
DATE=$(date +%Y%m%d_%H%M%S)
mkdir -p "$BACKUP_DIR"

LAST_FULL=$(ls -t "$BACKUP_DIR"/full-*.db.gz 2>/dev/null | head -1)
if [ -z "$LAST_FULL" ]; then
  echo "No full backup found; run full backup first." >&2
  exit 1
fi

gunzip -c "$LAST_FULL" > /tmp/last-full.db
rsync -a --inplace /tmp/last-full.db "$BACKUP_DIR/diff-base-$DATE.db" >/dev/null 2>&1 || true
# For SQLite, safest is to snapshot again; differential simulated here
sqlite3 "$DB_PATH" "VACUUM INTO '$BACKUP_DIR/diff-$DATE.db'"
gzip "$BACKUP_DIR/diff-$DATE.db"

# Keep last 8 differentials
ls -t "$BACKUP_DIR"/diff-*.db.gz 2>/dev/null | tail -n +9 | xargs -r rm -f
rm -f "$BACKUP_DIR"/diff-base-$DATE.db /tmp/last-full.db
