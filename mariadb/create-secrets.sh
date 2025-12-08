#!/bin/bash
set -e

echo "=== MariaDB Secrets Generator ==="
echo "This script will generate random passwords and create Docker secrets."
echo "If secrets already exist, they will be replaced."
echo ""

# Function to create or replace a secret
create_or_replace_secret() {
    local secret_name=$1
    local secret_value=$2
    
    # Check if secret exists
    if docker secret inspect "$secret_name" >/dev/null 2>&1; then
        echo "Secret '$secret_name' exists. Removing..."
        docker secret rm "$secret_name"
    fi
    
    # Create the secret
    echo "$secret_value" | docker secret create "$secret_name" -
    echo "✓ Created secret: $secret_name"
}

# Generate random passwords (32 characters, base64)
echo "Generating random passwords..."
ROOT_PASS=$(openssl rand -base64 32 | tr -d '\n')
USER_PASS=$(openssl rand -base64 32 | tr -d '\n')
REPL_PASS=$(openssl rand -base64 32 | tr -d '\n')
MAXSCALE_PASS=$(openssl rand -base64 32 | tr -d '\n')

echo ""
echo "Creating Docker secrets..."

# Create or replace secrets
create_or_replace_secret "mysql_root_password" "$ROOT_PASS"
create_or_replace_secret "mysql_user_password" "$USER_PASS"
create_or_replace_secret "replication_password" "$REPL_PASS"
create_or_replace_secret "maxscale_password" "$MAXSCALE_PASS"

echo ""
echo "=== Secrets Created Successfully ==="
echo ""
echo "IMPORTANT: Save these passwords in a secure location!"
echo "You will need them to access your database."
echo ""
echo "MYSQL_ROOT_PASSWORD: $ROOT_PASS"
echo "MYSQL_USER_PASSWORD: $USER_PASS"
echo "REPLICATION_PASSWORD: $REPL_PASS"
echo "MAXSCALE_PASSWORD: $MAXSCALE_PASS"
echo ""
echo "You can now deploy your stack with:"
echo "  docker stack deploy -c docker-compose.swarm.yml mariadb"
echo ""
