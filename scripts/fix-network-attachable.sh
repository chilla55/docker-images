#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

if [ -z "$1" ]; then
    echo -e "${RED}Error: Network name required${NC}"
    echo ""
    echo "Usage: $0 <network-name> [subnet]"
    echo ""
    echo "Example:"
    echo "  $0 web-net"
    echo "  $0 web-net 10.0.1.0/24"
    echo ""
    exit 1
fi

NETWORK=$1
SUBNET=$2

# Check if network exists
if ! docker network inspect $NETWORK >/dev/null 2>&1; then
    echo -e "${RED}Network '$NETWORK' does not exist${NC}"
    exit 1
fi

# Check if attachable
ATTACHABLE=$(docker network inspect $NETWORK --format '{{.Attachable}}')
if [ "$ATTACHABLE" == "true" ]; then
    echo -e "${GREEN}Network '$NETWORK' is already attachable${NC}"
    exit 0
fi

echo -e "${YELLOW}Network '$NETWORK' is NOT attachable${NC}"
echo ""
echo "To make it attachable, we need to recreate it."
echo "This will disconnect all services temporarily."
echo ""

# Get current subnet
CURRENT_SUBNET=$(docker network inspect $NETWORK --format '{{range .IPAM.Config}}{{.Subnet}}{{end}}')
echo "Current subnet: $CURRENT_SUBNET"

if [ -z "$SUBNET" ]; then
    SUBNET=$CURRENT_SUBNET
fi

echo "New subnet: $SUBNET"
echo ""

read -p "Continue? (yes/no): " CONFIRM
if [ "$CONFIRM" != "yes" ]; then
    echo "Aborted."
    exit 0
fi

echo ""
echo -e "${YELLOW}Removing network '$NETWORK'...${NC}"
docker network rm $NETWORK

echo -e "${YELLOW}Creating network '$NETWORK' with --attachable flag...${NC}"
if [ -n "$SUBNET" ]; then
    docker network create \
        --driver overlay \
        --attachable \
        --subnet="$SUBNET" \
        $NETWORK
else
    docker network create \
        --driver overlay \
        --attachable \
        $NETWORK
fi

echo -e "${GREEN}✓ Network '$NETWORK' recreated with attachable flag${NC}"
echo ""
echo "Verification:"
docker network inspect $NETWORK --format 'Attachable: {{.Attachable}}, Subnet: {{range .IPAM.Config}}{{.Subnet}}{{end}}'
echo ""
echo -e "${YELLOW}Remember to redeploy services that use this network!${NC}"
