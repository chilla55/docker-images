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
echo -e "${YELLOW}Step 2: Creating PostgreSQL Full Backup${NC}"
POSTGRES_CONTAINER=$(docker ps -q -f name=postgresql)
if [ -z "$POSTGRES_CONTAINER" ]; then
    echo -e "${YELLOW}⚠ PostgreSQL container not found, skipping${NC}"
else
    # Check if backup script exists in container
    if docker exec $POSTGRES_CONTAINER test -f /usr/local/bin/backup-full.sh; then
        # New container with backup script
        docker exec $POSTGRES_CONTAINER /usr/local/bin/backup-full.sh
        if [ $? -eq 0 ]; then
            echo -e "${GREEN}✓ PostgreSQL backup completed${NC}"
            echo "  Location: ${BACKUP_DIR}/postgresql/full/"
            ls -lh ${BACKUP_DIR}/postgresql/full/ | tail -5
        else
            echo -e "${RED}✗ PostgreSQL backup failed${NC}"
            exit 1
        fi
echo ""
echo "To restore on new swarm setup:"
echo ""
echo "MariaDB:"
echo "  1. Deploy the mariadb service with new image"
echo "  2. The new container has auto-restore - it will use the backup automatically"
echo "  3. Or restore manually:"
echo "     bzcat /mnt/storagebox/backups/mariadb/full/<backup-file> | docker exec -i <container> mysql -u root -p"
echo ""
echo "PostgreSQL:"
echo "  1. Deploy the postgresql service"
echo "  2. If new container has backup script:"
echo "     docker exec <container> /usr/local/bin/backup-restore.sh /backups/full/<backup-file>"
echo "  3. Or restore manually:"
echo "     bzcat /mnt/storagebox/backups/postgresql/full/<backup-file> | docker exec -i <container> psql -U postgres"
            echo -e "${GREEN}✓ PostgreSQL backup completed${NC}"
            echo "  Location: $BACKUP_FILE"
            echo "  Size: $(du -h "$BACKUP_FILE" | cut -f1)"
        else
            echo -e "${RED}✗ PostgreSQL backup failed${NC}"
            exit 1
        fi
    fi
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
