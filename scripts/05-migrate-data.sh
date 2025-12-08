#!/bin/bash
# ============================================================================
# Data Migration Script - Migrate from old MariaDB to new Swarm deployment
# ============================================================================
# This script helps migrate data from your old MariaDB container to the new
# Swarm deployment with new passwords
# 
# Run this AFTER deploying MariaDB stack but BEFORE deploying applications
# ============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo "============================================================================"
echo "Data Migration Script - Old MariaDB to New Swarm Deployment"
echo "============================================================================"

# Retrieve new passwords from secrets
NEW_ROOT_PASS=$(docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d)
NEW_PTERO_PASS=$(docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d)
NEW_VW_PASS=$(docker secret inspect vaultwarden_db_password -f '{{.Spec.Data}}' | base64 -d)

echo ""
echo -e "${BLUE}This script will:${NC}"
echo "  1. Connect to your old MariaDB container"
echo "  2. Export Pterodactyl and Vaultwarden databases"
echo "  3. Import them into the new Swarm MariaDB"
echo "  4. Create users with new random passwords"
echo ""
echo -e "${YELLOW}Prerequisites:${NC}"
echo "  - Old MariaDB container must be running"
echo "  - New MariaDB stack must be deployed and healthy"
echo "  - You need the old MariaDB password (123lol789)"
echo ""
read -p "Press Enter to continue or Ctrl+C to cancel..."

# Prompt for old container details
echo ""
echo -e "${YELLOW}Enter old MariaDB container name or ID:${NC}"
read -p "Old container: " OLD_CONTAINER

echo -e "${YELLOW}Enter old MariaDB root password:${NC}"
read -s OLD_ROOT_PASS
echo ""

# Verify old container is accessible
echo ""
echo "Verifying connection to old MariaDB..."
if ! docker exec "$OLD_CONTAINER" mysql -uroot -p"$OLD_ROOT_PASS" -e "SELECT 1;" &>/dev/null; then
    echo -e "${RED}✗ Cannot connect to old MariaDB container${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Connected to old MariaDB${NC}"

# Verify new MariaDB is accessible
echo ""
echo "Verifying connection to new MariaDB..."
NEW_CONTAINER=$(docker ps -q -f name=mariadb_mariadb-primary | head -n1)
if [ -z "$NEW_CONTAINER" ]; then
    echo -e "${RED}✗ New MariaDB primary container not found${NC}"
    exit 1
fi

if ! docker exec "$NEW_CONTAINER" mysql -uroot -p"$NEW_ROOT_PASS" -e "SELECT 1;" &>/dev/null; then
    echo -e "${RED}✗ Cannot connect to new MariaDB container${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Connected to new MariaDB${NC}"

# Create temporary directory for dumps
DUMP_DIR=$(mktemp -d)
echo ""
echo "Using temporary directory: $DUMP_DIR"

# Export Pterodactyl database
echo ""
echo -e "${BLUE}Exporting Pterodactyl database...${NC}"
if docker exec "$OLD_CONTAINER" mysql -uroot -p"$OLD_ROOT_PASS" -e "SHOW DATABASES LIKE 'panel';" | grep -q panel; then
    docker exec "$OLD_CONTAINER" mysqldump -uroot -p"$OLD_ROOT_PASS" \
        --single-transaction \
        --routines \
        --triggers \
        --events \
        panel > "$DUMP_DIR/panel.sql"
    echo -e "${GREEN}✓ Pterodactyl database exported${NC}"
    MIGRATE_PTERO=true
else
    echo -e "${YELLOW}⚠ Pterodactyl database 'panel' not found in old container, skipping${NC}"
    MIGRATE_PTERO=false
fi

# Export Vaultwarden database
echo ""
echo -e "${BLUE}Exporting Vaultwarden database...${NC}"
if docker exec "$OLD_CONTAINER" mysql -uroot -p"$OLD_ROOT_PASS" -e "SHOW DATABASES LIKE 'vaultwarden';" | grep -q vaultwarden; then
    docker exec "$OLD_CONTAINER" mysqldump -uroot -p"$OLD_ROOT_PASS" \
        --single-transaction \
        --routines \
        --triggers \
        --events \
        vaultwarden > "$DUMP_DIR/vaultwarden.sql"
    echo -e "${GREEN}✓ Vaultwarden database exported${NC}"
    MIGRATE_VW=true
else
    echo -e "${YELLOW}⚠ Vaultwarden database not found in old container, skipping${NC}"
    MIGRATE_VW=false
fi

# Import Pterodactyl database
if [ "$MIGRATE_PTERO" = true ]; then
    echo ""
    echo -e "${BLUE}Importing Pterodactyl database to new MariaDB...${NC}"
    
    # Create database
    docker exec -i "$NEW_CONTAINER" mysql -uroot -p"$NEW_ROOT_PASS" <<EOF
