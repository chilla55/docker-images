# Pterodactyl Panel - Quick Reference

## 📋 File Overview

```
petrodactyl/
├── Dockerfile              # Multi-stage Alpine build (~200MB)
├── Caddyfile              # Static file server (port 8080)
├── supervisord.conf       # Process manager (PHP-FPM, Caddy, Worker, Scheduler)
├── entrypoint.sh          # Initialization, secret loading, validation
├── healthcheck.sh         # Comprehensive health checks
├── docker-compose.swarm.yml  # Swarm deployment stack
├── validate-env.sh        # Pre-deployment validation
├── Makefile               # Common operations
├── README.md              # Full documentation
├── UPGRADE_GUIDE.md       # Migration procedures
└── .dockerignore          # Build optimization
```

---

## 🚀 Common Commands

### First-Time Setup
```bash
# 1. Validate environment
./validate-env.sh

# 2. Create secrets
make secrets-create

# 3. Edit configuration
nano docker-compose.swarm.yml  # Update APP_URL, DB settings, etc.

# 4. Deploy with migrations
make deploy-migrate

# 5. Create admin user
make exec CMD="php artisan p:user:make --email=admin@example.com --admin=1"
```

### Daily Operations
```bash
make logs              # Follow logs
make health            # Check service health
make ps                # Show service tasks
make exec CMD="..."    # Run artisan command
make shell             # Open container shell
```

### Upgrades
```bash
make upgrade NEW_VERSION=v1.11.12   # Automated upgrade
make rollback                        # Quick rollback
```

### Troubleshooting
```bash
make logs-tail         # Last 100 lines
make test              # Run healthcheck
docker service ps pterodactyl_panel --no-trunc  # Full task details
```

---

## 🔐 Required Secrets

| Secret | Required | Command |
|--------|----------|---------|
| `pterodactyl_app_key` | ✅ | `docker run --rm IMAGE php artisan key:generate --show` |
| `pterodactyl_db_password` | ✅ | Your database password |
| `pterodactyl_redis_password` | ⚠️ | Redis password (if auth enabled) |
| `pterodactyl_mail_password` | ⚠️ | SMTP password (if using email) |

---

## 🏗️ Architecture Components

| Component | Port | Purpose |
|-----------|------|---------|
| **PHP-FPM** | 9000 | PHP processing (FastCGI) |
| **Caddy** | 80 | Static file serving |
| **Queue Worker** | - | Background job processing |
| **Scheduler** | - | Cron job execution |
| **Supervisord** | - | Process management |

---

## 📊 Service Ports

```
External Nginx → 443/80
         ↓
    Docker Swarm
         ↓
┌────────────────────┐
│ Pterodactyl Panel  │
│  - Caddy:     80   │ ← Static files
│  - PHP-FPM:   9000 │ ← PHP processing
└────────────────────┘
```

---

## 🔍 Health Check Points

✅ `.env` file exists  
✅ `APP_KEY` configured  
✅ PHP-FPM process running  
✅ PHP-FPM port 9000 open  
✅ Caddy process running  
✅ Caddy port 80 open  
✅ Queue worker running  
✅ Scheduler running  
✅ Database connectivity  
✅ Migrations applied  

---

## ⚡ Performance Tuning

### PHP-FPM (in Dockerfile)
- `pm = ondemand` - Spawn workers on demand
- `pm.max_children = 20` - Max worker processes
- `memory_limit = 256M` - Per-request memory

### Supervisor
- `priority` - Start order (PHP-FPM first)
- `stopwaitsecs` - Graceful shutdown time
- `autorestart = true` - Auto-recovery

### Docker Resources
```yaml
resources:
  limits:
    cpus: '2.0'
    memory: 2G
  reservations:
    cpus: '0.5'
    memory: 512M
```

---

## 🐛 Common Issues

| Problem | Solution |
|---------|----------|
| Container won't start | Check `make logs` for errors |
| DB connection failed | Verify secrets and network connectivity |
| PHP-FPM not responding | Check port 9000: `nc -zv localhost 9000` |
| Queue worker stuck | Restart: `make exec CMD="supervisorctl restart queue-worker"` |
| Migrations failed | Check `APP_KEY` format (must start with `base64:`) |

---

## 📝 Environment Variables

### Critical Settings
```bash
APP_ENV=production          # Never use 'local' or 'development'
APP_DEBUG=false            # Never enable in production
APP_URL=https://your.domain  # Must match nginx configuration
TRUSTED_PROXIES=*          # For nginx reverse proxy
```

### Database
```bash
DB_HOST=mariadb
DB_PORT=3306
DB_DATABASE=panel
DB_USERNAME=paneluser
DB_PASSWORD_FILE=/run/secrets/pterodactyl_db_password
```

### Runtime Behavior
```bash
RUN_MIGRATIONS_ON_START=false   # Only enable during upgrades
RUN_SEED_ON_START=false         # Only enable for first deployment
```

---

## 🔄 Update Workflow

1. **Backup** → `make backup-db`
2. **Test** → Deploy to staging first
3. **Upgrade** → `make upgrade NEW_VERSION=vX.Y.Z`
4. **Monitor** → `make logs`
5. **Verify** → `make health`
6. **Rollback if needed** → `make rollback`

---

## 📞 Quick Links

- **Documentation:** `README.md`
- **Upgrade Guide:** `UPGRADE_GUIDE.md`
- **Validation:** `./validate-env.sh`
- **Pterodactyl Docs:** https://pterodactyl.io/panel/1.0/getting_started.html

---

## 🎯 Best Practices

✅ Always backup database before upgrades  
✅ Test upgrades in staging first  
✅ Keep `RUN_MIGRATIONS_ON_START=false` in production  
✅ Monitor logs during deployment  
✅ Use secrets for all sensitive data  
✅ Pin versions (don't use `:latest` in production)  
✅ Set resource limits in stack file  
✅ Enable healthchecks  
✅ Use `start-first` update order  

---

**Last Updated:** December 7, 2025  
**Version:** 1.0.0
