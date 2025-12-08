# Zero-Downtime Migration Guide for Docker Swarm

## Overview

This guide will help you migrate all services to Docker Swarm with zero downtime across your 3-node cluster:

- **srv1**: Primary databases (MariaDB, PostgreSQL) + Web (Nginx) + Redis
- **srv2**: Orchestra/Management + Certbot
- **mail**: Secondary databases (MariaDB, PostgreSQL) - Currently Swarm Leader in Drain mode

## Prerequisites

- Docker Swarm initialized across all nodes
- All nodes can communicate on overlay network
- Images built and pushed to registry (ghcr.io/chilla55/*)
- Root or sudo access on manager node (mail)

## Migration Timeline

**Total estimated time**: 45-60 minutes  
**Expected downtime**: 0 seconds (rolling updates)

---

## Phase 1: Pre-Deployment Setup (15 minutes)

### Step 1.1: Connect to Swarm Manager

```bash
# SSH to the mail server (Swarm manager)
ssh root@mail

# Navigate to working directory
cd /serverdata/docker
```

### Step 1.2: Run Prerequisites Check

```bash
# Make scripts executable
chmod +x scripts/*.sh

# Run prerequisites check
./scripts/00-check-prerequisites.sh
```

**Expected output**: Should show warnings about missing networks, secrets, and configs.

### Step 1.3: Setup Node Labels

```bash
# Configure node labels for service placement
./scripts/02-setup-node-labels.sh
```

**Verify labels**:
```bash
docker node inspect srv1 --format '{{.Spec.Labels}}'
docker node inspect srv2 --format '{{.Spec.Labels}}'
docker node inspect mail --format '{{.Spec.Labels}}'
```

**Expected output**:
- srv1: `mariadb.node:srv1 postgresql.node:srv1 redis.node:srv1 web.node:web`
- srv2: `certbot.node:srv2 orchestra.node:srv2`
- mail: `mariadb.node:mail postgresql.node:mail`

### Step 1.4: Create Overlay Networks

```bash
# Create all required networks
./scripts/01-setup-networks.sh
```

**Verify networks**:
```bash
docker network ls --filter driver=overlay
```

**Expected networks**: `web-net`, `mariadb-net`, `postgres-net`, `redis-net`

### Step 1.5: Create Docker Secrets

```bash
# Create all secrets (will prompt for Storage Box password and Cloudflare token)
./scripts/03-setup-secrets.sh
```

**Important**: When prompted, enter:
- **Storage Box password**: Your Hetzner Storage Box password
- **Cloudflare API token**: Your Cloudflare API token for DNS challenges

**Save these commands for later reference**:
```bash
# View MariaDB password (123lol789)
docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d

# View generated passwords
docker secret inspect postgres_password -f '{{.Spec.Data}}' | base64 -d
docker secret inspect redis_password -f '{{.Spec.Data}}' | base64 -d
docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d
docker secret inspect vaultwarden_admin_token -f '{{.Spec.Data}}' | base64 -d
```

### Step 1.6: Create Nginx Config

```bash
# Create Docker config for nginx
./scripts/04-setup-nginx-config.sh
```

**Verify config**:
```bash
docker config ls
```

### Step 1.7: Final Prerequisites Check

```bash
# Re-run prerequisites check - should pass now
./scripts/00-check-prerequisites.sh
```

**Expected output**: "All checks passed! System is ready for deployment"

---

## Phase 2: Database Layer Deployment (10 minutes)

### Step 2.1: Deploy MariaDB Stack (Primary + Secondary + MaxScale)

```bash
cd /serverdata/docker/mariadb

# Create environment file
cat > .env <<EOF
VERSION=latest
MYSQL_ROOT_PASSWORD=123lol789
MYSQL_DATABASE=panel
MYSQL_USER=paneluser
MYSQL_PASSWORD=$(docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d)
REPLICATION_PASSWORD=$(docker secret inspect mysql_replication_password -f '{{.Spec.Data}}' | base64 -d)
MAXSCALE_USER=admin
MAXSCALE_PASSWORD=$(docker secret inspect maxscale_password -f '{{.Spec.Data}}' | base64 -d)
EOF

# Deploy MariaDB stack
docker stack deploy -c docker-compose.swarm.yml mariadb
```

**Monitor deployment**:
```bash
# Watch services come up
watch -n 2 'docker service ls | grep mariadb'

# Check logs
docker service logs -f mariadb_mariadb-primary
docker service logs -f mariadb_mariadb-secondary
docker service logs -f mariadb_maxscale
```

**Wait for**: All services showing 1/1 replicas (about 3-5 minutes)

**Verify replication**:
```bash
# Check primary status
docker exec $(docker ps -q -f name=mariadb_mariadb-primary) mysql -uroot -p123lol789 -e "SHOW MASTER STATUS\G"

# Check secondary status
docker exec $(docker ps -q -f name=mariadb_mariadb-secondary) mysql -uroot -p123lol789 -e "SHOW SLAVE STATUS\G"
```

### Step 2.2: Deploy PostgreSQL Stack (Primary + Secondary + PgPool)

```bash
cd /serverdata/docker/postgresql

# Create environment file
cat > .env <<EOF
VERSION=latest
POSTGRES_PASSWORD=$(docker secret inspect postgres_password -f '{{.Spec.Data}}' | base64 -d)
POSTGRES_DB=app_db
POSTGRES_USER=postgres
REPLICATION_USER=replicator
REPLICATION_PASSWORD=$(docker secret inspect postgres_replication_password -f '{{.Spec.Data}}' | base64 -d)
PGPOOL_ADMIN_USER=admin
PGPOOL_ADMIN_PASSWORD=$(docker secret inspect pgpool_admin_password -f '{{.Spec.Data}}' | base64 -d)
EOF

# Deploy PostgreSQL stack
docker stack deploy -c docker-compose.swarm.yml postgresql
```

**Monitor deployment**:
```bash
watch -n 2 'docker service ls | grep postgresql'
```

**Wait for**: All services showing 1/1 replicas

---

## Phase 3: Cache Layer Deployment (5 minutes)

### Step 3.1: Deploy Redis

```bash
cd /serverdata/docker/redis

# Create environment file
cat > .env <<EOF
VERSION=latest
REDIS_PASSWORD=$(docker secret inspect redis_password -f '{{.Spec.Data}}' | base64 -d)
EOF

# Deploy Redis stack
docker stack deploy -c docker-compose.swarm.yml redis
```

**Monitor deployment**:
```bash
docker service logs -f redis_redis
```

**Verify Redis**:
```bash
# Test connection
docker exec $(docker ps -q -f name=redis_redis) redis-cli -a "$(docker secret inspect redis_password -f '{{.Spec.Data}}' | base64 -d)" ping
```

**Expected output**: `PONG`

---

## Phase 4: Application Layer Deployment (10 minutes)

### Step 4.1: Create Database for Pterodactyl

```bash
# Connect to MariaDB via MaxScale and create database
docker exec -it $(docker ps -q -f name=mariadb_maxscale) mysql -h mariadb-primary -uroot -p123lol789 <<EOF
CREATE DATABASE IF NOT EXISTS panel;
CREATE USER IF NOT EXISTS 'paneluser'@'%' IDENTIFIED BY '$(docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d)';
GRANT ALL PRIVILEGES ON panel.* TO 'paneluser'@'%';
FLUSH PRIVILEGES;
EOF
```

### Step 4.2: Deploy Pterodactyl Panel

```bash
cd /serverdata/docker/petrodactyl

# Update environment variables in docker-compose.swarm.yml or create .env file
# Make sure to update:
# - APP_URL to your actual domain
# - DB_HOST should be "maxscale" (connects to port 4006 for read-write splitting)
# - REDIS_HOST should be "redis"

# Deploy Pterodactyl stack
docker stack deploy -c docker-compose.swarm.yml pterodactyl
```

**Monitor deployment**:
```bash
docker service logs -f pterodactyl_panel
```

**Wait for**: Service to initialize and run migrations

### Step 4.3: Create Database for Vaultwarden

```bash
# Create Vaultwarden database
docker exec -it $(docker ps -q -f name=mariadb_maxscale) mysql -h mariadb-primary -uroot -p123lol789 <<EOF
CREATE DATABASE IF NOT EXISTS vaultwarden;
CREATE USER IF NOT EXISTS 'vaultwarden'@'%' IDENTIFIED BY '$(docker secret inspect vaultwarden_db_password -f '{{.Spec.Data}}' | base64 -d)';
GRANT ALL PRIVILEGES ON vaultwarden.* TO 'vaultwarden'@'%';
FLUSH PRIVILEGES;
EOF
```

### Step 4.4: Deploy Vaultwarden

```bash
cd /serverdata/docker/vaultwarden

# Update environment variables in docker-compose.swarm.yml
# Make sure to update:
# - VAULTWARDEN_DOMAIN
# - DB_HOST should be "maxscale"
# - SMTP settings

# Deploy Vaultwarden stack
docker stack deploy -c docker-compose.swarm.yml vaultwarden
```

**Monitor deployment**:
```bash
docker service logs -f vaultwarden_vaultwarden
```

---

## Phase 5: Infrastructure Layer Deployment (10 minutes)

### Step 5.1: Deploy Certbot (SSL Certificate Management)

```bash
cd /serverdata/docker/certbot

# Update environment variables in docker-compose.swarm.yml:
# - CERT_EMAIL
# - CERT_DOMAINS
# - STORAGE_BOX_HOST
# - STORAGE_BOX_USER

# Deploy Certbot stack
docker stack deploy -c docker-compose.swarm.yml certbot
```

**Monitor deployment**:
```bash
docker service logs -f certbot_certbot
```

**Wait for**: Initial certificate generation (may take 2-3 minutes)

### Step 5.2: Deploy Nginx (Web Server/Reverse Proxy)

```bash
cd /serverdata/docker/nginx

# Update environment variables in docker-compose.swarm.yml:
# - CERT_WATCH_PATH (set to your actual domain)
# - STORAGE_BOX_HOST
# - STORAGE_BOX_USER
# - Configure your site configurations on Storage Box

# Deploy Nginx stack
docker stack deploy -c docker-compose.swarm.yml nginx
```

**Monitor deployment**:
```bash
docker service logs -f nginx_nginx
```

**Verify Nginx**:
```bash
# Check if Nginx is listening
curl -I http://localhost

# Check SSL (if certificates are ready)
curl -I https://your-domain.com
```

---

## Phase 6: Verification & Health Checks (5 minutes)

### Step 6.1: Check All Services

```bash
# List all services
docker service ls

# Check service health
for service in $(docker service ls --format '{{.Name}}'); do
    echo "=== $service ==="
    docker service ps $service --no-trunc
done
```

**Expected**: All services showing 1/1 (or configured replicas) in Running state

### Step 6.2: Verify Database Replication

```bash
# MariaDB replication lag
docker exec $(docker ps -q -f name=mariadb_mariadb-secondary) mysql -uroot -p123lol789 -e "SHOW SLAVE STATUS\G" | grep Seconds_Behind_Master

# PostgreSQL replication lag
docker exec $(docker ps -q -f name=postgresql_postgresql-secondary) psql -U postgres -c "SELECT EXTRACT(EPOCH FROM (now() - pg_last_xact_replay_timestamp())) AS replication_lag_seconds;"
```

**Expected**: Replication lag < 1 second

### Step 6.3: Test Application Connectivity

```bash
# Test Pterodactyl database connection
docker exec $(docker ps -q -f name=pterodactyl_panel) php artisan migrate:status

# Test Vaultwarden database connection
docker service logs vaultwarden_vaultwarden | grep -i "database"

# Test Redis
docker exec $(docker ps -q -f name=pterodactyl_panel) php artisan cache:clear
```

### Step 6.4: Verify Network Connectivity

```bash
# Test overlay network connectivity
docker run --rm --network mariadb-net alpine ping -c 3 mariadb-primary
docker run --rm --network mariadb-net alpine ping -c 3 maxscale
docker run --rm --network redis-net alpine ping -c 3 redis
```

---

## Phase 7: Post-Deployment Configuration

### Step 7.1: Activate mail Node (Optional)

Currently, the `mail` node is in Drain mode. Once all services are running and stable, you can activate it:

```bash
# Make mail node Active (only if you want to run services on it)
docker node update --availability active mail
```

**Note**: Since `mail` is labeled for secondary databases only, it won't receive other services unless you update placement constraints.

### Step 7.2: Configure Auto-Renewal for Certificates

Certbot will automatically run certificate renewals based on `RENEW_INTERVAL` (default: every 12 hours). Verify it's working:

```bash
# Check Certbot logs
docker service logs certbot_certbot | grep -i renew
```

### Step 7.3: Setup Monitoring (Optional)

```bash
# Monitor service status
watch -n 5 'docker service ls'

# Monitor node resources
docker node ls
docker node inspect srv1 --format '{{.Status}}'
```

---

## Rollback Procedures

### Rolling Back a Single Service

```bash
# Rollback to previous version
docker service rollback <stack-name>_<service-name>

# Examples:
docker service rollback pterodactyl_panel
docker service rollback nginx_nginx
```

### Removing a Complete Stack

```bash
# Remove entire stack
docker stack rm <stack-name>

# Examples:
docker stack rm pterodactyl
docker stack rm vaultwarden
docker stack rm nginx
```

### Emergency: Full Rollback

```bash
# Remove all stacks (in reverse order)
docker stack rm nginx
docker stack rm certbot
docker stack rm vaultwarden
docker stack rm pterodactyl
docker stack rm redis
docker stack rm postgresql
docker stack rm mariadb

# Remove networks
docker network rm web-net mariadb-net postgres-net redis-net

# Secrets and configs are preserved for re-deployment
```

---

## Troubleshooting

### Service Won't Start

```bash
# Check service logs
docker service logs <service-name> --tail 100

# Check service tasks (including failed ones)
docker service ps <service-name> --no-trunc

# Inspect service configuration
docker service inspect <service-name> --pretty
```

### Database Connection Issues

```bash
# Verify database is accessible
docker exec $(docker ps -q -f name=mariadb_maxscale) mysql -h mariadb-primary -uroot -p123lol789 -e "SELECT 1;"

# Check MaxScale routing
docker exec $(docker ps -q -f name=mariadb_maxscale) maxctrl list servers
```

### Network Issues

```bash
# List networks
docker network ls

# Inspect network
docker network inspect <network-name>

# Test connectivity from a service
docker exec $(docker ps -q -f name=<service>) ping <target-service>
```

### Secret/Config Not Found

```bash
# List secrets
docker secret ls

# Recreate missing secret
echo "your-secret-value" | docker secret create secret-name -

# List configs
docker config ls

# Note: Configs are immutable, create new version if needed
docker config create nginx_conf_v2 /path/to/nginx.conf
```

---

## Important Commands Reference

### Service Management
```bash
# List all services
docker service ls

# Scale a service
docker service scale <service-name>=<replicas>

# Update a service
docker service update <service-name>

# Remove a service
docker service rm <service-name>
```

### Stack Management
```bash
# Deploy stack
docker stack deploy -c docker-compose.swarm.yml <stack-name>

# List stacks
docker stack ls

# List stack services
docker stack services <stack-name>

# Remove stack
docker stack rm <stack-name>
```

### Logs and Debugging
```bash
# Follow service logs
docker service logs -f <service-name>

# Last 100 lines
docker service logs --tail 100 <service-name>

# With timestamps
docker service logs -t <service-name>
```

### Node Management
```bash
# List nodes
docker node ls

# Inspect node
docker node inspect <node-name>

# Update node availability
docker node update --availability <active|pause|drain> <node-name>

# Add/remove labels
docker node update --label-add key=value <node-name>
docker node update --label-rm key <node-name>
```

---

## Success Criteria

✅ All services show 1/1 (or configured) replicas  
✅ Database replication lag < 1 second  
✅ Applications can connect to databases via MaxScale/PgPool  
✅ Redis is accessible and responding to PING  
✅ Nginx is serving HTTP/HTTPS traffic  
✅ Certbot has generated valid SSL certificates  
✅ All healthchecks are passing  
✅ No error messages in service logs  
✅ Overlay networks are functioning  
✅ Services can resolve each other by name  

---

## Next Steps

1. Configure your site configurations on Hetzner Storage Box (`/sites` directory)
2. Set up your domain DNS to point to your servers
3. Configure Cloudflare (if using) for DDoS protection
4. Set up monitoring and alerting (Prometheus/Grafana)
5. Configure backups for databases and persistent volumes
6. Review and adjust resource limits based on actual usage
7. Document your custom configurations and environment variables

---

## Support

For issues or questions:
1. Check service logs: `docker service logs <service-name>`
2. Review the ARCHITECTURE.md and README.md files in each service directory
3. Verify prerequisite check passes: `./scripts/00-check-prerequisites.sh`
4. Check Docker Swarm documentation: https://docs.docker.com/engine/swarm/

**MariaDB Root Password**: 123lol789 (as specified)
