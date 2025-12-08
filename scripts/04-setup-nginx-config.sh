#!/bin/bash
# ============================================================================
# Nginx Config Setup Script for Docker Swarm
# ============================================================================
# Creates Docker config for nginx.conf
# Run this on the Swarm manager node (mail)
# ============================================================================

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NGINX_DIR="$(dirname "$SCRIPT_DIR")/nginx"

echo "============================================================================"
echo "Creating Nginx Docker Config"
echo "============================================================================"

# Check if nginx.conf exists
if [ ! -f "$NGINX_DIR/nginx.conf" ]; then
    echo -e "${RED}✗ nginx.conf not found at $NGINX_DIR/nginx.conf${NC}"
    exit 1
fi

# Remove old config if it exists
if docker config inspect nginx_conf &>/dev/null; then
    echo -e "${YELLOW}Config nginx_conf already exists.${NC}"
    echo -e "${YELLOW}Note: Docker configs are immutable. To update, you must:${NC}"
    echo "  1. Remove the old config (after stopping services using it)"
    echo "  2. Create a new config with a different name or version"
    echo ""
    echo "Current config info:"
    docker config inspect nginx_conf --format '{{.ID}} - Created: {{.CreatedAt}}'
    echo ""
    read -p "Do you want to create a versioned config instead? (nginx_conf_v2) [y/N]: " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        CONFIG_NAME="nginx_conf_v2"
    else
        echo "Skipping config creation."
        exit 0
    fi
else
    CONFIG_NAME="nginx_conf"
fi

# Create config from file
echo ""
echo "Creating Docker config from $NGINX_DIR/nginx.conf..."
docker config create "$CONFIG_NAME" "$NGINX_DIR/nginx.conf"
echo -e "${GREEN}✓ Created config: $CONFIG_NAME${NC}"

echo ""
echo "============================================================================"
echo "Nginx config creation complete!"
echo "============================================================================"
echo ""
echo "Verify config:"
echo "  docker config ls"
echo "  docker config inspect $CONFIG_NAME"
