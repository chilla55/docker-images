# Quick Setup Guide - Certbot with Hetzner Storage Box (CIFS Mount)

## 1️⃣ Prepare Cloudflare API Token

```bash
# Get token from: https://dash.cloudflare.com/profile/api-tokens
# Permissions needed:
#   - Zone:DNS:Edit
#   - Zone:Zone:Read
```

## 2️⃣ Prepare Hetzner Storage Box

```bash
# Get credentials from: https://robot.hetzner.com/storage
# Enable: Samba/CIFS in the Storage Box settings
# Note: username (e.g., u123456), server (e.g., u123456.your-storagebox.de), password
```

## 3️⃣ Create Configuration Files

```bash
cd certbot/

# Cloudflare credentials
cp cloudflare.ini.example cloudflare.ini
echo "dns_cloudflare_api_token = YOUR_TOKEN" > cloudflare.ini

# Storage Box password (plain text - will be stored as Docker secret)
echo "your-storage-box-password" > storagebox.txt
```

## 4️⃣ Test Storage Box Connection

```bash
# Run as root/sudo
sudo ./test-storagebox.sh
```

## 5️⃣ Create Docker Secrets

```bash
./setup.sh

# Or manually:
docker secret create cloudflare_credentials cloudflare.ini
docker secret create storagebox_password storagebox.txt
rm storagebox.txt  # Remove after creating secret
```

## 6️⃣ Configure Service

Edit `docker-compose.swarm.yml`:

```yaml
environment:
  CERT_EMAIL: your-email@example.com
  CERT_DOMAINS: example.com,*.example.com
  STORAGE_BOX_HOST: u123456.your-storagebox.de
  STORAGE_BOX_USER: u123456
  STORAGE_BOX_PATH: /certs  # Path on Storage Box
  NGINX_SERVICE_NAME: nginx_nginx  # Check: docker service ls
```

## 7️⃣ Build & Deploy

```bash
make build
make deploy

# Or manually:
docker build -t ghcr.io/chilla55/certbot-storagebox:latest .
docker stack deploy -c docker-compose.swarm.yml certbot
```

## 8️⃣ Verify

```bash
# Watch logs
make logs

# Should see:
# ✓ Cloudflare credentials configured
# ✓ Successfully mounted Storage Box at /etc/letsencrypt
# ✓ Successfully obtained certificate
```

## 🔧 Common Commands

```bash
make logs          # View logs
make status        # Check status
make manual-renew  # Force renewal
make shell         # Open shell
make restart       # Restart service
```

## 🚨 Troubleshooting

**Cloudflare error:**
- Check API token permissions
- Verify domain is on Cloudflare

**Storage Box mount failed:**
- Test with: `sudo ./test-storagebox.sh`
- Verify Samba/CIFS enabled in Hetzner Robot panel
- Check hostname, username, password

**Nginx not reloading:**
- Check service name: `docker service ls | grep nginx`
- Update `NGINX_SERVICE_NAME` in compose file

## 📁 How It Works

1. **Certbot container starts** and mounts Storage Box to `/etc/letsencrypt` via CIFS
2. **Certificates are written** directly to Storage Box
3. **Nginx reads certificates** from the same mounted Storage Box
4. **No sync needed** - both containers access the same storage

## 📊 Certificate Location

On Storage Box (CIFS): `//u123456.your-storagebox.de/certs/`
Mounted in certbot: `/etc/letsencrypt/`
Same mount in nginx: `/etc/nginx/certs/` (to be configured)

## ⏰ Renewal Schedule

- Checks every 12 hours (configurable)
- Let's Encrypt renews 30 days before expiry
- Writes directly to Storage Box mount
- Auto-reloads nginx

## ✨ Benefits of CIFS Mount

- ✅ **Simpler** - No sync process needed
- ✅ **Direct access** - Both certbot and nginx read from same source
- ✅ **Automatic backup** - Storage Box handles redundancy
- ✅ **Multi-node** - Share across Docker Swarm nodes
- ✅ **Real-time** - Changes immediately visible to all containers
