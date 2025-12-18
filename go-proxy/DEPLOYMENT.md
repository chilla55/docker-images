# Docker Swarm Deployment Guide

Complete guide for deploying the Go reverse proxy on Docker Swarm.

## Prerequisites

- Docker Swarm initialized (`docker swarm init`)
- Overlay network created (`docker network create --driver overlay web-net`)
- Storage Box or NFS mount for certificates/configs at `/mnt/storagebox`
- Node labels configured for placement constraints

## Quick Deployment

### 1. Setup Node Labels

Label your web-facing node:

```bash
docker node update --label-add web.node=web <node-name>
```

### 2. Prepare Configuration

Create directory structure on Storage Box:

```bash
mkdir -p /mnt/storagebox/{certs,sites,backups/proxy}
```

Copy your configurations:

```bash
# Global config
cp global.yaml /mnt/storagebox/global.yaml

# Site configs
cp sites-available/*.yaml /mnt/storagebox/sites/

# Certificates (from Let's Encrypt)
cp -r /etc/letsencrypt/live/example.com /mnt/storagebox/certs/
```

### 3. Deploy Stack

```bash
docker stack deploy -c docker-compose.swarm.yml proxy
```

### 4. Verify Deployment

```bash
# Check service status
docker service ls | grep proxy

# View logs
docker service logs -f proxy_proxy

# Test health endpoint
curl http://localhost:8080/health
```

---

## docker-compose.swarm.yml

Complete production-ready compose file:

```yaml
version: '3.8'

services:
  proxy:
    image: ghcr.io/chilla55/go-proxy:latest
    networks:
      - web-net
    
    deploy:
      mode: replicated
      replicas: 1
      placement:
        constraints:
          - node.labels.web.node == web
      update_config:
        parallelism: 1
        delay: 10s
        order: start-first
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
      resources:
        limits:
          cpus: '2'
          memory: 8G
        reservations:
          cpus: '1'
          memory: 4G
    
    ports:
      # HTTP
      - target: 80
        published: 80
        protocol: tcp
        mode: host
      
      # HTTPS (TCP)
      - target: 443
        published: 443
        protocol: tcp
        mode: host
      
      # HTTPS (UDP for HTTP/3)
      - target: 443
        published: 443
        protocol: udp
        mode: host
      
      # Service Registry
      - target: 81
        published: 81
        protocol: tcp
        mode: host
      
      # Health/Metrics
      - target: 8080
        published: 8080
        protocol: tcp
        mode: host
    
    environment:
      DEBUG: "0"
      SITES_PATH: "/etc/proxy/sites-available"
      GLOBAL_CONFIG: "/etc/proxy/global.yaml"
      DB_PATH: "/data/proxy.db"
      BACKUP_DIR: "/mnt/storagebox/backups/proxy"
      TZ: "UTC"
    
    configs:
      - source: proxy_global_config
        target: /etc/proxy/global.yaml
        mode: 0444
    
    volumes:
      # Mount Let's Encrypt certificates with rslave propagation
      - type: bind
        source: /mnt/storagebox/certs
        target: /etc/proxy/certs
        read_only: true
        bind:
          propagation: rslave
      
      # Mount site configurations with rslave propagation
      - type: bind
        source: /mnt/storagebox/sites
        target: /etc/proxy/sites-available
        read_only: true
        bind:
          propagation: rslave
      
      # Persist SQLite DB
      - type: volume
        source: proxy-data
        target: /data

      # Backups directory (bind from Storage Box)
      - type: bind
        source: /mnt/storagebox/backups/proxy
        target: /mnt/storagebox/backups/proxy
        bind:
          propagation: rslave
    
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s

configs:
  proxy_global_config:
    file: ./global.yaml

networks:
  web-net:
    external: true

volumes:
  proxy-data:
```

---

## Volume Configuration

### Bind Propagation

Using `rslave` propagation allows certificate renewals without container restarts:

```yaml
bind:
  propagation: rslave
```

When certbot renews certificates in `/mnt/storagebox/certs`, the proxy container automatically sees the updated files.

### Read-Only Mounts

Security best practice - mount configs read-only:

