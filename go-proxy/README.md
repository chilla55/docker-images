# Go Reverse Proxy

A high-performance, production-ready HTTP/HTTPS/HTTP3 reverse proxy written in pure Go. Designed for Docker Swarm deployments with enterprise-grade features including circuit breakers, rate limiting, WAF, GDPR compliance, and comprehensive observability.

## ✨ Features

### Core Proxy
- **HTTP/1.1, HTTP/2, HTTP/3** - Full protocol support including QUIC
- **Automatic HTTPS** - TLS termination with Let's Encrypt integration
- **WebSocket Support** - Native WebSocket proxying with configurable timeouts
- **Dynamic Configuration** - YAML-based configs with live reloading (no restarts)
- **Service Registry** - Dynamic backend registration on port 81

### Reliability & Performance
- **Circuit Breakers** - Automatic failure detection and recovery
- **Connection Pooling** - Efficient HTTP connection management
- **Retry Logic** - Configurable retry with exponential backoff
- **Health Checks** - Active upstream health monitoring
- **Graceful Shutdown** - Zero-downtime deployments with connection draining

### Security
- **WAF** (Web Application Firewall) - SQL injection, XSS, path traversal protection
- **Rate Limiting** - Per-IP and per-route request throttling
- **GDPR Compliance** - PII masking with configurable IP anonymization
- **Security Headers** - HSTS, CSP, X-Frame-Options, etc.
- **GeoIP Filtering** - Country-based access control with alerts

### Observability
- **Metrics** - Prometheus-compatible metrics on `:8080/metrics`
- **Access Logs** - Structured JSON logging with SQLite persistence
- **Certificate Monitoring** - Automatic expiry alerts (30/7/1 days)
- **Health Dashboard** - Real-time status on `:8080/health`
- **Traffic Analytics** - Request/response time analysis, error rates
- **Webhook Alerts** - Discord/Slack notifications for incidents

### Operations
- **Automated Backups** - Full, differential, and incremental SQLite backups
- **Data Retention** - Configurable retention policies (GDPR-compliant)
- **Audit Logging** - Compliance-ready security event tracking
- **Maintenance Mode** - Per-service maintenance pages
- **Compression** - Brotli and Gzip with intelligent content-type detection

## 🚀 Quick Start

### 1. Create Global Configuration

```yaml
# global.yaml
defaults:
  headers:
    Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
    X-Frame-Options: DENY
    X-Content-Type-Options: nosniff
  
  options:
    health_check_interval: 30s
    timeout: 30s
    compression: true
    http2: true
    http3: true

blackhole:
  unknown_domains: true

tls:
  certificates:
    - domains:
        - "*.example.com"
        - "example.com"
      cert_file: /etc/proxy/certs/example.com/fullchain.pem
      key_file: /etc/proxy/certs/example.com/privkey.pem
```

### 2. Create Site Configuration

```yaml
# sites-available/myapp.yaml
enabled: true

service:
  name: myapp
  maintenance_port: 8081

routes:
  - domains:
      - myapp.example.com
    path: /
    backend: http://myapp:8080

options:
  health_check_path: /health
  timeout: 30s
  
  circuit_breaker:
    enabled: true
    failure_threshold: 5
    timeout: 30s
  
  rate_limit:
    enabled: true
    requests_per_min: 60
    per_ip: true
```

### 3. Run with Docker

```bash
docker run -d \
  --name go-proxy \
  -p 80:80 \
  -p 443:443 \
  -p 8080:8080 \
  -v /path/to/certs:/etc/proxy/certs:ro \
  -v /path/to/sites:/etc/proxy/sites-available:ro \
  -v /path/to/global.yaml:/etc/proxy/global.yaml:ro \
  -v proxy-data:/data \
  ghcr.io/chilla55/go-proxy:latest
```

## 📖 Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SITES_PATH` | `/etc/proxy/sites-available` | Site configuration directory |
| `GLOBAL_CONFIG` | `/etc/proxy/global.yaml` | Global config file path |
| `DB_PATH` | `/data/proxy.db` | SQLite database location |
| `BACKUP_DIR` | `/mnt/storagebox/backups/proxy` | Backup destination |
| `HTTP_ADDR` | `:80` | HTTP listen address |
| `HTTPS_ADDR` | `:443` | HTTPS listen address |
| `HEALTH_PORT` | `8080` | Health/metrics server port |
| `REGISTRY_PORT` | `81` | Service registry port |
| `DEBUG` | `0` | Enable debug logging (1=on) |
| `TZ` | `UTC` | Timezone for logs |

