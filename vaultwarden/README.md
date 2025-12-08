# Vaultwarden for Docker Swarm

Self-hosted Bitwarden-compatible password manager with enterprise-grade security.

## Features

- **Secrets Management**: All passwords stored as Docker secrets
- **MySQL Backend**: Connects to MariaDB via MaxScale orchestrator
- **SMTP Integration**: Email notifications for vault access
- **RSA Key Persistence**: Stored on Hetzner Storage Box CIFS mount
- **Health Monitoring**: Checks `/alive` endpoint every 30s
- **Multi-arch**: linux/amd64, linux/arm64
- **Web Vault**: Built-in web interface

## Architecture

```
┌─────────────┐
│ Vaultwarden │ (node1)
│  Container  │
└──────┬──────┘
       │
       ├─── web-net ────────> nginx (reverse proxy)
       │
       ├─── mariadb-net ────> maxscale:4006 (MySQL)
       │
       └─── CIFS mount ─────> Storage Box /vaultwarden/keys
                               (RSA key persistence)
```

## Prerequisites

### 1. Create Docker Secrets

```bash
# Database password
echo "your-db-password" | docker secret create vaultwarden_db_password -

# Admin token (argon2id hash - generate via Vaultwarden admin panel or online)
echo '$argon2id$v=19$m=65540,t=3,p=4$...' | docker secret create vaultwarden_admin_token -

# SMTP password
echo "your-smtp-password" | docker secret create vaultwarden_smtp_password -

# Storage Box password (shared with nginx/certbot)
echo "your-storagebox-password" | docker secret create storagebox_password -
```

**Generate Admin Token:**
```bash
# Using Vaultwarden CLI
docker run --rm -it vaultwarden/server /vaultwarden hash --preset owasp
# Enter your desired admin password when prompted
```

### 2. Configure Storage Box

Create directory structure on your Hetzner Storage Box:

```bash
# SSH into Storage Box
ssh u123456@u123456.your-storagebox.de

# Create Vaultwarden keys directory
mkdir -p /backup/vaultwarden/keys
chmod 700 /backup/vaultwarden/keys
exit
```

### 3. Label Nodes

```bash
# Run Vaultwarden on node1 (where data volume is)
docker node update --label-add vaultwarden.node=node1 <node1-name>
```

### 4. Create Networks

```bash
# Application network (nginx -> vaultwarden)
docker network create --driver overlay --opt encrypted=true web-net

# Database network (vaultwarden -> maxscale)
docker network create --driver overlay --opt encrypted=true mariadb-net
```

## Quick Start

### Build Image

### Build Image

```bash
make build
```

### Deploy to Swarm

```bash
# Deploy stack (secrets must exist first)
docker stack deploy -c docker-compose.swarm.yml vaultwarden

# Check status
docker service ls | grep vaultwarden
docker service logs -f vaultwarden_vaultwarden
```

## Database Setup

Create database and user in MariaDB:

```sql
-- Connect to MariaDB primary
docker exec -it $(docker ps -q -f name=mariadb-primary) mysql -u root -p

-- Create database and user
CREATE DATABASE IF NOT EXISTS vaultwarden CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'vaultwarden'@'%' IDENTIFIED BY 'your-db-password';
GRANT ALL PRIVILEGES ON vaultwarden.* TO 'vaultwarden'@'%';
FLUSH PRIVILEGES;
```

## Configuration

All settings configured via environment variables in `docker-compose.swarm.yml`.

### Database Settings

```yaml
DB_TYPE: mysql
DB_HOST: maxscale
DB_PORT: 4006
DB_USER: vaultwarden
DB_NAME: vaultwarden
DATABASE_PASSWORD_FILE: /run/secrets/vaultwarden_db_password
```

### Storage Box Settings

```yaml
STORAGE_BOX_KEYS_ENABLED: true
STORAGE_BOX_HOST: u123456.your-storagebox.de
STORAGE_BOX_USER: u123456
STORAGE_BOX_PASSWORD_FILE: /run/secrets/vaultwarden_storagebox_password
STORAGE_BOX_KEYS_PATH: /vaultwarden/keys
STORAGE_BOX_KEYS_MOUNT_POINT: /data/keys
```

**How RSA Keys Work:**

1. On first startup, Vaultwarden generates `rsa_key.pem` and `rsa_key.pub.pem` in `/data/`
2. Entrypoint script mounts Storage Box to `/data/keys`
3. If keys exist on Storage Box, they're symlinked to `/data/`
4. If not, Vaultwarden generates them, then they're moved to Storage Box
5. Keys persist across container restarts and node failures

### SMTP Settings