DROP DATABASE IF EXISTS panel;
CREATE DATABASE panel CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
EOF
    
    # Import data
    docker exec -i "$NEW_CONTAINER" mysql -uroot -p"$NEW_ROOT_PASS" panel < "$DUMP_DIR/panel.sql"
    
    # Drop old users and create new user with new password
    docker exec -i "$NEW_CONTAINER" mysql -uroot -p"$NEW_ROOT_PASS" <<EOF
-- Remove old users (paneluser, pterodactyl, etc.)
DROP USER IF EXISTS 'paneluser'@'%';
DROP USER IF EXISTS 'paneluser'@'localhost';
DROP USER IF EXISTS 'pterodactyl'@'%';
# Import Vaultwarden database
if [ "$MIGRATE_VW" = true ]; then
    echo ""
    echo -e "${BLUE}Importing Vaultwarden database to new MariaDB...${NC}"
    
    # Create database
    docker exec -i "$NEW_CONTAINER" mysql -uroot -p"$NEW_ROOT_PASS" <<EOF
DROP DATABASE IF EXISTS vaultwarden;
CREATE DATABASE vaultwarden CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
EOF
    
    # Import data
    docker exec -i "$NEW_CONTAINER" mysql -uroot -p"$NEW_ROOT_PASS" vaultwarden < "$DUMP_DIR/vaultwarden.sql"
    
    # Drop old users and create new user with new password
    docker exec -i "$NEW_CONTAINER" mysql -uroot -p"$NEW_ROOT_PASS" <<EOF
-- Remove old users
DROP USER IF EXISTS 'vaultwarden'@'%';
DROP USER IF EXISTS 'vaultwarden'@'localhost';
DROP USER IF EXISTS 'bitwarden'@'%';
DROP USER IF EXISTS 'bitwarden'@'localhost';

-- Create new user with new random password
CREATE USER 'vaultwarden'@'%' IDENTIFIED BY '$NEW_VW_PASS';
GRANT ALL PRIVILEGES ON vaultwarden.* TO 'vaultwarden'@'%';
FLUSH PRIVILEGES;
EOF
    echo -e "${GREEN}✓ Vaultwarden database imported${NC}"
    echo -e "${GREEN}✓ New user 'vaultwarden' created with random password${NC}"
fi  # Create database and user
    docker exec -i "$NEW_CONTAINER" mysql -uroot -p"$NEW_ROOT_PASS" <<EOF
DROP DATABASE IF EXISTS vaultwarden;
CREATE DATABASE vaultwarden CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
DROP USER IF EXISTS 'vaultwarden'@'%';
CREATE USER 'vaultwarden'@'%' IDENTIFIED BY '$NEW_VW_PASS';
GRANT ALL PRIVILEGES ON vaultwarden.* TO 'vaultwarden'@'%';
FLUSH PRIVILEGES;
EOF
    
    # Import data
    docker exec -i "$NEW_CONTAINER" mysql -uroot -p"$NEW_ROOT_PASS" vaultwarden < "$DUMP_DIR/vaultwarden.sql"
    echo -e "${GREEN}✓ Vaultwarden database imported with new password${NC}"
fi

# Cleanup
echo ""
echo "Cleaning up temporary files..."
rm -rf "$DUMP_DIR"
echo -e "${GREEN}✓ Cleanup complete${NC}"

# Summary
echo ""
echo "============================================================================"
echo -e "${GREEN}Migration Complete!${NC}"
echo "============================================================================"
echo ""
echo "Databases migrated with NEW random passwords:"
echo ""
if [ "$MIGRATE_PTERO" = true ]; then
    echo "Pterodactyl (panel):"
    echo "  - Username: ptero"
    echo "  - Password: (stored in secret: pterodactyl_db_password)"
    echo "  - Retrieve: docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d"
fi
echo ""
if [ "$MIGRATE_VW" = true ]; then
    echo "Vaultwarden:"
    echo "  - Username: vaultwarden"
    echo "  - Password: (stored in secret: vaultwarden_db_password)"
    echo "  - Retrieve: docker secret inspect vaultwarden_db_password -f '{{.Spec.Data}}' | base64 -d"
fi
echo ""
echo "MariaDB Root Password:"
echo "  - Password: (stored in secret: mysql_root_password)"
echo "  - Retrieve: docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d"
echo ""
echo -e "${YELLOW}IMPORTANT: Your applications are now configured to use the new passwords${NC}"
echo -e "${YELLOW}stored in Docker secrets. You can now deploy Pterodactyl and Vaultwarden.${NC}"
echo ""
echo "Next steps:"
echo "  1. Deploy Pterodactyl: cd /serverdata/docker/petrodactyl && docker stack deploy -c docker-compose.swarm.yml pterodactyl"
echo "  2. Deploy Vaultwarden: cd /serverdata/docker/vaultwarden && docker stack deploy -c docker-compose.swarm.yml vaultwarden"
echo ""
