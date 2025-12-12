# Quick Setup - Certbot (Cloudflare + host SMB mount)

1) **Mount Storage Box on host**
```bash
sudo mkdir -p /mnt/storagebox/certs
# Ensure fstab/systemd mount exists; see FSTAB_SETUP.md
mount | grep storagebox
```

2) **Create Cloudflare secret**
```bash
printf 'dns_cloudflare_api_token = %s\n' "$CF_API_TOKEN" | docker secret create cloudflare_api_token -
```

3) **Adjust compose (if needed)**
- `CERT_EMAIL`: admin@chilla55.de (change to yours)
- `CERT_DOMAINS`: chilla55.de,*.chilla55.de (comma-separated)
- `RENEW_INTERVAL`: `12h` by default
- `CERTBOT_DRY_RUN`: set to `false` for production

4) **Deploy**
```bash
docker stack deploy -c docker-compose.swarm.yml certbot
```

5) **Verify**
```bash
docker service logs -f certbot_certbot
```
- You should see the mount info for `/etc/letsencrypt` and renewal loop starting.

6) **Nginx usage**
Bind the same host path (`/mnt/storagebox/certs`) into nginx as read-only (e.g., `/etc/nginx/certs`).

That’s it—no rclone sync or container-side mounts required.
