# Docker Swarm Deployment Guide

Quick guide for deploying the Go proxy to Docker Swarm with certificates from storagebox.

## Prerequisites

1. **Certificates on Storagebox**: `/mnt/storagebox/certs/chilla55.de/`
   - `fullchain.pem` (certificate + chain)
   - `privkey.pem` (private key)
   - `cert.pem` (not used)
   - `chain.pem` (not used)

2. **Node Label**: `node.labels.web.node == web`
   ```bash
   docker node update --label-add web.node=web <node-name>
   ```

3. **Network**: `web-net` must exist
   ```bash
   docker network create --driver overlay --attachable web-net
   ```

## Configuration

### 1. Update global.yaml

```yaml
tls:
  certificates:
    - domains:
        - "*.chilla55.de"
        - "chilla55.de"
      cert_file: /etc/proxy/certs/chilla55.de/fullchain.pem
      key_file: /etc/proxy/certs/chilla55.de/privkey.pem
```

### 2. Verify Storagebox Mount

Ensure storagebox is mounted with proper propagation on the swarm node:

```bash
ssh srv1
mount | grep storagebox
# Should show: /mnt/storagebox type cifs ...
```

## Build and Push

```bash
# Build image
docker build -t ghcr.io/chilla55/nginx:latest .

# Push to registry
docker push ghcr.io/chilla55/nginx:latest
```

## Deploy

```bash
# Deploy stack
docker stack deploy -c docker-compose.swarm.yml nginx

# Check services
docker service ls | grep nginx

# Check logs
docker service logs -f nginx_nginx

# You should see:
# [proxy-manager] Loaded certificate for domains: [*.chilla55.de chilla55.de]
# [proxy-manager] Loaded 1 TLS certificate(s)
# [proxy-manager] All services started successfully
```

## Verify Deployment

```bash
# Check health
curl http://srv1:8080/health
# Expected: healthy

# Check metrics
curl http://srv1:8080/metrics

# Test HTTPS with your domain
curl -I https://gpanel.chilla55.de
# Expected: HTTP/2 200
```

## Certificate Paths in Container

When deployed, certificates are mounted:
- **Host**: `/mnt/storagebox/certs/chilla55.de/fullchain.pem`
- **Container**: `/etc/proxy/certs/chilla55.de/fullchain.pem`

The `rslave` propagation ensures certificate renewals on the host are visible in the container.

## Site Configuration

Place YAML site configs in `/mnt/storagebox/sites/`:

```yaml
# /mnt/storagebox/sites/gpanel.yaml
enabled: true

service:
  name: gpanel

routes:
  - domains:
      - gpanel.chilla55.de
    path: /
    backend: http://gpanel:8080
```

The watcher will automatically detect and load new configurations.

## Ports Exposed

- **80/tcp**: HTTP (redirects to HTTPS)
- **443/tcp**: HTTPS (HTTP/2)
- **443/udp**: HTTP/3 (QUIC)
- **81/tcp**: Service Registry (dynamic routes)
- **8080/tcp**: Health/Metrics

## Update/Rollback

```bash
# Update service (rolling update)
docker service update --image ghcr.io/chilla55/nginx:latest nginx_nginx

# Force update (restart all)
docker service update --force nginx_nginx

# Rollback
docker service rollback nginx_nginx
```

## Troubleshooting

### Certificate Not Loading

```bash
# Check if certs are accessible in container
docker exec $(docker ps -q -f name=nginx_nginx) \
  ls -la /etc/proxy/certs/chilla55.de/

# Should show fullchain.pem and privkey.pem
```

### Service Won't Start

```bash
# Check logs for errors
docker service logs nginx_nginx | grep -i error

# Common issues:
# - Certificate files not found
# - Invalid YAML syntax in global.yaml
# - Network not created
# - Node label missing
```

### Certificate Renewal

After renewing Let's Encrypt certificates:

```bash
# Certificates auto-renewed by Certbot on host
# The proxy automatically detects and reloads them!
# No manual intervention needed.

# Check logs to confirm reload
docker service logs nginx_nginx | grep cert-watcher
```

Expected output:
```
[cert-watcher] Certificate file changed: /etc/proxy/certs/chilla55.de/fullchain.pem
[cert-watcher] Reloading certificates from disk...
[cert-watcher] Successfully reloaded 1 certificate(s)
[proxy] Certificates updated: 1 certificate(s) loaded
```

**No restart required!** The certificate watcher automatically detects renewals and hot-reloads them with zero downtime.

## Migration from Old Nginx

If migrating from the old nginx setup:

1. ✅ Certificates are already in `/mnt/storagebox/certs/` (no change needed)
2. ✅ Sites will need converting from nginx conf to YAML
3. ✅ Service registry protocol changed (ROUTE/HEADER/OPTIONS)
4. ✅ Port 81 is now service registry (was maintenance port)

## Performance Notes

- **HTTP/3**: Enabled by default on UDP/443
- **HTTP/2**: Enabled by default on TCP/443
- **Resources**: 1-2 CPU, 4-8GB RAM allocated
- **Placement**: Pinned to `web.node` label

## Next Steps

1. Convert existing nginx site configs to YAML format
2. Update services to use new registry protocol (port 81)
3. Test domains one by one
4. Monitor metrics on port 8080
