#!/bin/bash
# ============================================================================
# Environment Validator for Pterodactyl Panel Deployment
# ============================================================================
# Validates Docker Swarm setup, networks, secrets, and configuration
# ============================================================================

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Counters
PASSED=0
FAILED=0
WARNINGS=0

# Helper functions
print_header() {
    echo -e "\n${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

check_pass() {
    echo -e "${GREEN}✓${NC} $1"
    PASSED=$((PASSED + 1))
}

check_fail() {
    echo -e "${RED}✗${NC} $1"
    FAILED=$((FAILED + 1))
}

check_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
    WARNINGS=$((WARNINGS + 1))
}

# ============================================================================
# Validation Checks
# ============================================================================

print_header "Pterodactyl Panel Deployment Validator"

# ──────────────────────────────────────────────────────────────────────────
# Docker & Swarm Checks
# ──────────────────────────────────────────────────────────────────────────
print_header "1. Docker & Swarm Configuration"

if command -v docker &> /dev/null; then
    DOCKER_VERSION=$(docker --version | awk '{print $3}' | sed 's/,//')
    check_pass "Docker installed: $DOCKER_VERSION"
else
    check_fail "Docker not installed"
fi

if docker info 2>/dev/null | grep -q "Swarm: active"; then
    check_pass "Docker Swarm is active"
    SWARM_MANAGERS=$(docker node ls --filter "role=manager" -q | wc -l)
    SWARM_WORKERS=$(docker node ls --filter "role=worker" -q | wc -l)
    echo -e "   Managers: $SWARM_MANAGERS, Workers: $SWARM_WORKERS"
else
    check_fail "Docker Swarm not initialized (run: docker swarm init)"
fi

# ──────────────────────────────────────────────────────────────────────────
# Network Checks
# ──────────────────────────────────────────────────────────────────────────
print_header "2. Docker Networks"

REQUIRED_NETWORKS=("proxy" "database" "cache")

for network in "${REQUIRED_NETWORKS[@]}"; do
    if docker network ls | grep -q "$network"; then
        check_pass "Network '$network' exists"
    else
        check_fail "Network '$network' missing (run: docker network create --driver overlay $network)"
    fi
done

# ──────────────────────────────────────────────────────────────────────────
# Secrets Checks
# ──────────────────────────────────────────────────────────────────────────
print_header "3. Docker Secrets"

REQUIRED_SECRETS=(
    "pterodactyl_app_key"
    "pterodactyl_db_password"
)

OPTIONAL_SECRETS=(
    "pterodactyl_redis_password"
    "pterodactyl_mail_password"
)

for secret in "${REQUIRED_SECRETS[@]}"; do
    if docker secret ls | grep -q "$secret"; then
        check_pass "Secret '$secret' exists"
    else
        check_fail "Secret '$secret' missing"
    fi
done

for secret in "${OPTIONAL_SECRETS[@]}"; do
    if docker secret ls | grep -q "$secret"; then
        check_pass "Secret '$secret' exists"
    else
        check_warn "Secret '$secret' not found (optional)"
    fi
done

# ──────────────────────────────────────────────────────────────────────────
# External Services
# ──────────────────────────────────────────────────────────────────────────
print_header "4. External Services"

# Check MariaDB
if docker service ls 2>/dev/null | grep -q "mariadb\|mysql"; then
    check_pass "MariaDB service found"
else
    check_warn "MariaDB service not found in Swarm"
fi

# Check Redis
if docker service ls 2>/dev/null | grep -q "redis"; then
    check_pass "Redis service found"
else
    check_warn "Redis service not found in Swarm"
fi

# Check Nginx
if docker service ls 2>/dev/null | grep -q "nginx"; then
    check_pass "Nginx service found"
else
    check_warn "Nginx service not found in Swarm"
fi

# ──────────────────────────────────────────────────────────────────────────
# File Checks
# ──────────────────────────────────────────────────────────────────────────
print_header "5. Required Files"

REQUIRED_FILES=(
    "Dockerfile"
    "Caddyfile"
    "supervisord.conf"
    "entrypoint.sh"
    "healthcheck.sh"
    "docker-compose.swarm.yml"
)

for file in "${REQUIRED_FILES[@]}"; do
    if [ -f "$file" ]; then
        check_pass "File '$file' exists"
    else
        check_fail "File '$file' missing"
    fi
done

# Check script permissions
for script in "entrypoint.sh" "healthcheck.sh"; do
    if [ -x "$script" ]; then
        check_pass "Script '$script' is executable"
    else
        check_warn "Script '$script' not executable (run: chmod +x $script)"
    fi
done

# ──────────────────────────────────────────────────────────────────────────
# Configuration Validation
# ──────────────────────────────────────────────────────────────────────────
print_header "6. Stack Configuration"

if [ -f "docker-compose.swarm.yml" ]; then
    # Check if APP_URL is set
    if grep -q "APP_URL: https://panel.example.com" docker-compose.swarm.yml; then
        check_warn "APP_URL still set to example.com (update in docker-compose.swarm.yml)"
    else
        check_pass "APP_URL configured"
    fi
    
    # Check if DB settings are configured
    if grep -q "DB_HOST: mariadb" docker-compose.swarm.yml; then
        check_pass "Database host configured"
    else
        check_warn "Database host may need configuration"
    fi
    
    # Check if migrations are disabled
    if grep -q 'RUN_MIGRATIONS_ON_START: "false"' docker-compose.swarm.yml; then
        check_pass "Auto-migrations disabled (recommended for production)"
    else
        check_warn "Auto-migrations enabled (only enable during upgrades)"
    fi
fi

# ──────────────────────────────────────────────────────────────────────────
# Port Availability
# ──────────────────────────────────────────────────────────────────────────
print_header "7. Port Availability"

REQUIRED_PORTS=(8080 9000)

for port in "${REQUIRED_PORTS[@]}"; do
    if ! ss -tulpn 2>/dev/null | grep -q ":$port " && ! netstat -tuln 2>/dev/null | grep -q ":$port "; then
        check_pass "Port $port available"
    else
        check_warn "Port $port may be in use"
    fi
done

# ──────────────────────────────────────────────────────────────────────────
# Summary
# ──────────────────────────────────────────────────────────────────────────
print_header "Validation Summary"

echo -e "${GREEN}Passed:   $PASSED${NC}"
echo -e "${YELLOW}Warnings: $WARNINGS${NC}"
echo -e "${RED}Failed:   $FAILED${NC}"

echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}✓ Validation passed! Ready to deploy.${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo -e "Next steps:"
    echo -e "  1. Review warnings above (if any)"
    echo -e "  2. Update configuration in ${BLUE}docker-compose.swarm.yml${NC}"
    echo -e "  3. Deploy: ${BLUE}docker stack deploy -c docker-compose.swarm.yml pterodactyl${NC}"
    echo -e "  4. Monitor: ${BLUE}docker service logs -f pterodactyl_panel${NC}"
    exit 0
else
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${RED}✗ Validation failed. Please fix errors above.${NC}"
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    exit 1
fi
