# Pterodactyl Panel - Upgrade & Migration Guide

## 📋 Overview

This guide covers safe version upgrades, database migrations, and rollback procedures for the Pterodactyl Panel Docker Swarm deployment.

---

## 🔄 Version Upgrade Process

### Prerequisites

1. **Backup Database**
   ```bash
   # On the MariaDB host
   docker exec mariadb mysqldump -u paneluser -p panel > panel_backup_$(date +%Y%m%d).sql
   ```

2. **Backup Redis Data** (optional, but recommended)
   ```bash
   docker exec redis redis-cli --rdb /data/dump_$(date +%Y%m%d).rdb
   ```

3. **Check Current Version**
   ```bash
   docker service inspect pterodactyl_panel --format '{{.Spec.TaskTemplate.ContainerSpec.Image}}'
   ```

---

## 🚀 Upgrade Steps

### Method 1: Standard Upgrade (Recommended)

```bash
# 1. Update the stack file with new version
sed -i 's/v1.11.11/v1.11.12/g' docker-compose.swarm.yml

# 2. Enable migrations for this deployment
sed -i 's/RUN_MIGRATIONS_ON_START: "false"/RUN_MIGRATIONS_ON_START: "true"/g' docker-compose.swarm.yml

# 3. Deploy the updated stack
docker stack deploy -c docker-compose.swarm.yml pterodactyl

# 4. Monitor the deployment
docker service logs -f pterodactyl_panel

# 5. Wait for healthy status
docker service ps pterodactyl_panel

# 6. Disable automatic migrations for future restarts
sed -i 's/RUN_MIGRATIONS_ON_START: "true"/RUN_MIGRATIONS_ON_START: "false"/g' docker-compose.swarm.yml
docker stack deploy -c docker-compose.swarm.yml pterodactyl
```

### Method 2: Manual Migration (Maximum Control)

```bash
# 1. Scale service to 0 replicas
docker service scale pterodactyl_panel=0

# 2. Wait for service to stop
docker service ps pterodactyl_panel

# 3. Run migrations manually using a temporary container
docker run --rm \
  --network database \
  --network cache \
  -e APP_ENV=production \
  -e DB_HOST=mariadb \
  -e DB_DATABASE=panel \
  -e DB_USERNAME=paneluser \
  -e DB_PASSWORD="$(docker secret inspect pterodactyl_db_password --format '{{.Spec.Data}}')" \
  -e RUN_MIGRATIONS_ON_START=true \
  ghcr.io/chilla55/pterodactyl-panel:v1.11.12 \
  php artisan migrate --force

# 4. Update stack file with new version
sed -i 's/v1.11.11/v1.11.12/g' docker-compose.swarm.yml

# 5. Deploy new version
docker stack deploy -c docker-compose.swarm.yml pterodactyl

# 6. Scale back to 1 replica
docker service scale pterodactyl_panel=1
```

---

## ⏮️ Rollback Procedure

### Quick Rollback (Within 5 Minutes of Deployment)

```bash
# Docker Swarm automatically keeps previous version
docker service rollback pterodactyl_panel
```

### Manual Rollback (After Database Migration)

```bash
# 1. Scale service down
docker service scale pterodactyl_panel=0

# 2. Restore database backup
docker exec -i mariadb mysql -u paneluser -p panel < panel_backup_20241207.sql

# 3. Update stack file to previous version
sed -i 's/v1.11.12/v1.11.11/g' docker-compose.swarm.yml

# 4. Deploy previous version
docker stack deploy -c docker-compose.swarm.yml pterodactyl

# 5. Scale back to 1 replica
docker service scale pterodactyl_panel=1
```

---

## 🔍 Health Checks After Upgrade

### 1. Check Service Status
```bash
docker service ps pterodactyl_panel
docker service logs pterodactyl_panel --tail 100
```

### 2. Verify Container Health
```bash
# Should show "healthy" status
docker ps --filter "name=pterodactyl_panel" --format "table {{.Names}}\t{{.Status}}"
```

### 3. Check Database Migrations
```bash
docker exec $(docker ps -q -f name=pterodactyl_panel) php artisan migrate:status
```

### 4. Test Application Access
```bash
# Check Caddy
curl -I http://localhost:8080/caddy-health

# Check PHP-FPM (if accessible)
curl -I http://your-panel-domain.com
```

