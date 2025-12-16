#!/bin/bash
set -e

echo "=== One-Time Database Export for Migration ==="
echo "This script triggers immediate backups of all databases"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP_DIR="/mnt/storagebox/backups"

echo -e "${YELLOW}Step 1: Triggering MariaDB Full Backup${NC}"
docker exec $(docker ps -q -f name=mariadb) /usr/local/bin/backup-full.sh
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ MariaDB backup completed${NC}"
    echo "  Location: ${BACKUP_DIR}/mariadb/full/"
    ls -lh ${BACKUP_DIR}/mariadb/full/ | tail -5
else
    echo -e "${RED}✗ MariaDB backup failed${NC}"
    exit 1
fi

echo ""
echo -e "${YELLOW}Step 2: Triggering PostgreSQL Full Backup${NC}"
docker exec $(docker ps -q -f name=postgresql) /usr/local/bin/backup-full.sh
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ PostgreSQL backup completed${NC}"
    echo "  Location: ${BACKUP_DIR}/postgresql/full/"
    ls -lh ${BACKUP_DIR}/postgresql/full/ | tail -5
else
    echo -e "${RED}✗ PostgreSQL backup failed${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}=== All Database Backups Completed ===${NC}"
echo ""
echo "Backup locations:"
echo "  MariaDB:    ${BACKUP_DIR}/mariadb/full/"
echo "  PostgreSQL: ${BACKUP_DIR}/postgresql/full/"
echo ""
echo "To restore on new swarm setup:"
echo "  1. Deploy the database services"
echo "  2. Remove the volumes: docker volume rm <stack>_mariadb-data <stack>_postgresql-data"
echo "  3. Redeploy the services - they will auto-restore from backup"
echo ""
echo "Or restore manually:"
echo "  MariaDB:    docker exec <container> /usr/local/bin/backup-restore.sh /backups/full/<backup-file>"
echo "  PostgreSQL: docker exec <container> /usr/local/bin/backup-restore.sh /backups/full/<backup-file>"
