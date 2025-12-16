#!/bin/bash
set -e

echo "=== One-Time MariaDB Export for Migration ==="
echo "This script creates a full backup of MariaDB for swarm migration"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_DIR="/mnt/storagebox/backups"

echo -e "${YELLOW}Step 1: Creating MariaDB Full Backup${NC}"
MARIADB_CONTAINER=$(docker ps -q -f name=mariadb)
if [ -z "$MARIADB_CONTAINER" ]; then
    echo -e "${RED}✗ MariaDB container not found${NC}"
    exit 1
fi

# Create backup directory
mkdir -p ${BACKUP_DIR}/mariadb/full/

# Get root password
MYSQL_ROOT_PASSWORD=$(docker exec $MARIADB_CONTAINER cat /run/secrets/mysql_root_password 2>/dev/null || echo "")
if [ -z "$MYSQL_ROOT_PASSWORD" ]; then
    echo -e "${RED}✗ Could not read MySQL root password${NC}"
    exit 1
fi

# Create backup with timestamp
BACKUP_FILE="${BACKUP_DIR}/mariadb/full/mariadb-full-$(date +%Y%m%d-%H%M%S).sql.bz2"
echo "  Creating backup: $BACKUP_FILE"

# Determine which dump command to use
if docker exec $MARIADB_CONTAINER which mariadb-dump >/dev/null 2>&1; then
    DUMP_CMD="mariadb-dump"
elif docker exec $MARIADB_CONTAINER which mysqldump >/dev/null 2>&1; then
    DUMP_CMD="mysqldump"
else
    echo -e "${RED}✗ Neither mariadb-dump nor mysqldump found in container${NC}"
    exit 1
fi

echo "  Using dump command: $DUMP_CMD"

# Use MYSQL_PWD environment variable to avoid password in process list
if docker exec -e MYSQL_PWD="${MYSQL_ROOT_PASSWORD}" $MARIADB_CONTAINER $DUMP_CMD \
    -u root \
    --all-databases \
    --single-transaction \
    --quick \
    --lock-tables=false \
    --routines \
    --triggers \
    --events 2>&1 | bzip2 > "$BACKUP_FILE"; then
    
    # Check if file is not empty (more than 1KB)
    FILE_SIZE=$(stat -f%z "$BACKUP_FILE" 2>/dev/null || stat -c%s "$BACKUP_FILE" 2>/dev/null)
    if [ "$FILE_SIZE" -lt 1024 ]; then
        echo -e "${RED}✗ Backup file is too small ($FILE_SIZE bytes), something went wrong${NC}"
        echo "  Backup content:"
        bzcat "$BACKUP_FILE" 2>&1
        rm -f "$BACKUP_FILE"
        exit 1
    fi
    
    echo -e "${GREEN}✓ MariaDB backup completed${NC}"
    echo "  Location: $BACKUP_FILE"
    echo "  Size: $(du -h "$BACKUP_FILE" | cut -f1)"
else
    echo -e "${RED}✗ MariaDB backup failed${NC}"
    [ -f "$BACKUP_FILE" ] && rm -f "$BACKUP_FILE"
    exit 1
fi

echo ""
echo -e "${GREEN}=== MariaDB Backup Completed ===${NC}"
echo ""
echo "Backup location: ${BACKUP_DIR}/mariadb/full/"
echo ""
echo "To restore on new swarm setup:"
echo "  1. Deploy the new mariadb service with the updated image"
echo "  2. The new container has BACKUP_AUTO_RESTORE=true enabled"
echo "  3. It will automatically restore from the latest backup on first start"
echo ""
echo "Or restore manually:"
echo "  bzcat ${BACKUP_DIR}/mariadb/full/<backup-file> | \\"
echo "    docker exec -i <container> mysql -u root -p\$(cat /run/secrets/mysql_root_password)"
