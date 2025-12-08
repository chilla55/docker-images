# Quick Start Guide - Zero Downtime Migration

Execute these commands on your Swarm manager node (mail):

## 1. Setup Phase (Run Once)

```bash
# SSH to mail server
ssh root@mail
cd /serverdata/docker

# Make scripts executable
chmod +x scripts/*.sh

# Setup node labels
./scripts/02-setup-node-labels.sh

# Create networks
./scripts/01-setup-networks.sh

# Create secrets (will prompt for passwords)
./scripts/03-setup-secrets.sh
# You will be prompted for:
# - Pterodactyl APP_KEY (from your .env: base64:vkZQbBpCCSLR0xnQtzFccvEddBpghFWOJS6qNCIfBDo=)
# - Storage Box password
# - Cloudflare API token (optional)
# All database passwords will be randomly generated

# Create nginx config
./scripts/04-setup-nginx-config.sh

# Verify everything is ready
./scripts/00-check-prerequisites.sh
```

## 2. Deploy Services (In Order)

```bash
# Deploy MariaDB (Primary: srv1, Secondary: mail)
cd /serverdata/docker/mariadb
cat > .env <<'EOF'
VERSION=latest
MYSQL_DATABASE=panel
MYSQL_USER=ptero
EOF
# Add passwords from secrets
echo "MYSQL_ROOT_PASSWORD=$(docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d)" >> .env
echo "MYSQL_PASSWORD=$(docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d)" >> .env
echo "REPLICATION_PASSWORD=$(docker secret inspect mysql_replication_password -f '{{.Spec.Data}}' | base64 -d)" >> .env
echo "MAXSCALE_USER=admin" >> .env
echo "MAXSCALE_PASSWORD=$(docker secret inspect maxscale_password -f '{{.Spec.Data}}' | base64 -d)" >> .env
docker stack deploy -c docker-compose.swarm.yml mariadb

# Wait for MariaDB to be healthy (watch until 3/3)
watch -n 2 'docker service ls | grep mariadb'

# Deploy PostgreSQL (Primary: srv1, Secondary: mail)
cd /serverdata/docker/postgresql
cat > .env <<'EOF'
VERSION=latest
POSTGRES_DB=app_db
POSTGRES_USER=postgres
REPLICATION_USER=replicator
PGPOOL_ADMIN_USER=admin
EOF
echo "POSTGRES_PASSWORD=$(docker secret inspect postgres_password -f '{{.Spec.Data}}' | base64 -d)" >> .env
echo "REPLICATION_PASSWORD=$(docker secret inspect postgres_replication_password -f '{{.Spec.Data}}' | base64 -d)" >> .env
echo "PGPOOL_ADMIN_PASSWORD=$(docker secret inspect pgpool_admin_password -f '{{.Spec.Data}}' | base64 -d)" >> .env
docker stack deploy -c docker-compose.swarm.yml postgresql

# Wait for PostgreSQL to be healthy
# Wait for PostgreSQL to be healthy
watch -n 2 'docker service ls | grep postgresql'

# ============================================================================
# MIGRATION STEP: Migrate data from old MariaDB container (if applicable)
# ============================================================================
# If you have existing Pterodactyl/Vaultwarden data in an old container:
./scripts/05-migrate-data.sh
docker stack deploy -c docker-compose.swarm.yml redis

# ============================================================================
# DATABASE SETUP: Create databases and users (if NOT using migration script)
# ============================================================================
# If you used the migration script (05-migrate-data.sh), SKIP this section.
# If starting fresh, run these commands:

# Create Pterodactyl databaseh new random passwords
# - Create users with new passwords stored in Docker secrets
# 
# If starting fresh, skip this step and continue below
# ============================================================================

# Deploy Redis (on srv1)dis
cat > .env <<'EOF'
VERSION=latest
REDIS_PASSWORD=
EOF
docker stack deploy -c docker-compose.swarm.yml redis

# Create Pterodactyl database
MYSQL_ROOT_PASS=$(docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d)
PTERO_DB_PASS=$(docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d)
docker exec -it $(docker ps -q -f name=mariadb_maxscale) mysql -h mariadb-primary -uroot -p"$MYSQL_ROOT_PASS" <<EOF
CREATE DATABASE IF NOT EXISTS panel;
CREATE USER IF NOT EXISTS 'ptero'@'%' IDENTIFIED BY '$PTERO_DB_PASS';
GRANT ALL PRIVILEGES ON panel.* TO 'ptero'@'%';
FLUSH PRIVILEGES;
EOF

# Deploy Pterodactyl
cd /serverdata/docker/petrodactyl
docker stack deploy -c docker-compose.swarm.yml pterodactyl

# Create Vaultwarden database
MYSQL_ROOT_PASS=$(docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d)
VW_DB_PASS=$(docker secret inspect vaultwarden_db_password -f '{{.Spec.Data}}' | base64 -d)
docker exec -it $(docker ps -q -f name=mariadb_maxscale) mysql -h mariadb-primary -uroot -p"$MYSQL_ROOT_PASS" <<EOF
CREATE DATABASE IF NOT EXISTS vaultwarden;
CREATE USER IF NOT EXISTS 'vaultwarden'@'%' IDENTIFIED BY '$VW_DB_PASS';
GRANT ALL PRIVILEGES ON vaultwarden.* TO 'vaultwarden'@'%';
FLUSH PRIVILEGES;
EOF

# Deploy Vaultwarden (Update VAULTWARDEN_DOMAIN first!)
cd /serverdata/docker/vaultwarden
# Edit docker-compose.swarm.yml and set VAULTWARDEN_DOMAIN
docker stack deploy -c docker-compose.swarm.yml vaultwarden

# Deploy Certbot (on srv2, update domains first!)
cd /serverdata/docker/certbot
# Edit docker-compose.swarm.yml and set CERT_EMAIL, CERT_DOMAINS, STORAGE_BOX_*
docker stack deploy -c docker-compose.swarm.yml certbot

# Wait for certificates to be generated
docker service logs -f certbot_certbot

# Deploy Nginx (on srv1)
cd /serverdata/docker/nginx
# Edit docker-compose.swarm.yml and set CERT_WATCH_PATH, STORAGE_BOX_*
docker stack deploy -c docker-compose.swarm.yml nginx
```

