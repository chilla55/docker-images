#!/bin/bash
# ============================================================================
# Secrets Setup Script for Docker Swarm
# ============================================================================
# Creates all required secrets for the service stack
# Run this on the Swarm manager node (mail)
# ============================================================================

set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo "============================================================================"
echo "Creating Docker Swarm Secrets"
echo "============================================================================"

# Function to create secret from stdin
create_secret() {
    local secret_name=$1
    local secret_value=$2
    
    if docker secret inspect "$secret_name" &>/dev/null; then
        echo -e "${YELLOW}Secret $secret_name already exists, skipping${NC}"
    else
        echo -n "$secret_value" | docker secret create "$secret_name" -
        echo -e "${GREEN}✓ Created secret: $secret_name${NC}"
    fi
}

# Function to create secret from file
create_secret_from_file() {
    local secret_name=$1
    local file_path=$2
    
    if docker secret inspect "$secret_name" &>/dev/null; then
        echo -e "${YELLOW}Secret $secret_name already exists, skipping${NC}"
    else
        if [ ! -f "$file_path" ]; then
            echo -e "${RED}✗ File not found: $file_path${NC}"
            return 1
        fi
        docker secret create "$secret_name" "$file_path"
        echo -e "${GREEN}✓ Created secret: $secret_name from $file_path${NC}"
    fi
}

echo ""
echo "Creating database secrets..."
# MariaDB/MySQL password - generate random for new deployment
echo -e "${GREEN}Generating random MariaDB root password...${NC}"
create_secret "mysql_root_password" "$(openssl rand -base64 32)"

create_secret "mysql_replication_password" "$(openssl rand -base64 32)"
create_secret "maxscale_password" "$(openssl rand -base64 32)"

# PostgreSQL
echo -e "${GREEN}Generating random PostgreSQL passwords...${NC}"
create_secret "postgres_password" "$(openssl rand -base64 32)"
create_secret "postgres_replication_password" "$(openssl rand -base64 32)"
create_secret "pgpool_admin_password" "$(openssl rand -base64 32)"

echo ""
echo "Creating Redis secrets..."
# Redis with no password (null in Pterodactyl config)
create_secret "redis_password" ""

echo ""
echo "Creating Pterodactyl secrets..."
# APP_KEY from user
echo -e "${YELLOW}Enter Pterodactyl APP_KEY (from your .env file):${NC}"
read -s ptero_app_key
create_secret "pterodactyl_app_key" "$ptero_app_key"

# Database password for ptero user - generate random
echo -e "${GREEN}Generating random Pterodactyl database password...${NC}"
create_secret "pterodactyl_db_password" "$(openssl rand -base64 32)"

# Redis has no password
create_secret "pterodactyl_redis_password" ""
# No SMTP password needed
create_secret "pterodactyl_mail_password" ""

echo ""
echo "Creating Vaultwarden secrets..."
echo ""
echo "Creating Vaultwarden secrets..."
echo -e "${GREEN}Generating random Vaultwarden passwords...${NC}"
create_secret "vaultwarden_admin_token" "$(openssl rand -base64 48)"
create_secret "vaultwarden_db_password" "$(openssl rand -base64 32)"
create_secret "vaultwarden_smtp_password" "your-smtp-password-here"
echo "Creating Storage Box secret..."
echo -e "${YELLOW}Please enter your Hetzner Storage Box password:${NC}"
read -s storagebox_pass
create_secret "storagebox_password" "$storagebox_pass"

echo ""
echo "Creating Cloudflare credentials file..."
echo -e "${YELLOW}Please provide Cloudflare API token (or press Enter to use example file):${NC}"
read -p "Cloudflare API Token: " cf_token

if [ -n "$cf_token" ]; then
    # Create temporary file with Cloudflare credentials
    TEMP_CF=$(mktemp)
    cat > "$TEMP_CF" <<EOF
# Cloudflare API token
dns_cloudflare_api_token = $cf_token
EOF
    create_secret_from_file "cloudflare_credentials" "$TEMP_CF"
    rm -f "$TEMP_CF"
else
    echo -e "${YELLOW}Skipping cloudflare_credentials - create manually from cloudflare.ini.example${NC}"
fi

echo ""
echo "============================================================================"
echo "Secret creation complete!"
echo "============================================================================"
echo ""
echo -e "${YELLOW}IMPORTANT: Save these commands to retrieve passwords later:${NC}"
echo ""
echo "# MariaDB root password:"
echo "  docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d"
echo ""
echo "# PostgreSQL password:"
echo "  docker secret inspect postgres_password -f '{{.Spec.Data}}' | base64 -d"
echo ""
echo "# Redis password:"
echo "  docker secret inspect redis_password -f '{{.Spec.Data}}' | base64 -d"
echo ""
echo "# Pterodactyl DB password:"
echo "  docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d"
echo ""
echo "# Vaultwarden admin token:"
echo "  docker secret inspect vaultwarden_admin_token -f '{{.Spec.Data}}' | base64 -d"
echo ""
echo "Verify secrets:"
echo "  docker secret ls"
