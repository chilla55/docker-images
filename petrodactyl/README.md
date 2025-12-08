# Pterodactyl Panel - Docker Swarm Deployment

![Pterodactyl](https://pterodactyl.io/logos/Banner%20Logo%20Black@2x.png)

**All-in-one Pterodactyl Panel container for Docker Swarm** - Includes PHP-FPM, Caddy, Queue Worker, and Scheduler in a single, stateless container.

---

## 🌟 Features

- ✅ **Stateless Design** - No bind mounts, runs on any Swarm node
- 🔒 **Version Pinned** - Explicit Pterodactyl version baked into image
- 🔐 **Secrets Management** - Native Docker Swarm secrets for sensitive data
- 🏥 **Health Checks** - Comprehensive monitoring of all services
- 📦 **Multi-Stage Build** - Optimized Alpine-based image (~200MB)
- 🚀 **Production Ready** - Proper logging, caching, and error handling
- 🔄 **Easy Updates** - Controlled migration process
- 📊 **Multi-Platform** - Supports amd64 and arm64

---

## 📁 Repository Structure

```
petrodactyl/
├── Dockerfile              # Multi-stage optimized build
├── Caddyfile              # Static file server configuration
├── supervisord.conf       # Process manager for all services
├── entrypoint.sh          # Initialization and secret loading
├── healthcheck.sh         # Comprehensive health checks
├── docker-compose.swarm.yml  # Docker Swarm stack file
├── UPGRADE_GUIDE.md       # Migration and upgrade procedures
└── README.md              # This file
```

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────┐
│         Docker Swarm Node (Any Node)            │
│                                                 │
│  ┌───────────────────────────────────────────┐ │
│  │  Pterodactyl Panel Container              │ │
│  │                                           │ │
│  │  ┌─────────────┐  ┌──────────────────┐  │ │
│  │  │ Supervisord │──│  PHP-FPM :9000   │  │ │
│  │  └─────────────┘  └──────────────────┘  │ │
│  │         │                                │ │
│  │         ├────────► Caddy :8080          │ │
│  │         │                                │ │
│  │         ├────────► Queue Worker         │ │
│  │         │                                │ │
│  │         └────────► Scheduler            │ │
│  └───────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘
           │              │
           ▼              ▼
    ┌──────────┐   ┌──────────┐
    │  MariaDB │   │  Redis   │
    └──────────┘   └──────────┘
           │
           ▼
    ┌──────────────┐
    │ Nginx (TLS)  │
    └──────────────┘
```

---

## 🚀 Quick Start

### Prerequisites

1. Docker Swarm initialized
2. External networks created:
   ```bash
   docker network create --driver overlay proxy
   docker network create --driver overlay database
   docker network create --driver overlay cache
   ```
3. MariaDB and Redis services running
4. Nginx reverse proxy configured

### Initial Deployment

#### 1. Create Secrets

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

#### 2. Configure Stack File

Edit `docker-compose.swarm.yml`:

```yaml
environment:
  APP_URL: https://your-panel-domain.com
  DB_HOST: mariadb
  DB_DATABASE: panel
  DB_USERNAME: paneluser
  REDIS_HOST: redis
  MAIL_HOST: smtp.your-domain.com
  MAIL_USERNAME: noreply@your-domain.com
  # ... other settings
```

#### 3. Deploy Stack

```bash
# For first deployment, enable migrations and seeding
sed -i 's/RUN_MIGRATIONS_ON_START: "false"/RUN_MIGRATIONS_ON_START: "true"/g' docker-compose.swarm.yml
sed -i 's/RUN_SEED_ON_START: "false"/RUN_SEED_ON_START: "true"/g' docker-compose.swarm.yml

# Deploy
docker stack deploy -c docker-compose.swarm.yml pterodactyl

# Monitor deployment
docker service logs -f pterodactyl_panel
```

#### 4. Create Admin User

```bash
# Wait for service to be healthy
docker service ps pterodactyl_panel

# Create admin user
docker exec $(docker ps -q -f name=pterodactyl_panel) \
  php artisan p:user:make \
  --email=admin@example.com \
  --username=admin \
  --name-first=Admin \
  --name-last=User \
  --password=SecurePassword123 \
  --admin=1
```

#### 5. Disable Auto-Migration

```bash
# After successful deployment, disable automatic migrations
sed -i 's/RUN_MIGRATIONS_ON_START: "true"/RUN_MIGRATIONS_ON_START: "false"/g' docker-compose.swarm.yml
sed -i 's/RUN_SEED_ON_START: "true"/RUN_SEED_ON_START: "false"/g' docker-compose.swarm.yml

# Redeploy
docker stack deploy -c docker-compose.swarm.yml pterodactyl
```

---

## 🔧 Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `APP_ENV` | Yes | `production` | Application environment |
| `APP_URL` | Yes | - | Public URL of panel |
| `APP_KEY_FILE` | Yes | - | Path to APP_KEY secret |
| `DB_HOST` | Yes | `mariadb` | Database host |
| `DB_DATABASE` | Yes | `panel` | Database name |
| `DB_USERNAME` | Yes | `paneluser` | Database user |
| `DB_PASSWORD_FILE` | Yes | - | Path to DB password secret |
| `REDIS_HOST` | Yes | `redis` | Redis host |
| `REDIS_PASSWORD_FILE` | No | - | Path to Redis password secret |
| `RUN_MIGRATIONS_ON_START` | No | `false` | Run migrations on startup |
| `RUN_SEED_ON_START` | No | `false` | Run seeders on startup |

See `docker-compose.swarm.yml` for complete list.

---

## 🔐 Secrets

The container uses Docker Swarm secrets for sensitive data:

- `pterodactyl_app_key` - Laravel APP_KEY (base64: prefixed)
- `pterodactyl_db_password` - Database password
- `pterodactyl_redis_password` - Redis password (optional)
- `pterodactyl_mail_password` - SMTP password (optional)

**Security Note:** Never commit secrets to version control.

---

## 📊 Monitoring & Health Checks

### Service Health

```bash
# Check service status
docker service ps pterodactyl_panel

# View logs
docker service logs pterodactyl_panel --tail 100 -f

# Check container health
docker ps --filter "name=pterodactyl_panel"
```

### Health Check Details

The healthcheck script verifies:
- ✅ `.env` file exists
- ✅ `APP_KEY` is configured
- ✅ PHP-FPM process and port 9000
- ✅ Caddy process and port 8080
- ✅ Queue worker process
- ✅ Scheduler process
- ✅ Database connectivity
- ✅ Migration status

---

## 🔄 Upgrades

See [UPGRADE_GUIDE.md](./UPGRADE_GUIDE.md) for detailed upgrade procedures.

**Quick Upgrade:**

```bash
# 1. Update version in stack file
sed -i 's/v1.11.11/v1.11.12/g' docker-compose.swarm.yml

# 2. Enable migrations
sed -i 's/RUN_MIGRATIONS_ON_START: "false"/RUN_MIGRATIONS_ON_START: "true"/g' docker-compose.swarm.yml

# 3. Deploy
docker stack deploy -c docker-compose.swarm.yml pterodactyl

# 4. Monitor
docker service logs -f pterodactyl_panel
```

---

## 🐛 Troubleshooting

### Container Won't Start

```bash
# Check logs
docker service logs pterodactyl_panel --tail 200

# Common issues:
# - Missing secrets
# - Database unreachable
# - Invalid APP_KEY format
```

### Database Connection Failed

```bash
# Test database connectivity
docker exec $(docker ps -q -f name=pterodactyl_panel) \
  mariadb -h mariadb -u paneluser -p -e "SELECT 1"

# Verify secrets
docker secret inspect pterodactyl_db_password
```

### Queue Worker Not Processing Jobs

```bash
# Check queue worker logs
docker exec $(docker ps -q -f name=pterodactyl_panel) \
  supervisorctl tail -f queue-worker

# Restart queue worker
docker exec $(docker ps -q -f name=pterodactyl_panel) \
  supervisorctl restart queue-worker
```

### PHP-FPM Not Responding

```bash
# Check PHP-FPM status
docker exec $(docker ps -q -f name=pterodactyl_panel) \
  supervisorctl status php-fpm

# Check PHP-FPM port
docker exec $(docker ps -q -f name=pterodactyl_panel) \
  nc -zv localhost 9000
```

---

## 🏗️ Building Locally

```bash
# Build for single platform
docker build \
  --build-arg PANEL_VERSION=v1.11.11 \
  -t pterodactyl-panel:v1.11.11 \
  ./petrodactyl

# Build multi-platform
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg PANEL_VERSION=v1.11.11 \
  -t pterodactyl-panel:v1.11.11 \
  ./petrodactyl
```

---

## 🔗 External Services

### Nginx Reverse Proxy Configuration

Example nginx configuration for upstream:

```nginx
upstream pterodactyl_static {
    server pterodactyl-panel:80;  # Caddy for static files
}

upstream pterodactyl_php {
    server pterodactyl-panel:9000;  # PHP-FPM
}

server {
    listen 443 ssl http2;
    server_name panel.example.com;

    # SSL configuration
    ssl_certificate /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;

    # PHP requests
    location ~ \.php$ {
        fastcgi_pass pterodactyl_php;
        fastcgi_index index.php;
        fastcgi_param SCRIPT_FILENAME $document_root$fastcgi_script_name;
        include fastcgi_params;
    }

    # Static files
    location / {
        proxy_pass http://pterodactyl_static;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## 📜 License

This Docker setup is provided as-is. Pterodactyl Panel itself is licensed under the MIT License.

---

## 🙏 Credits

- **Pterodactyl Panel:** [pterodactyl.io](https://pterodactyl.io)
- **Docker:** [docker.com](https://docker.com)
- **Caddy:** [caddyserver.com](https://caddyserver.com)

---

## 📞 Support

- **Pterodactyl Docs:** https://pterodactyl.io/panel/1.0/getting_started.html
- **Issues:** GitHub Issues
- **Discussions:** GitHub Discussions

---

**Version:** 1.0.0  
**Last Updated:** December 7, 2025