```yaml
SMTP_HOST: smtp.mailbox.org
SMTP_FROM: vaultwarden@your-domain.com
SMTP_PORT: 465
SMTP_SECURITY: force_tls
SMTP_USERNAME: vaultwarden@your-domain.com
SMTP_PASSWORD_FILE: /run/secrets/vaultwarden_smtp_password
```

## Nginx Configuration

Add to nginx reverse proxy:

```nginx
server {
    listen 443 ssl http2;
    server_name vault.your-domain.com;

    ssl_certificate /etc/nginx/certs/vault.your-domain.com/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/vault.your-domain.com/privkey.pem;

    location / {
        proxy_pass http://vaultwarden:80;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebSocket support for notifications
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    client_max_body_size 525M;
}
```

## Monitoring

### Health Check

```bash
# Check service health
docker service ps vaultwarden_vaultwarden

# Manual health check
docker exec $(docker ps -q -f name=vaultwarden) /usr/local/bin/healthcheck.sh
```

### Storage Box Mount

```bash
# Verify mount
docker exec $(docker ps -q -f name=vaultwarden) mountpoint -q /data/keys && echo "Mounted" || echo "Not mounted"

# Check RSA keys
docker exec $(docker ps -q -f name=vaultwarden) ls -la /data/
docker exec $(docker ps -q -f name=vaultwarden) ls -la /data/keys/
```

## Backup & Restore

### Automatic Backup

RSA keys automatically backed up to Storage Box at `/vaultwarden/keys/`.

### Manual Backup

```bash
# Backup data directory
docker run --rm \
  -v vaultwarden_data:/data \
  -v $(pwd):/backup \
  alpine tar czf /backup/vaultwarden-backup-$(date +%Y%m%d).tar.gz /data

# Download from Storage Box
scp u123456@u123456.your-storagebox.de:/backup/vaultwarden/keys/* ./backup/
```

### Restore

```bash
# Upload to Storage Box
scp rsa_key.pem rsa_key.pub.pem u123456@u123456.your-storagebox.de:/backup/vaultwarden/keys/

# Keys auto-restored on next container start
```

## Troubleshooting

### Storage Box Mount Failed

```bash
# Test CIFS mount manually
docker run --rm -it --privileged alpine sh

# Inside container:
apk add cifs-utils
mkdir /test
mount -t cifs "//u123456.your-storagebox.de/backup/vaultwarden/keys" /test \
  -o "username=u123456,password=your-pass,vers=3.0"
ls /test
```

### Database Connection Failed

```bash
# Test MaxScale connectivity
docker exec $(docker ps -q -f name=vaultwarden) nc -zv maxscale 4006
```

### Admin Panel Not Accessible

Access at: `https://vault.your-domain.com/admin`

## Security Notes

1. **Secrets**: All passwords stored as Docker secrets
2. **Network Encryption**: overlay networks use `--opt encrypted=true`
3. **No Port Exposure**: Only accessible via nginx on `web-net`
4. **HTTPS Required**: Always use SSL/TLS
5. **Admin Token**: Use strong argon2id hash
6. **Storage Box**: CIFS encryption (vers=3.0), permissions 0600/0700

## Networks

- **web-net**: Nginx reverse proxy access
- **mariadb-net**: Database access via MaxScale

## Resource Usage

- **CPU**: 0.5-1 core
- **Memory**: 256-512MB
- **Storage**: ~50MB + user data

## Backup

Data persists in the `vaultwarden-data` volume:

```bash
# Backup
docker run --rm -v vaultwarden_vaultwarden-data:/data -v $(pwd):/backup alpine tar czf /backup/vaultwarden-backup.tar.gz -C /data .

# Restore
docker run --rm -v vaultwarden_vaultwarden-data:/data -v $(pwd):/backup alpine tar xzf /backup/vaultwarden-backup.tar.gz -C /data
```

## Security

- Disable signups after creating accounts
- Use strong admin token (generate with `openssl rand -base64 48`)
- Enable 2FA for all users
- Regular backups
- HTTPS only (enforced by DOMAIN setting)

## Nginx Configuration

Example nginx config for Vaultwarden:

```nginx
server {
    listen 443 ssl http2;
    server_name vw.example.com;

    location / {
        proxy_pass http://vaultwarden:80;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /notifications/hub {
        proxy_pass http://vaultwarden:3012;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    location /notifications/hub/negotiate {
        proxy_pass http://vaultwarden:80;
    }
}
```

## Troubleshooting

**Check logs:**
```bash
docker service logs vaultwarden_vaultwarden
```

**Verify database connection:**
```bash
docker exec -it $(docker ps -q -f name=vaultwarden) sh
wget -O- http://maxscale:4006
```

**Reset admin token:**
Update `ADMIN_TOKEN` environment variable and redeploy.
