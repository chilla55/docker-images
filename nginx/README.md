# Nginx Docker Image for Swarm

Custom-built Nginx with Brotli, HTTP/3, and Cloudflare Real IP support.

## Features

- **Nginx**: Latest mainline (configurable via VERSION file)
- **Modules**: Brotli, Headers-More, Cache Purge, HTTP/3, HTTP/2
- **Cloudflare Integration**: Auto-updating real IP configuration
- **Certificate Watcher**: Auto-reload on cert changes
- **Docker Swarm Ready**: Overlay networks, configs, health checks
- **Small Image**: Alpine-based (~50MB)

## Quick Start

### Build
```bash
make build
```

### Deploy to Swarm
```bash
# Create external network
docker network create --driver overlay proxy

# Create nginx config
docker config create nginx_main_config nginx.conf

# Deploy stack
make deploy
```

## Configuration

### VERSION File
Controls the Nginx version to build:
```
1.29.0
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CF_REALIP_AUTO` | `true` | Enable Cloudflare IP auto-update |
| `CF_REALIP_INTERVAL` | `21600` | Update interval (6 hours) |
| `CF_REALIP_MAX_FAILS` | `5` | Max failures before unhealthy |
| `CERT_WATCH_PATH` | `""` | Path to certificate to watch |
| `CERT_WATCH_INTERVAL` | `300` | Cert check interval (5 min) |
| `CERT_WATCH_DEBUG` | `0` | Enable debug logging |

## Site Configurations

Mount your site configs to `/etc/nginx/sites-enabled/`:

```yaml
volumes:
  - ./sites:/etc/nginx/sites-enabled:ro
```

Example site config:
```nginx
server {
    listen 80;
    server_name example.com;

    location / {
        proxy_pass http://backend:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

## Health Checks

The healthcheck monitors:
- Nginx process running
- Cloudflare IP update status
- Update staleness/failures

## Updates

To update Nginx version:
1. Edit `VERSION` file
2. Run `make build`
3. Run `make push`
4. Run `docker service update --image ghcr.io/chilla55/nginx:latest nginx_nginx`

## Architecture

```
┌─────────────────────────────────────┐
│  Docker Swarm Stack (nginx)         │
├─────────────────────────────────────┤
│                                     │
│  ┌─────────────────────────────┐   │
│  │  Nginx Service (replicas=2)  │   │
│  │  - Port 80/443               │   │
│  │  - Cloudflare Real IP        │   │
│  │  - Certificate Watcher       │   │
│  └─────────────────────────────┘   │
│           │                         │
│           └─── Overlay Network      │
│                (proxy)              │
└─────────────────────────────────────┘
```

## License

See repository LICENSE file.
