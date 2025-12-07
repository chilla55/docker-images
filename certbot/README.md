# Certbot with Hetzner Storage Box Integration

Automated Let's Encrypt certificate management with Cloudflare DNS validation and Hetzner Storage Box backup/sync for Docker Swarm.

## Features

- 🔐 **Automatic certificate renewal** using Let's Encrypt
- 🌐 **Cloudflare DNS-01 challenge** for wildcard certificates
- 💾 **Hetzner Storage Box sync** via rclone (SFTP/WebDAV/FTP)
- 🔄 **Automatic nginx reload** after certificate renewal
- 🐳 **Docker Swarm compatible** with secrets management
- 📊 **Health checks** to monitor certificate validity
- 🔁 **Bi-directional sync** - pull existing certs from Storage Box on startup

## Architecture

```
┌─────────────────┐
│  Certbot        │
│  Container      │
│                 │
│  1. Renew certs │◄────── Cloudflare DNS API
│  2. Fix perms   │
│  3. Sync ────────┼────►  Hetzner Storage Box
│  4. Reload nginx│        (SFTP/WebDAV/FTP)
└─────────────────┘
         │
         │ (shared volume)
         ▼
┌─────────────────┐
│  Nginx          │
│  Container(s)   │
│                 │
│  Uses certs ────┼────►  SSL/TLS for websites
└─────────────────┘
```

## Quick Start

### 1. Prerequisites

- Docker Swarm initialized
- Cloudflare account with domain
- Hetzner Storage Box (any size)

### 2. Get Cloudflare API Token

1. Go to https://dash.cloudflare.com/profile/api-tokens
2. Create token with permissions:
   - `Zone:DNS:Edit`
   - `Zone:Zone:Read`
3. Copy the token

### 3. Get Hetzner Storage Box Credentials

1. Go to https://robot.hetzner.com/storage
2. Note your Storage Box details:
   - Username (e.g., `u123456`)
   - Server (e.g., `u123456.your-storagebox.de`)
   - Password
3. Enable SFTP/WebDAV in Storage Box settings

### 4. Setup Configuration Files

```bash
cd certbot/

# Create Cloudflare credentials
cp cloudflare.ini.example cloudflare.ini
nano cloudflare.ini
# Add your Cloudflare API token

# Create rclone configuration
cp rclone.conf.example rclone.conf

# Encrypt your Storage Box password
docker run --rm -it rclone/rclone obscure 'your-storage-box-password'
# Copy the encrypted output

nano rclone.conf
# Fill in your Storage Box details and encrypted password
```

### 5. Test Storage Box Connection

```bash
# Make test script executable
chmod +x test-storagebox.sh

# Test connection
./test-storagebox.sh ./rclone.conf hetzner-storagebox
```

### 6. Create Docker Secrets

```bash
# Make setup script executable
chmod +x setup.sh

# Run setup (creates Docker secrets)
./setup.sh
```

Or manually:

```bash
docker secret create cloudflare_credentials cloudflare.ini
docker secret create rclone_config rclone.conf
```

### 7. Configure docker-compose.swarm.yml

Edit the environment variables:

```yaml
environment:
  CERT_EMAIL: your-email@example.com
  CERT_DOMAINS: example.com,*.example.com
  NGINX_SERVICE_NAME: nginx_nginx  # Your nginx service name
```

### 8. Build and Deploy

```bash
# Using Makefile
make build
make deploy

# Or manually
docker build -t ghcr.io/chilla55/certbot-storagebox:latest .
docker stack deploy -c docker-compose.swarm.yml certbot
```

### 9. Monitor

```bash
# View logs
docker service logs -f certbot_certbot

# Check status
docker stack ps certbot

# Or using Makefile
make logs
make status
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CERT_EMAIL` | `admin@example.com` | Email for Let's Encrypt notifications |
| `CERT_DOMAINS` | `example.com,*.example.com` | Comma-separated list of domains |
| `RENEW_INTERVAL` | `12h` | How often to check for renewals (s/m/h/d) |
| `CLOUDFLARE_CREDENTIALS` | `/run/secrets/cloudflare_credentials` | Path to Cloudflare credentials |
| `STORAGE_BOX_ENABLED` | `true` | Enable/disable Storage Box sync |
| `STORAGE_BOX_REMOTE` | `hetzner-storagebox` | Rclone remote name |
| `STORAGE_BOX_PATH` | `/certs` | Path on Storage Box |
| `RCLONE_CONFIG` | `/run/secrets/rclone_config` | Path to rclone config |
| `NGINX_RELOAD_ENABLED` | `true` | Auto-reload nginx after renewal |
| `NGINX_SERVICE_NAME` | `nginx_nginx` | Docker service name pattern for nginx |
| `DEBUG` | `false` | Enable debug logging |

### Cloudflare Configuration

**cloudflare.ini** (Option 1 - API Token - Recommended):
```ini
dns_cloudflare_api_token = your_api_token_here
```

**cloudflare.ini** (Option 2 - Global API Key):
```ini
dns_cloudflare_email = your-email@example.com
dns_cloudflare_api_key = your_global_api_key_here
```

### Rclone Configuration for Hetzner Storage Box

#### SFTP (Recommended)

```ini
[hetzner-storagebox]
type = sftp
host = u123456.your-storagebox.de
user = u123456
port = 23
pass = ENCRYPTED_PASSWORD_HERE
shell_type = unix
```

#### WebDAV (Alternative)

```ini
[hetzner-storagebox]
type = webdav
url = https://u123456.your-storagebox.de
vendor = other
user = u123456
pass = ENCRYPTED_PASSWORD_HERE
```

