#!/bin/bash
# ============================================================================
# Node Labels Setup Script for Docker Swarm
# ============================================================================
# Sets up node labels for service placement
# Run this on the Swarm manager node (mail)
# 
# Node Configuration:
# - srv1: Primary DB (MariaDB Primary, PostgreSQL Primary) + Web
# - srv2: Orchestra + Certbot
# - mail: Secondary DB (MariaDB Secondary, PostgreSQL Secondary)
# ============================================================================

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "============================================================================"
echo "Setting Up Node Labels"
echo "============================================================================"

# Function to add label
add_label() {
    local node=$1
    local label=$2
    
    if docker node inspect "$node" --format '{{range $k,$v := .Spec.Labels}}{{$k}}={{$v}} {{end}}' 2>/dev/null | grep -q "$label"; then
        echo -e "${YELLOW}Node $node already has label $label, skipping${NC}"
    else
        docker node update --label-add "$label" "$node"
        echo -e "${GREEN}✓ Added label $label to node $node${NC}"
    fi
}

echo ""
echo "Setting up labels for srv1 (Primary DB + Web)..."
add_label "srv1" "mariadb.node=srv1"
add_label "srv1" "postgresql.node=srv1"
add_label "srv1" "web.node=web"
add_label "srv1" "redis.node=srv1"

echo ""
echo "Setting up labels for srv2 (Orchestra + Certbot)..."
add_label "srv2" "orchestra.node=srv2"
add_label "srv2" "certbot.node=srv2"

echo ""
echo "Setting up labels for mail (Secondary DB)..."
add_label "mail" "mariadb.node=mail"
add_label "mail" "postgresql.node=mail"

echo ""
echo "============================================================================"
echo "Node labels configured successfully!"
echo "============================================================================"
echo ""
echo "Verify labels:"
echo "  docker node inspect srv1 --format '{{.Spec.Labels}}'"
echo "  docker node inspect srv2 --format '{{.Spec.Labels}}'"
echo "  docker node inspect mail --format '{{.Spec.Labels}}'"
