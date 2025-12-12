# Certbot with Cloudflare DNS and Host-Mounted Storage Box

Lightweight Certbot image that solves DNS-01 via Cloudflare and writes certificates directly to a host-mounted Hetzner Storage Box path (`/mnt/storagebox/certs` → `/etc/letsencrypt`). No rclone sync or container-side mounting is required.

## What changed
- Host handles the SMB3 mount; the container just consumes `/etc/letsencrypt` as a bind mount.
- Only the Cloudflare API token secret is needed.
- Minimal environment: email, domains, interval, optional dry-run.

## Prerequisites
- Docker Swarm with external network `web-net` (as used in `docker-compose.swarm.yml`).
- Host SMB3 mount present at `/mnt/storagebox` (see `FSTAB_SETUP.md`). Verify with `mount | grep storagebox`.
- Docker secret `cloudflare_api_token` whose content is a single line: `dns_cloudflare_api_token = <your-token>`.

## Deploy (TL;DR)
1) Ensure the host mount exists and contains `certs/`:
```bash
sudo mkdir -p /mnt/storagebox/certs
mount | grep storagebox
```
2) Create the Cloudflare secret:
```bash
printf 'dns_cloudflare_api_token = %s\n' "$CF_API_TOKEN" | docker secret create cloudflare_api_token -
```
3) Adjust `docker-compose.swarm.yml` if needed:
- `CERT_EMAIL`, `CERT_DOMAINS`
- `RENEW_INTERVAL` (default `12h`)
- `CERTBOT_DRY_RUN` ("true" to use Let’s Encrypt staging)

4) Deploy:
```bash
docker stack deploy -c docker-compose.swarm.yml certbot
```
5) Check logs/health:
```bash
docker service logs -f certbot_certbot
```

## Configuration
| Variable | Default | Description |
| --- | --- | --- |
| `CERT_EMAIL` | `admin@example.com` | Email for Let’s Encrypt notices |
| `CERT_DOMAINS` | `example.com,*.example.com` | Comma-separated domains (wildcards ok) |
| `RENEW_INTERVAL` | `12h` | Renewal check interval (`s/m/h/d`) |
| `CERTBOT_DRY_RUN` | `false` | `true` uses Let’s Encrypt staging |
| `CLOUDFLARE_CREDENTIALS_FILE` | `/run/secrets/cloudflare_api_token` | Path to the token (usually unchanged) |
| `DEBUG` | `false` | Extra logging |

## Paths
- Host: `/mnt/storagebox/certs`
- Container: `/etc/letsencrypt` (bind from host)
- Nginx (example): bind the same host path read-only to your nginx container, e.g. `/etc/nginx/certs`.

## Health & maintenance
- Healthcheck verifies the mount and certificate expiry for the first domain in `CERT_DOMAINS`.
- Manual renew: `docker exec $(docker ps -qf "name=certbot_certbot") certbot renew --force-renewal`
- Restart service: `docker service update --force certbot_certbot`

## Troubleshooting
- **Mount warning in logs**: The container cannot see `/etc/letsencrypt`. Confirm the host bind `- /mnt/storagebox/certs:/etc/letsencrypt:rshared` and that the host mount is active.
- **Cloudflare errors**: Check token permissions (`Zone:DNS:Edit`, `Zone:Zone:Read`).
- **No cert issued**: Ensure DNS is managed by Cloudflare and domains match `CERT_DOMAINS`.

## References
- Host mount how-to: `FSTAB_SETUP.md`
- Deploy manifest: `docker-compose.swarm.yml`
- Image details: `Dockerfile`