```yaml
read_only: true
```

Only `/data` (SQLite) and `/mnt/storagebox/backups` are writable.

### Volume Persistence

The `proxy-data` volume persists:
- SQLite database (`proxy.db`)
- Access logs
- Security events
- Metrics history

**Backup Recommendation:** Include `proxy-data` in backup strategy.

---

## Storage Box Setup

Recommended directory structure:

```
/mnt/storagebox/
├── certs/
│   ├── example.com/
│   │   ├── fullchain.pem
│   │   └── privkey.pem
│   └── *.example.com/
│       ├── fullchain.pem
│       └── privkey.pem
├── sites/
│   ├── app1.yaml
│   ├── app2.yaml
│   └── api.yaml
├── backups/
│   └── proxy/
│       ├── full/
│       ├── differential/
│       └── incremental/
└── global.yaml
```

### NFS Mount (Recommended)

Mount Storage Box via NFS for better performance:

```bash
# /etc/fstab
storagebox.example.com:/storage /mnt/storagebox nfs defaults,_netdev 0 0
```

### SSHFS Alternative

```bash
sshfs user@storagebox:/storage /mnt/storagebox \
  -o allow_other,default_permissions,uid=1000,gid=1000
```

---

## Deployment Operations

### Rolling Updates

Zero-downtime updates with start-first strategy:

```bash
# Update to new image version
docker service update \
  --image ghcr.io/chilla55/go-proxy:v2.0.0 \
  proxy_proxy
```

**Process:**
1. Start new container
2. Wait for health check to pass
3. Stop old container
4. Cleanup

### Force Restart

Restart without changing image:

```bash
docker service update --force proxy_proxy
```

### Rollback

Revert to previous version:

```bash
docker service rollback proxy_proxy
```

### Scale

Add more replicas (behind load balancer):

```bash
docker service scale proxy_proxy=3
```

**Note:** Shared SQLite limits scaling. For >1 replica, use external PostgreSQL for logs/metrics.

---

## Configuration Updates

### Adding a New Site

1. Create site config:
```bash
vim /mnt/storagebox/sites/newapp.yaml
```

2. Config is automatically reloaded (no restart needed)

3. Verify in logs:
```bash
docker service logs proxy_proxy | grep "Loaded site"
```

### Updating Global Config

Global config requires restart:

```bash
# Edit config
vim /mnt/storagebox/global.yaml

# Restart service
docker service update --force proxy_proxy
```

### Certificate Renewal

Certificates auto-reload with `rslave` propagation:

```bash
# Certbot renews certificate
certbot renew

# Proxy automatically detects new cert
# Check logs for reload confirmation
docker service logs proxy_proxy | grep "certificate"
```

---

## Health Checks

### Container Health

Swarm uses built-in healthcheck:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 10s
```

### Health Endpoint

Check proxy status:

```bash
curl http://localhost:8080/health
```

**Response:**
```json
{
  "status": "healthy",
  "uptime": "72h15m30s",
  "sites_loaded": 12,
  "active_connections": 45,
  "circuit_breakers": {
    "open": 0,
    "half_open": 0
  }
}
```

### Metrics Endpoint

Prometheus metrics:

```bash
curl http://localhost:8080/metrics
```

**Key Metrics:**
- `proxy_requests_total`
- `proxy_request_duration_seconds`
- `proxy_active_connections`
- `proxy_backend_errors_total`
- `proxy_circuit_breaker_state`
- `proxy_certificate_expiry_days`

---

## Monitoring Integration

### Prometheus

Scrape configuration:

```yaml
scrape_configs:
  - job_name: 'go-proxy'
    static_configs:
      - targets: ['proxy-node:8080']
    metrics_path: /metrics
    scrape_interval: 15s
```

### Grafana Dashboard

Import dashboard for visualization:

**Metrics:**
- Request rate (req/s)
- Response time (p50, p95, p99)
- Error rate (%)
- Backend health status
- Circuit breaker states
- Certificate expiry countdown

### Webhook Alerts

Configure Discord/Slack notifications in `global.yaml`:

```yaml
webhook:
  url: "https://discord.com/api/webhooks/..."
  enabled: true