### Site Configuration Options

Complete configuration reference: [CONFIGURATION.md](CONFIGURATION.md)

**Route Options:**
- `domains` - List of domain names
- `path` - URL path prefix
- `backend` - Upstream server URL
- `websocket` - Enable WebSocket support
- `headers` - Custom response headers

**Advanced Features:**
- `timeouts` - Connect, read, write, idle timeouts
- `circuit_breaker` - Failure detection and recovery
- `rate_limit` - Request throttling per IP/route
- `waf` - Web Application Firewall rules
- `pii` - GDPR-compliant data masking
- `geoip` - Geographic access control
- `compression` - Brotli/Gzip configuration
- `retry` - Automatic retry with backoff
- `connection_pool` - HTTP connection tuning

## 🔧 Service Registry

Backends can dynamically register/deregister routes via HTTP on port `81`:

### Register Route
```bash
curl -X POST http://proxy:81/register \
  -H "Content-Type: application/json" \
  -d '{
    "host": "api.example.com",
    "path": "/v2",
    "backend": "http://api-v2:9000",
    "options": {
      "timeout": "60s",
      "websocket": true
    }
  }'
```

### Deregister Route
```bash
curl -X POST http://proxy:81/deregister \
  -H "Content-Type: application/json" \
  -d '{
    "host": "api.example.com",
    "path": "/v2"
  }'
```

## 📊 Monitoring

### Health Check
```bash
curl http://localhost:8080/health
```

### Prometheus Metrics
```bash
curl http://localhost:8080/metrics
```

**Available Metrics:**
- `proxy_requests_total` - Total request count
- `proxy_request_duration_seconds` - Request latency histogram
- `proxy_backend_errors_total` - Backend error count
- `proxy_circuit_breaker_state` - Circuit breaker status
- `proxy_active_connections` - Current active connections
- `proxy_certificate_expiry_days` - Certificate expiration time

### Logs
All logs are structured JSON written to stdout and SQLite database:

```bash
docker logs -f go-proxy
```

Access log table: `access_logs` (30-day retention)  
Security events: `security_events` (90-day retention)  
Audit trail: `audit_log` (365-day retention)

## 🔐 Security Best Practices

1. **Enable WAF** - Protect against common web attacks
2. **Rate Limiting** - Prevent abuse and DDoS
3. **GDPR Compliance** - Mask PII in logs (IPs, headers)
4. **Certificate Monitoring** - Set up webhook alerts for expiry
5. **GeoIP Filtering** - Block unexpected geographic regions
6. **Security Headers** - Use strong defaults from global config
7. **Audit Logging** - Enable for compliance requirements

## 📦 Automated Backups

Built-in SQLite backup system with cron scheduling:

- **Full Backup** - Every Sunday at 01:00 UTC
- **Differential** - Every Wednesday at 02:00 UTC
- **Incremental** - Every hour on the hour
- **Retention Cleanup** - Daily at 03:00 UTC

Backups stored in `BACKUP_DIR` with automatic rotation.

## 🐳 Docker Swarm Deployment

See [DEPLOYMENT.md](DEPLOYMENT.md) for complete Docker Swarm setup including:
- Stack deployment with compose file
- Volume mount configuration
- Health check integration
- Rolling updates
- Troubleshooting

## 🛠️ Development

### Build from Source
```bash
cd proxy-manager
go build -o proxy-manager .
./proxy-manager --help
```

### Run Tests
```bash
cd proxy-manager
go test -v ./...
```

### Generate Coverage
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## 📋 Examples

Check the `sites-available/` directory for example configurations:
- `example-simple.yaml` - Basic single-domain site
- `example-multi-domain.yaml` - Multi-domain with wildcard certs
- `example-api-gateway.yaml` - API gateway with WebSocket

## 🤝 Contributing

This is a private project for production use. For issues or feature requests, contact the maintainer.

## 📄 License

Proprietary - All rights reserved.

## 🔗 Related Projects

- **nginx** - Traditional nginx container (legacy, being phased out)
- **certbot** - Let's Encrypt certificate automation
- **mariadb** - Database service with automated backups
- **postgresql** - PostgreSQL with backup strategy
- **redis** - Redis cache service

---

**Built with Go 1.22** | **Production-ready since 2025** | **Zero-downtime deployments**