#### FTP (Not Recommended)

```ini
[hetzner-storagebox]
type = ftp
host = u123456.your-storagebox.de
user = u123456
port = 21
pass = ENCRYPTED_PASSWORD_HERE
```

**Encrypt password:**
```bash
docker run --rm -it rclone/rclone obscure 'your-plain-password'
```

## Makefile Commands

```bash
make help          # Show all available commands
make build         # Build Docker image
make setup         # Setup Docker secrets
make test-rclone   # Test Storage Box connection
make deploy        # Deploy stack
make logs          # View logs
make status        # Show stack status
make manual-renew  # Force certificate renewal
make manual-sync   # Force sync to Storage Box
make shell         # Open shell in container
make remove        # Remove stack
make restart       # Restart service
```

## How It Works

### Initial Deployment

1. Container starts and pulls existing certificates from Storage Box (if any)
2. Checks if certificates exist locally
3. If not, obtains new certificates from Let's Encrypt
4. Fixes permissions for nginx (gid 1001)
5. Syncs certificates to Storage Box
6. Enters renewal loop

### Renewal Process

Every `RENEW_INTERVAL` (default 12 hours):

1. Checks if certificates need renewal (Let's Encrypt renews 30 days before expiry)
2. If renewal needed:
   - Obtains new certificates via Cloudflare DNS challenge
   - Fixes permissions
   - Syncs to Storage Box
   - Signals nginx to reload
3. Sleeps until next check

### Storage Box Sync

- **Bi-directional:** Can pull and push certificates
- **Automatic:** Syncs after every renewal
- **Resilient:** Multiple retries with backoff
- **Efficient:** Only syncs changed files

## Nginx Integration

The nginx service should use the shared volume:

```yaml
services:
  nginx:
    volumes:
      - certbot_certs:/etc/nginx/certs:ro

volumes:
  certbot_certs:
    external: true
    name: certbot_certbot_data
```

Example nginx config:

```nginx
server {
    listen 443 ssl http2;
    server_name example.com;
    
    ssl_certificate /etc/nginx/certs/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/live/example.com/privkey.pem;
    
    # ... rest of config
}
```

## Troubleshooting

### Certificate not obtained

```bash
# Check logs
make logs

# Common issues:
# - Wrong Cloudflare API token/permissions
# - Domain not managed by Cloudflare
# - DNS not propagated yet
```

### Storage Box connection failed

```bash
# Test connection
./test-storagebox.sh

# Common issues:
# - Wrong credentials
# - SFTP/WebDAV not enabled in Storage Box settings
# - Wrong port (SFTP: 23, WebDAV: 443, FTP: 21)
# - Password not encrypted with 'rclone obscure'
```

### Nginx not reloading

```bash
# Check nginx service name
docker service ls | grep nginx

# Update in docker-compose.swarm.yml:
NGINX_SERVICE_NAME: your_actual_service_name

# Or disable auto-reload and reload manually:
NGINX_RELOAD_ENABLED: "false"
```

### Manual certificate renewal

```bash
make manual-renew

# Or directly:
docker exec $(docker ps -qf "name=certbot_certbot") \
    certbot renew --force-renewal
```

### Manual sync to Storage Box

```bash
make manual-sync

# Or directly:
docker exec $(docker ps -qf "name=certbot_certbot") \
    rclone sync /etc/letsencrypt hetzner-storagebox:/certs -v
```

### Check certificate expiry

```bash
docker exec $(docker ps -qf "name=certbot_certbot") \
    certbot certificates
```

## Security Considerations

1. **Secrets Management:** All credentials stored as Docker secrets
2. **Encrypted Passwords:** Rclone passwords should be encrypted
3. **Read-only Mounts:** Nginx mounts certificates as read-only
4. **Least Privilege:** Cloudflare API token with minimal permissions
5. **No Root:** Container runs as non-root where possible

## Storage Box Benefits

- ✅ **Centralized Storage:** One place for all certificates
- ✅ **Multi-node Access:** Share certs across Docker Swarm nodes
- ✅ **Backup:** Automatic backup to Hetzner's infrastructure
- ✅ **Disaster Recovery:** Easy restore from Storage Box
- ✅ **10 Snapshots:** Hetzner provides automatic snapshots
- ✅ **Version History:** Can recover from accidental deletions

## Advanced Usage

### Multiple Domains

```yaml
environment:
  CERT_DOMAINS: example.com,*.example.com,another.com,*.another.com
```

### Different Renewal Interval

```yaml
environment:
  RENEW_INTERVAL: 6h  # Check every 6 hours
```

### Disable Storage Box (local only)

```yaml
environment:
  STORAGE_BOX_ENABLED: "false"
```

### Custom Storage Box Path

```yaml
environment:
  STORAGE_BOX_PATH: /backups/ssl-certs
```

## Migration from Existing Setup

If you have existing certificates:

1. Deploy certbot service with `STORAGE_BOX_ENABLED: "false"`
2. Copy existing certificates to `/etc/letsencrypt` in container
3. Run manual sync: `make manual-sync`
4. Enable Storage Box sync and redeploy

Or upload directly to Storage Box:

```bash
rclone sync /path/to/existing/letsencrypt \
    hetzner-storagebox:/certs \
    --config rclone.conf
```

## License

MIT License - see repository for details

## Support

For issues or questions:
- Check logs: `make logs`
- Test Storage Box: `./test-storagebox.sh`
- Review Hetzner docs: https://docs.hetzner.com/robot/storage-box
- Review Let's Encrypt docs: https://letsencrypt.org/docs/
