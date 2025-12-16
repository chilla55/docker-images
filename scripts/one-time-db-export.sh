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

docker exec $MARIADB_CONTAINER mysqldump \
    -u root \
    -p"${MYSQL_ROOT_PASSWORD}" \
    --all-databases \
    --single-transaction \
    --quick \
    --lock-tables=false \
    --routines \
    --triggers \
    --events | bzip2 > "$BACKUP_FILE"

if [ $? -eq 0 ] && [ -f "$BACKUP_FILE" ]; then
    echo -e "${GREEN}✓ MariaDB backup completed${NC}"
    echo "  Location: $BACKUP_FILE"
    echo "  Size: $(du -h "$BACKUP_FILE" | cut -f1)"
else
    echo -e "${RED}✗ MariaDB backup failed${NC}"
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
