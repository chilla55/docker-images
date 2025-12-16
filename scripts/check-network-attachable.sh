#!/bin/bash

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "=== Checking Docker Networks ==="
echo ""

for network in web-net mariadb-net postgres-net redis-net; do
    if docker network inspect $network >/dev/null 2>&1; then
        ATTACHABLE=$(docker network inspect $network --format '{{.Attachable}}')
        DRIVER=$(docker network inspect $network --format '{{.Driver}}')
        SUBNET=$(docker network inspect $network --format '{{range .IPAM.Config}}{{.Subnet}}{{end}}')
        
        echo -e "Network: ${YELLOW}$network${NC}"
        echo "  Driver: $DRIVER"
        echo "  Subnet: $SUBNET"
        
        if [ "$ATTACHABLE" == "true" ]; then
            echo -e "  Attachable: ${GREEN}✓ YES${NC}"
        else
            echo -e "  Attachable: ${RED}✗ NO${NC} (non-swarm containers cannot connect)"
        fi
        echo ""
    else
        echo -e "Network: ${RED}$network (not found)${NC}"
        echo ""
    fi
done
