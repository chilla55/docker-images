#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "=== Connect Non-Swarm Container to web-net ==="
echo ""

# Check if container name provided
if [ -z "$1" ]; then
    echo -e "${RED}Error: Container name or ID required${NC}"
    echo ""
    echo "Usage: $0 <container-name-or-id>"
    echo ""
    echo "Example:"
    echo "  $0 my-app-container"
    echo ""
    echo "Available non-swarm containers:"
    docker ps --filter "label=com.docker.swarm.service.id=" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}" 2>/dev/null || docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
    exit 1
fi

CONTAINER=$1

# Check if container exists
if ! docker ps -q -f "name=^${CONTAINER}$" | grep -q .; then
    echo -e "${RED}Error: Container '${CONTAINER}' not found or not running${NC}"
    echo ""
    echo "Available containers:"
    docker ps --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
    exit 1
fi

# Check if web-net exists
if ! docker network inspect web-net >/dev/null 2>&1; then
    echo -e "${YELLOW}Network 'web-net' does not exist. Creating it...${NC}"
    docker network create --driver overlay --attachable web-net
    echo -e "${GREEN}✓ Network 'web-net' created${NC}"
else
    # Check if web-net is attachable
    ATTACHABLE=$(docker network inspect web-net --format '{{.Attachable}}')
    if [ "$ATTACHABLE" != "true" ]; then
        echo -e "${RED}Error: Network 'web-net' exists but is not attachable${NC}"
        echo ""
        echo "To fix this, you need to recreate the network:"
        echo "  1. Remove all services from web-net"
        echo "  2. docker network rm web-net"
        echo "  3. docker network create --driver overlay --attachable web-net"
        echo ""
        echo "Or run: docker network create --driver overlay --attachable web-net (if it doesn't exist yet)"
        exit 1
    fi
fi

# Check if already connected
if docker inspect ${CONTAINER} | grep -q '"web-net"'; then
    echo -e "${YELLOW}Container '${CONTAINER}' is already connected to web-net${NC}"
    echo ""
    echo "Current networks:"
    docker inspect ${CONTAINER} --format '{{range $k, $v := .NetworkSettings.Networks}}{{$k}} {{end}}'
    exit 0
fi

# Connect to web-net
echo -e "${YELLOW}Connecting '${CONTAINER}' to web-net...${NC}"
docker network connect web-net ${CONTAINER}

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Successfully connected '${CONTAINER}' to web-net${NC}"
    echo ""
    echo "Container networks:"
    docker inspect ${CONTAINER} --format '{{range $k, $v := .NetworkSettings.Networks}}  - {{$k}}: {{$v.IPAddress}}{{println}}{{end}}'
    echo ""
    echo "The container can now be reached by nginx using hostname: ${CONTAINER}"
else
    echo -e "${RED}✗ Failed to connect container to web-net${NC}"
    exit 1
fi