## 3. Verify Deployment

```bash
# Check all services are running
docker service ls

# Verify databases
docker exec $(docker ps -q -f name=mariadb_mariadb-secondary) mysql -uroot -p"$(docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d)" -e "SHOW SLAVE STATUS\G" | grep Seconds_Behind_Master

# Test Redis
docker exec $(docker ps -q -f name=redis_redis) redis-cli ping

# Check application logs
docker service logs pterodactyl_panel --tail 50
docker service logs vaultwarden_vaultwarden --tail 50
docker service logs nginx_nginx --tail 50
```

## 4. Rollback (If Needed)

```bash
# Remove specific stack
docker stack rm <stack-name>

# Remove all stacks (reverse order)
docker stack rm nginx certbot vaultwarden pterodactyl redis postgresql mariadb
```

## Important Notes

- All passwords are stored securely in Docker secrets
- **Node Layout**:
  - srv1: MariaDB Primary, PostgreSQL Primary, Redis, Nginx
  - srv2: Certbot, Orchestra
  - mail: MariaDB Secondary, PostgreSQL Secondary
- All services use secrets for sensitive data
- Update domain names and Storage Box credentials before deploying
- Certificate generation may take 2-3 minutes on first run
- All nodes are managers in your cluster

## Retrieve Passwords

```bash
# MariaDB root
docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d

# PostgreSQL
docker secret inspect postgres_password -f '{{.Spec.Data}}' | base64 -d

# Redis (empty - no password)
docker secret inspect redis_password -f '{{.Spec.Data}}' | base64 -d

# Pterodactyl DB
docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d

# Pterodactyl APP_KEY
docker secret inspect pterodactyl_app_key -f '{{.Spec.Data}}' | base64 -d

# Vaultwarden Admin
docker secret inspect vaultwarden_admin_token -f '{{.Spec.Data}}' | base64 -d
```

## Configuration Values

These values are already configured in the docker-compose.swarm.yml files:

**Pterodactyl:**
- APP_URL: `https://gpanel.chilla55.de`
- DB_HOST: `maxscale` (port 4006 for read-write splitting)
- DB_USERNAME: `ptero`
- DB_DATABASE: `panel`
- REDIS_HOST: `redis` (no password)
- MAIL_FROM: `no-reply@chilla55.de`
- HASHIDS_SALT: `aMxyVI3NeaVOCVajWfyz`

For detailed instructions, see `ZERO_DOWNTIME_MIGRATION.md`
