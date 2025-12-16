#!/bin/bash
set -e

echo "=== Setting up Vaultwarden Database and User ==="

# Get the vaultwarden DB password from Docker secret
VW_DB_PASS=$(docker secret inspect vaultwarden_db_password -f '{{.Spec.Data}}' | base64 -d)

if [ -z "$VW_DB_PASS" ]; then
    echo "ERROR: Could not retrieve vaultwarden_db_password secret"
    exit 1
fi

echo "Retrieved vaultwarden database password from secret"

# Get the MariaDB root password from Docker secret
MYSQL_ROOT_PASS=$(docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d)

if [ -z "$MYSQL_ROOT_PASS" ]; then
    echo "ERROR: Could not retrieve mysql_root_password secret"
    exit 1
fi

echo "Retrieved MySQL root password from secret"

# Find the MariaDB container
MARIADB_CONTAINER=$(docker ps -q -f name=mariadb_mariadb)

if [ -z "$MARIADB_CONTAINER" ]; then
    echo "ERROR: MariaDB container not found. Is the service running?"
    echo "Try: docker service ls | grep mariadb"
    exit 1
fi

echo "Found MariaDB container: $MARIADB_CONTAINER"

# Create database and user
echo "Creating vaultwarden database and user..."

docker exec -i "$MARIADB_CONTAINER" mysql -uroot -p"$MYSQL_ROOT_PASS" <<EOF
-- Create database if it doesn't exist
CREATE DATABASE IF NOT EXISTS vaultwarden CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

-- Create user if it doesn't exist
CREATE USER IF NOT EXISTS 'vaultwarden'@'%' IDENTIFIED BY '$VW_DB_PASS';

-- Grant privileges
GRANT ALL PRIVILEGES ON vaultwarden.* TO 'vaultwarden'@'%';

-- Flush privileges
FLUSH PRIVILEGES;

-- Show result
SELECT User, Host FROM mysql.user WHERE User = 'vaultwarden';
SHOW DATABASES LIKE 'vaultwarden';
EOF

if [ $? -eq 0 ]; then
    echo ""
    echo "✓ Vaultwarden database and user created successfully!"
    echo ""
    echo "Database: vaultwarden"
    echo "User: vaultwarden@%"
    echo "Password: (from vaultwarden_db_password secret)"
    echo ""
    echo "You can now start/restart the vaultwarden service:"
    echo "  docker service update --force vaultwarden_vaultwarden"
else
    echo "ERROR: Failed to create vaultwarden database and user"
    exit 1
fi
