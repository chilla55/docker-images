# Nginx Docker Image for Swarm

Custom-built Nginx with Brotli, HTTP/3, and Cloudflare Real IP support.

## Features

- **Nginx**: Latest mainline (configurable via VERSION file)
- **Modules**: Brotli, Headers-More, Cache Purge, HTTP/3, HTTP/2
- **Cloudflare Integration**: Auto-updating real IP configuration
- **Certificate Watcher**: Auto-reload on cert changes
- **Sites Watcher**: Auto-reload when site configs change
- **Docker Swarm Ready**: Overlay networks, configs, health checks
- **Host Bind Mounts**: Simple integration with host storagebox mounts
- **Small Image**: Alpine-based (~50MB)

## Quick Start

### Prerequisites

Ensure the host has storagebox mounted:
```bash
# Host should have these paths ready:
/mnt/storagebox/certs/       # SSL certificates
/mnt/storagebox/sites/       # Nginx site configurations
```

### Build
```bash
make build
```

### Deploy to Swarm
```bash
# Create external network
docker network create --driver overlay web-net

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

### Storage Box Setup

See **STORAGEBOX_SETUP.md** for detailed configuration of certificate and site paths.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CF_REALIP_AUTO` | `true` | Enable Cloudflare IP auto-update |
| `CF_REALIP_INTERVAL` | `21600` | Update interval (6 hours) |
| `CERT_WATCH_PATH` | `""` | Path to certificate to watch |
| `CERT_WATCH_INTERVAL` | `300` | Cert check interval (5 min) |
| `CERT_WATCH_DEBUG` | `0` | Enable cert watcher debug logging |
| `SITES_WATCH_PATH` | `/etc/nginx/sites-enabled` | Sites directory to watch |
| `SITES_WATCH_INTERVAL` | `30` | Sites check interval (seconds) |
| `SITES_WATCH_DEBUG` | `0` | Enable sites watcher debug logging |

## Site Configurations

Site configs are automatically mounted from `/mnt/storagebox/sites` on the host.

Example site config at `/mnt/storagebox/sites/example.com.conf`:
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

Changes to site configs are detected automatically and nginx reloads (with config validation).

## Health Checks

The healthcheck monitors:
- Nginx process is running and responding
- Configuration is valid
- SSL certificates are properly configured

## Updates

To update Nginx version:
1. Edit `VERSION` file
2. Run `make build`
3. Run `make push`
4. Run `docker service update --image ghcr.io/chilla55/nginx:latest nginx_nginx`

## Watchers

Two background watchers automatically detect and handle changes:

### Certificate Watcher
- Monitors certificate file for changes
- Validates and reloads nginx on certificate update
- Useful for Let's Encrypt renewals
- Controlled via `CERT_WATCH_PATH` environment variable

### Sites Watcher  
- Monitors `/etc/nginx/sites-enabled` directory for changes
- Validates config before reloading
- Prevents bad configs from crashing nginx
- Automatically detects new/modified/deleted site files
- Controlled via `SITES_WATCH_PATH` and `SITES_WATCH_INTERVAL`
└─────────────────────────────────────┘
```

## License

See repository LICENSE file.
