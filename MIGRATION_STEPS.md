# Zero-Downtime Migration - Execution Guide

## ✅ Pre-Deployment Checklist

**You've already completed:**
- ✅ Node labels configured (srv1, srv2, mail)
- ✅ Overlay networks created (web-net, mariadb-net, postgres-net, redis-net)

**Now execute in this order:**

---

## Step 1: Create Docker Secrets

```bash
ssh root@mail
cd /serverdata/docker

# Create all secrets (will prompt for APP_KEY, Storage Box password, Cloudflare token)
./scripts/03-setup-secrets.sh
```

**You'll be prompted for:**
- Pterodactyl APP_KEY: `base64:vkZQbBpCCSLR0xnQtzFccvEddBpghFWOJS6qNCIfBDo=`
- Storage Box password: (your password)
- Cloudflare API token: (your token or skip)

All database passwords are **randomly generated**.

---

## Step 2: Create Nginx Config

```bash
./scripts/04-setup-nginx-config.sh
```

---

## Step 3: Verify Prerequisites

```bash
./scripts/00-check-prerequisites.sh
```

Should show all checks passed or minor warnings.

---

## Step 4: Deploy MariaDB Stack

```bash
cd /serverdata/docker/mariadb

# Create .env file with passwords from secrets
cat > .env <<'EOF'
VERSION=latest
MYSQL_DATABASE=panel
MYSQL_USER=ptero
EOF
echo "MYSQL_ROOT_PASSWORD=$(docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d)" >> .env
echo "MYSQL_PASSWORD=$(docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d)" >> .env
echo "REPLICATION_PASSWORD=$(docker secret inspect mysql_replication_password -f '{{.Spec.Data}}' | base64 -d)" >> .env
echo "MAXSCALE_USER=admin" >> .env
echo "MAXSCALE_PASSWORD=$(docker secret inspect maxscale_password -f '{{.Spec.Data}}' | base64 -d)" >> .env

# Deploy stack
docker stack deploy -c docker-compose.swarm.yml mariadb

# Wait for all 3 services to show 1/1
watch -n 2 'docker service ls | grep mariadb'
```

Press Ctrl+C when you see:
- mariadb_mariadb-primary: 1/1
- mariadb_mariadb-secondary: 1/1
- mariadb_maxscale: 1/1

---

## Step 5: Migrate Data from Old Container

```bash
cd /serverdata/docker

# Run migration script
./scripts/05-migrate-data.sh
```

**When prompted:**
- Old container name/ID: (your old mariadb container name)
- Old password: `123lol789`

**The script will:**
1. Export `panel` and `vaultwarden` databases
2. Import to new MariaDB
3. Remove old users (paneluser, etc.)
4. Create new users with random passwords from Docker secrets

---

## Step 6: Deploy PostgreSQL (Optional)

```bash
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
```

---

## Step 7: Deploy Redis

```bash
cd /serverdata/docker/redis

cat > .env <<'EOF'
VERSION=latest
REDIS_PASSWORD=
EOF

docker stack deploy -c docker-compose.swarm.yml redis
```

---

## Step 8: Deploy Pterodactyl

```bash
cd /serverdata/docker/petrodactyl

# Deploy (already configured with correct settings)
docker stack deploy -c docker-compose.swarm.yml pterodactyl

# Monitor deployment
docker service logs -f pterodactyl_panel
```

Look for successful migration messages.

---

## Step 9: Deploy Vaultwarden

```bash
cd /serverdata/docker/vaultwarden

# Edit docker-compose.swarm.yml if needed to set VAULTWARDEN_DOMAIN
# Current: VAULTWARDEN_DOMAIN: ${VAULTWARDEN_DOMAIN:-https://vw.chilla55.de}

docker stack deploy -c docker-compose.swarm.yml vaultwarden

# Monitor
docker service logs -f vaultwarden_vaultwarden
```

---

## Step 10: Deploy Certbot

```bash
cd /serverdata/docker/certbot

# Edit docker-compose.swarm.yml to configure:
# - CERT_EMAIL: your email
# - CERT_DOMAINS: your domains
# - STORAGE_BOX_HOST: your storage box hostname
# - STORAGE_BOX_USER: your storage box username

docker stack deploy -c docker-compose.swarm.yml certbot

# Wait for certificate generation (2-3 minutes)
docker service logs -f certbot_certbot
```

---

## Step 11: Deploy Nginx

```bash
cd /serverdata/docker/nginx

# Edit docker-compose.swarm.yml to configure:
# - CERT_WATCH_PATH: /etc/nginx/certs/live/your-domain.com/fullchain.pem
# - STORAGE_BOX_HOST: your storage box hostname
# - STORAGE_BOX_USER: your storage box username

docker stack deploy -c docker-compose.swarm.yml nginx

# Monitor
docker service logs -f nginx_nginx
```

---

## Step 12: Verify Deployment

```bash
# Check all services
docker service ls

# Verify database replication
docker exec $(docker ps -q -f name=mariadb_mariadb-secondary) \
  mysql -uroot -p"$(docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d)" \
  -e "SHOW SLAVE STATUS\G" | grep Seconds_Behind_Master

# Test Redis
docker exec $(docker ps -q -f name=redis_redis) redis-cli ping

# Check Pterodactyl
curl -I https://gpanel.chilla55.de

# View logs
docker service logs pterodactyl_panel --tail 50
docker service logs vaultwarden_vaultwarden --tail 50
docker service logs nginx_nginx --tail 50
```

---

## Rollback (If Needed)

```bash
# Remove specific stack
docker stack rm <stack-name>

# Remove all stacks (reverse order)
docker stack rm nginx certbot vaultwarden pterodactyl redis postgresql mariadb

# Secrets and networks are preserved for re-deployment
```

---

## Retrieve Passwords

```bash
# MariaDB root
docker secret inspect mysql_root_password -f '{{.Spec.Data}}' | base64 -d

# Pterodactyl DB password (ptero user)
docker secret inspect pterodactyl_db_password -f '{{.Spec.Data}}' | base64 -d

# Vaultwarden DB password
docker secret inspect vaultwarden_db_password -f '{{.Spec.Data}}' | base64 -d

# Vaultwarden admin token
docker secret inspect vaultwarden_admin_token -f '{{.Spec.Data}}' | base64 -d
```

---

## Summary

**With networks and labels already configured, you need to run:**

1. ✅ Create secrets (Step 1)
2. ✅ Create nginx config (Step 2)
3. ✅ Deploy MariaDB (Step 4)
4. ✅ Migrate data (Step 5) - **This creates new users with new passwords**
5. ✅ Deploy Redis (Step 7)
6. ✅ Deploy Pterodactyl (Step 8)
7. ✅ Deploy Vaultwarden (Step 9)
8. ✅ Deploy Certbot (Step 10)
9. ✅ Deploy Nginx (Step 11)
10. ✅ Verify (Step 12)

**Total time: ~30-40 minutes**