```

**Alert Triggers:**
- Certificate expiring (30/7/1 days)
- Backend down
- High error rate (>10% over 5min)
- Circuit breaker open
- GeoIP unusual access
- WAF attack detected

---

## Troubleshooting

### Service Won't Start

Check constraints:

```bash
# Verify node has correct label
docker node inspect <node-name> | grep web.node

# Check service placement
docker service ps proxy_proxy
```

### Certificate Errors

Verify certificate files:

```bash
# Check cert permissions
ls -la /mnt/storagebox/certs/example.com/

# Test certificate validity
openssl x509 -in /mnt/storagebox/certs/example.com/fullchain.pem -text -noout

# Check logs
docker service logs proxy_proxy | grep -i certificate
```

### Config Not Reloading

```bash
# Check file watcher
docker service logs proxy_proxy | grep watcher

# Verify file permissions
ls -la /mnt/storagebox/sites/

# Manual reload (force restart)
docker service update --force proxy_proxy
```

### High Memory Usage

```bash
# Check current usage
docker stats $(docker ps -q -f name=proxy)

# Increase limits in compose file
resources:
  limits:
    memory: 16G
```

### Database Locked

SQLite lock issues with multiple replicas:

```bash
# Scale to 1 replica only
docker service scale proxy_proxy=1

# Or migrate to PostgreSQL for multi-replica
```

### Port Conflicts

```bash
# Check port bindings
docker service inspect proxy_proxy | grep PublishedPort

# Verify nothing else uses ports 80/443
netstat -tlnp | grep -E ':(80|443) '
```

---

## Backup Strategy

### Automated Backups

Built-in cron jobs:

- **Full** - Sunday 01:00 UTC
- **Differential** - Wednesday 02:00 UTC
- **Incremental** - Every hour
- **Cleanup** - Daily 03:00 UTC

Backups stored in `BACKUP_DIR=/mnt/storagebox/backups/proxy`

### Manual Backup

```bash
# Access container
docker exec -it $(docker ps -q -f name=proxy_proxy) sh

# Run backup scripts
/usr/local/bin/backup-full.sh
```

### Restore from Backup

```bash
# Stop service
docker service scale proxy_proxy=0

# Restore database
docker run --rm \
  -v proxy-data:/data \
  -v /mnt/storagebox/backups/proxy:/backups \
  alpine sh -c "cp /backups/full/latest.db /data/proxy.db"

# Restart service
docker service scale proxy_proxy=1
```

---

## Security Hardening

### Firewall Rules

Allow only necessary ports:

```bash
# Allow HTTP/HTTPS
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 443/udp

# Health check (internal only)
ufw allow from 10.0.0.0/8 to any port 8080

# Service registry (internal only)
ufw allow from 10.0.0.0/8 to any port 81
```

### Docker Secrets (Alternative)

Use secrets instead of configs for sensitive data:

```yaml
secrets:
  - source: tls_cert
    target: /run/secrets/tls_cert
  - source: tls_key
    target: /run/secrets/tls_key
```

### Read-Only Root Filesystem

Enhance security with read-only rootfs:

```yaml
deploy:
  read_only: true
  tmpfs:
    - /tmp
```

---

## Production Checklist

- [ ] Node labels configured
- [ ] Network overlay created
- [ ] Storage Box mounted and tested
- [ ] Certificates in place and valid
- [ ] Global config validated
- [ ] Site configs tested
- [ ] Health check passing
- [ ] Metrics endpoint accessible
- [ ] Webhook alerts configured
- [ ] Firewall rules applied
- [ ] Backup cron jobs verified
- [ ] Monitoring integrated
- [ ] Log retention configured
- [ ] Resource limits tuned
- [ ] Documentation updated

---

## Support

For issues or questions, review logs:

```bash
# Real-time logs
docker service logs -f proxy_proxy

# Error logs only
docker service logs proxy_proxy 2>&1 | grep -i error

# Last 100 lines
docker service logs --tail 100 proxy_proxy
```

Check health status:
```bash
curl http://localhost:8080/health
```

Review metrics:
```bash
curl http://localhost:8080/metrics | grep proxy_
```
