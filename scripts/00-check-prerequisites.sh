#!/bin/bash
# ============================================================================
# Prerequisites Check Script for Zero-Downtime Migration
# ============================================================================
# This script verifies that all prerequisites are met before deployment
# Run this on the Swarm manager node (mail)
# ============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "============================================================================"
echo "Checking Docker Swarm Prerequisites"
echo "============================================================================"

ERRORS=0
WARNINGS=0

# Function to print status
print_status() {
    local status=$1
    local message=$2
    if [ "$status" == "OK" ]; then
        echo -e "${GREEN}✓${NC} $message"
    elif [ "$status" == "WARN" ]; then
        echo -e "${YELLOW}⚠${NC} $message"
        ((WARNINGS++))
    else
        echo -e "${RED}✗${NC} $message"
        ((ERRORS++))
    fi
}

# Check if running on swarm manager
echo ""
echo "Checking Swarm Status..."
if ! docker node ls &>/dev/null; then
    print_status "ERROR" "Not running on a Swarm manager node"
    exit 1
else
    print_status "OK" "Running on Swarm manager"
fi

# Check nodes
echo ""
echo "Checking Nodes..."
NODES=$(docker node ls --format "{{.Hostname}}" 2>/dev/null | wc -l)
if [ "$NODES" -lt 3 ]; then
    print_status "WARN" "Expected 3 nodes, found $NODES"
else
    print_status "OK" "Found $NODES nodes"
fi

# Check specific node labels
echo ""
echo "Checking Node Labels..."
LABELS_TO_CHECK=(
    "srv1:mariadb.node=srv1"
    "mail:mariadb.node=mail"
    "srv1:postgresql.node=srv1"
    "mail:postgresql.node=mail"
    "srv1:web.node=web"
    "srv2:orchestra.node=srv2"
    "srv2:certbot.node=srv2"
)

for label_check in "${LABELS_TO_CHECK[@]}"; do
    IFS=':' read -r node label <<< "$label_check"
    if docker node inspect "$node" --format '{{range $k,$v := .Spec.Labels}}{{$k}}={{$v}} {{end}}' 2>/dev/null | grep -q "$label"; then
        print_status "OK" "Node $node has label $label"
    else
        print_status "ERROR" "Node $node missing label: $label"
    fi
done

# Check required networks
echo ""
echo "Checking Networks..."
REQUIRED_NETWORKS=("web-net" "mariadb-net" "postgres-net" "redis-net")
for network in "${REQUIRED_NETWORKS[@]}"; do
    if docker network inspect "$network" &>/dev/null; then
        print_status "OK" "Network $network exists"
    else
        print_status "WARN" "Network $network does not exist (will be created)"
    fi
done

# Check required secrets
echo ""
echo "Checking Secrets..."
REQUIRED_SECRETS=(
    "storagebox_password"
    "cloudflare_credentials"
    "pterodactyl_app_key"
    "pterodactyl_db_password"
    "pterodactyl_redis_password"
    "pterodactyl_mail_password"
    "vaultwarden_admin_token"
    "vaultwarden_db_password"
    "vaultwarden_smtp_password"
)

for secret in "${REQUIRED_SECRETS[@]}"; do
    if docker secret inspect "$secret" &>/dev/null; then
        print_status "OK" "Secret $secret exists"
    else
        print_status "WARN" "Secret $secret does not exist (needs to be created)"
    fi
done

# Check required configs
echo ""
echo "Checking Configs..."
if docker config inspect "nginx_conf" &>/dev/null; then
    print_status "OK" "Config nginx_conf exists"
else
    print_status "WARN" "Config nginx_conf does not exist (needs to be created)"
fi

# Check images availability
echo ""
echo "Checking Images..."
REQUIRED_IMAGES=(
    "ghcr.io/chilla55/mariadb-replication:latest"
    "ghcr.io/chilla55/nginx:latest"
    "ghcr.io/chilla55/pterodactyl-panel:latest"
    "ghcr.io/chilla55/postgresql-replication:latest"
    "ghcr.io/chilla55/redis:latest"
    "ghcr.io/chilla55/vaultwarden:latest"
    "ghcr.io/chilla55/certbot-storagebox:latest"
)

for image in "${REQUIRED_IMAGES[@]}"; do
    print_status "WARN" "Image $image - verify it exists in registry"
done

# Summary
echo ""
echo "============================================================================"
echo "Summary"
echo "============================================================================"
if [ $ERRORS -gt 0 ]; then
    echo -e "${RED}Found $ERRORS error(s)${NC}"
    echo "Please fix errors before proceeding with deployment"
    exit 1
elif [ $WARNINGS -gt 0 ]; then
    echo -e "${YELLOW}Found $WARNINGS warning(s)${NC}"
    echo "Review warnings and run setup scripts to create missing resources"
    exit 0
else
    echo -e "${GREEN}All checks passed!${NC}"
    echo "System is ready for deployment"
    exit 0
fi