### 5. Monitor Logs
```bash
# Watch for errors
docker service logs -f pterodactyl_panel | grep -E "ERROR|WARN|FAIL"
```

---

## 🛠️ Common Migration Issues

### Issue: Migrations Fail Due to Missing APP_KEY

**Solution:**
```bash
# Generate APP_KEY first
docker run --rm ghcr.io/chilla55/pterodactyl-panel:v1.11.12 \
  php artisan key:generate --show

# Create/update secret
echo "base64:YOUR_GENERATED_KEY_HERE" | docker secret create pterodactyl_app_key_v2 -

# Update stack file to use new secret
# Then redeploy
```

### Issue: Database Connection Timeout

**Solution:**
```bash
# Check database is reachable
docker run --rm --network database busybox ping -c 3 mariadb

# Check database credentials
docker exec mariadb mysql -u paneluser -p -e "SELECT 1"
```

### Issue: Redis Connection Failed

**Solution:**
```bash
# Check Redis connectivity
docker run --rm --network cache redis:alpine redis-cli -h redis -a PASSWORD ping

# Verify Redis password secret
docker secret inspect pterodactyl_redis_password
```

---

## 📊 Pre-Migration Checklist

- [ ] Database backup completed and verified
- [ ] Redis snapshot taken (if persistent data is critical)
- [ ] Current version documented
- [ ] Stack file backed up
- [ ] Secrets verified and accessible
- [ ] Maintenance window scheduled (if applicable)
- [ ] Rollback plan prepared
- [ ] Team notified of upgrade

---

## 🔐 Secrets Management

### Creating Secrets for First Deployment

```bash
# Generate APP_KEY
APP_KEY=$(docker run --rm ghcr.io/chilla55/pterodactyl-panel:v1.11.11 php artisan key:generate --show)
echo "$APP_KEY" | docker secret create pterodactyl_app_key -

# Database password
echo "your-secure-db-password" | docker secret create pterodactyl_db_password -

# Redis password
echo "your-secure-redis-password" | docker secret create pterodactyl_redis_password -

# Mail password
echo "your-mail-password" | docker secret create pterodactyl_mail_password -
```

### Rotating Secrets

```bash
# 1. Create new secret version
echo "new-password" | docker secret create pterodactyl_db_password_v2 -

# 2. Update stack file to reference new secret
sed -i 's/pterodactyl_db_password/pterodactyl_db_password_v2/g' docker-compose.swarm.yml

# 3. Update database password
docker exec mariadb mysql -u root -p -e \
  "SET PASSWORD FOR 'paneluser'@'%' = PASSWORD('new-password');"

# 4. Redeploy
docker stack deploy -c docker-compose.swarm.yml pterodactyl

# 5. Remove old secret (after verification)
docker secret rm pterodactyl_db_password
```

---

## 📈 Zero-Downtime Upgrade (Advanced)

For production environments requiring zero downtime:

```bash
# 1. Deploy new version alongside old version (different service name)
# 2. Run migrations on new version
# 3. Switch nginx upstream to new service
# 4. Verify new version works
# 5. Scale down old version
```

**Note:** This requires more complex orchestration and is beyond the scope of this basic setup.

---

## 🧪 Testing Upgrades

### Development/Staging Environment

Always test upgrades in a non-production environment first:

```bash
# 1. Clone production database to staging
# 2. Deploy new version to staging swarm
# 3. Run migrations
# 4. Test all critical functionality
# 5. Document any issues
# 6. Proceed with production upgrade
```

---

## 📞 Support & Troubleshooting

### Logs Location
- **Container logs:** `docker service logs pterodactyl_panel`
- **Application logs:** Inside container at `/var/log/ptero/`
- **Supervisor logs:** `docker exec <container> supervisorctl tail -f <process>`

### Debug Mode (Emergency Only)

```bash
# Enable debug mode temporarily
docker service update \
  --env-add APP_DEBUG=true \
  pterodactyl_panel

# Remember to disable after troubleshooting
docker service update \
  --env-add APP_DEBUG=false \
  pterodactyl_panel
```

---

## 🔗 References

- [Pterodactyl Official Docs](https://pterodactyl.io/panel/1.0/updating.html)
- [Docker Swarm Secrets](https://docs.docker.com/engine/swarm/secrets/)
- [Laravel Migrations](https://laravel.com/docs/migrations)
