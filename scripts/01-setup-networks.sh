#!/bin/bash
# ============================================================================
# Network Setup Script for Docker Swarm
# ============================================================================
# Creates all required overlay networks for the service stack
# Run this on the Swarm manager node (mail)
# ============================================================================

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "============================================================================"
echo "Creating Docker Swarm Overlay Networks"
echo "============================================================================"

# Function to create network if it doesn't exist
create_network() {
    local network_name=$1
    local encrypted=${2:-false}
    
    if docker network inspect "$network_name" &>/dev/null; then
        echo -e "${YELLOW}Network $network_name already exists, skipping${NC}"
    else
        if [ "$encrypted" == "true" ]; then
            docker network create \
                --driver overlay \
                --attachable \
                --opt encrypted \
                "$network_name"
        else
            docker network create \
                --driver overlay \
                --attachable \
                "$network_name"
        fi
        echo -e "${GREEN}✓ Created network: $network_name${NC}"
    fi
}

# Create networks
echo ""
echo "Creating networks..."
create_network "web-net" false
create_network "mariadb-net" true
create_network "postgres-net" true
create_network "redis-net" true

echo ""
echo "============================================================================"
echo "Network creation complete!"
echo "============================================================================"
echo ""
echo "Verify networks:"
echo "  docker network ls --filter driver=overlay"
