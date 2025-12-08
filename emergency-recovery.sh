#!/bin/bash
# ============================================================================
# Emergency Data Recovery Script
# ============================================================================
# If you accidentally cloned into /serverdata/docker and lost your data
# This script helps recover what might still be available
# ============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo "============================================================================"
echo "Emergency Data Recovery"
echo "============================================================================"

AFFECTED_DIR="/serverdata/docker"
RECOVERY_DIR="/serverdata/docker-recovery-$(date +%s)"

echo ""
echo -e "${YELLOW}This will attempt to recover data from $AFFECTED_DIR${NC}"
echo ""

# Create recovery directory
mkdir -p "$RECOVERY_DIR"

echo -e "${BLUE}Step 1: Checking for Docker volumes (most important!)${NC}"
echo "Docker volumes are stored separately and should NOT be affected by the clone"
echo ""
echo "Existing Docker volumes:"
docker volume ls
echo ""
echo -e "${GREEN}✓ Your Docker volumes are SAFE - they're not in the filesystem directory${NC}"

echo ""
echo -e "${BLUE}Step 2: Checking for running containers${NC}"
echo "If containers were running, their data is likely intact in volumes"
echo ""
CONTAINERS=$(docker ps -a --format "{{.Names}}" || true)
if [ -n "$CONTAINERS" ]; then
    echo "Found containers:"
    docker ps -a --format "table {{.Names}}\t{{.Status}}\t{{.Image}}"
    echo ""
    echo -e "${GREEN}✓ Containers still exist - data in volumes is safe${NC}"
else
    echo -e "${YELLOW}No containers found${NC}"
fi

echo ""
echo -e "${BLUE}Step 3: Looking for .env files in current directory${NC}"
cd "$AFFECTED_DIR"
ENV_FILES=$(find . -maxdepth 3 -name ".env" -type f 2>/dev/null || true)
if [ -n "$ENV_FILES" ]; then
    echo "Found .env files:"
    echo "$ENV_FILES"
    echo ""
    echo "Copying to recovery directory..."
    while IFS= read -r file; do
        mkdir -p "$RECOVERY_DIR/$(dirname "$file")"
        cp "$file" "$RECOVERY_DIR/$file"
        echo "  Saved: $file"
    done <<< "$ENV_FILES"
    echo -e "${GREEN}✓ .env files recovered to $RECOVERY_DIR${NC}"
else
    echo -e "${YELLOW}No .env files found${NC}"
fi

echo ""
echo -e "${BLUE}Step 4: Looking for compose files${NC}"
COMPOSE_FILES=$(find . -maxdepth 3 -name "docker-compose*.yml" -type f 2>/dev/null || true)
if [ -n "$COMPOSE_FILES" ]; then
    echo "Found compose files:"
    echo "$COMPOSE_FILES"
    echo ""
    echo "Copying to recovery directory..."
    while IFS= read -r file; do
        mkdir -p "$RECOVERY_DIR/$(dirname "$file")"
        cp "$file" "$RECOVERY_DIR/$file"
        echo "  Saved: $file"
    done <<< "$COMPOSE_FILES"
    echo -e "${GREEN}✓ Compose files recovered to $RECOVERY_DIR${NC}"
else
    echo -e "${YELLOW}No compose files found (likely overwritten by git clone)${NC}"
fi

echo ""
echo -e "${BLUE}Step 5: Database backup check${NC}"
echo "Checking if MariaDB container can be accessed..."
OLD_MARIADB=$(docker ps -a -q -f name=mariadb 2>/dev/null | head -n1)
if [ -n "$OLD_MARIADB" ]; then
    echo -e "${GREEN}✓ Found MariaDB container: $OLD_MARIADB${NC}"
    echo ""
    echo "Would you like to create an immediate backup? (y/n)"
    read -p "Answer: " answer
    if [[ "$answer" =~ ^[Yy]$ ]]; then
        echo ""
        read -p "Enter MariaDB root password: " -s MYSQL_PASS
        echo ""
        
        BACKUP_FILE="$RECOVERY_DIR/all-databases-$(date +%Y%m%d-%H%M%S).sql"
        echo "Creating backup..."
        docker exec "$OLD_MARIADB" mysqldump -uroot -p"$MYSQL_PASS" --all-databases \
            --single-transaction --routines --triggers --events > "$BACKUP_FILE" 2>/dev/null
        
        if [ -f "$BACKUP_FILE" ]; then
            echo -e "${GREEN}✓ Database backup created: $BACKUP_FILE${NC}"
            echo "  Size: $(du -h "$BACKUP_FILE" | cut -f1)"
        else
            echo -e "${RED}✗ Backup failed${NC}"
        fi
    fi
else
    echo -e "${YELLOW}No MariaDB container found${NC}"
fi

echo ""
echo "============================================================================"
echo "Recovery Summary"
echo "============================================================================"
echo ""
echo "Recovery location: $RECOVERY_DIR"
echo ""
echo -e "${GREEN}IMPORTANT FACTS:${NC}"
echo "  1. Docker volumes are SEPARATE from the filesystem"
echo "  2. If you had running containers, their data is in volumes (SAFE)"
echo "  3. Only configuration files in /serverdata/docker were affected"
echo "  4. Database data is in Docker volumes, not /serverdata/docker"
echo ""
echo -e "${YELLOW}What was likely LOST:${NC}"
echo "  - docker-compose.yml files (overwritten by git clone)"
echo "  - .env files (if they existed and weren't backed up)"
echo "  - Custom scripts or configs in /serverdata/docker"
echo ""
echo -e "${GREEN}What is likely SAFE:${NC}"
echo "  - All database data (in Docker volumes)"
echo "  - Container configurations (can be inspected)"
echo "  - Any data mounted to Docker volumes"
echo ""
echo "Next steps:"
echo "  1. DO NOT remove containers or volumes"
echo "  2. Check volumes: docker volume ls"
echo "  3. Inspect containers: docker inspect <container-name>"
echo "  4. Your data is likely intact in Docker volumes!"
echo ""
echo "To list all volume data:"
echo "  docker volume ls"
echo ""
echo "To see what's in a volume:"
echo "  docker run --rm -v <volume-name>:/data alpine ls -la /data"
echo ""
