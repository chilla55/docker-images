# Configuration Reference

Complete guide to configuring the Go reverse proxy.

## Table of Contents

- [Global Configuration](#global-configuration)
- [Site Configuration](#site-configuration)
- [Environment Variables](#environment-variables)
- [Advanced Options](#advanced-options)
- [Examples](#examples)

## Global Configuration

The `global.yaml` file defines system-wide defaults and TLS certificates.

### Structure

```yaml
defaults:
  headers: {}      # Default HTTP headers for all sites
  options: {}      # Default options applied to all routes

blackhole:
  unknown_domains: bool    # Reject requests for undefined domains
  metrics_only: bool       # Track but don't log blackholed requests

tls:
  certificates: []         # SSL certificate configurations

webhook:
  url: string             # Webhook URL for alerts (Discord/Slack)
  enabled: bool           # Enable webhook notifications
```

### Defaults Section

#### Headers
Security headers applied to all responses unless overridden:

```yaml
defaults:
  headers:
    Strict-Transport-Security: "max-age=31536000; includeSubDomains; preload"
    X-Frame-Options: "DENY"
    X-Content-Type-Options: "nosniff"
    X-XSS-Protection: "1; mode=block"
    Referrer-Policy: "strict-origin-when-cross-origin"
    Permissions-Policy: "geolocation=(), microphone=(), camera=()"
    Content-Security-Policy: "default-src 'self'"
```

#### Options
Default behavior for all sites:

```yaml
defaults:
  options:
    health_check_interval: 30s
    health_check_timeout: 5s
    timeout: 30s
    max_body_size: 100M
    compression: true
    http2: true
    http3: true
```

### TLS Certificates

Configure SSL certificates (supports wildcards):

```yaml
tls:
  certificates:
    # Wildcard certificate
    - domains:
        - "*.example.com"
        - "example.com"
      cert_file: /etc/proxy/certs/example.com/fullchain.pem
      key_file: /etc/proxy/certs/example.com/privkey.pem
    
    # Specific subdomain
    - domains:
        - "api.example.com"
      cert_file: /etc/proxy/certs/api.example.com/fullchain.pem
      key_file: /etc/proxy/certs/api.example.com/privkey.pem
```

### Blackhole Configuration

Control behavior for unmapped domains:

```yaml
blackhole:
  unknown_domains: true    # Return 404 for unknown domains
  metrics_only: true       # Don't log blackholed requests (reduce noise)
```

### Webhook Alerts

Configure incident notifications:

```yaml
webhook:
  url: "https://discord.com/api/webhooks/..."
  enabled: true
```

**Alert Types:**
- Certificate expiring (30/7/1 days)
- Backend health failures
- High error rates (>10% over 5 minutes)
- Circuit breaker trips
- GeoIP unusual access

---

## Site Configuration

Per-site YAML files in `sites-available/` directory.

### Basic Structure

```yaml
enabled: bool               # Enable/disable site

service:
  name: string             # Service identifier
  maintenance_port: int    # Maintenance mode port (optional)

routes: []                 # Route definitions
headers: {}                # Custom response headers
options: {}                # Site-specific options
```

### Routes

Define URL routing rules:

```yaml
routes:
  - domains:              # List of domain names
      - example.com
      - www.example.com
    path: /               # URL path prefix
    backend: http://app:8080  # Upstream server
    websocket: false      # Enable WebSocket
    headers: {}           # Route-specific headers
```

**Path Matching:**
- `/` - Matches all paths
- `/api` - Matches `/api/*`
- `/api/v1` - Matches `/api/v1/*`

Longest prefix wins for overlapping paths.

### Headers

Custom response headers (merged with global defaults):

```yaml
headers:
  X-Custom-Header: "value"
  Access-Control-Allow-Origin: "*"
  Cache-Control: "public, max-age=3600"
```

---

## Advanced Options

All options are configured under `options:` in site config.

### Timeouts

```yaml
options:
  timeouts:
    connect: 5s          # Backend connection timeout
    read: 30s            # Read timeout
    write: 30s           # Write timeout
    idle: 120s           # Idle connection timeout
```

### Circuit Breaker

Automatic failure detection and recovery:

```yaml
options:
  circuit_breaker:
    enabled: true
    failure_threshold: 5      # Failures before opening
    success_threshold: 2      # Successes before closing
    timeout: 30s              # Stay open duration
    window: 60s               # Failure tracking window
```

**States:**
- **Closed** - Normal operation
- **Open** - All requests fail fast
- **Half-Open** - Testing recovery

### Rate Limiting

Throttle requests to prevent abuse:

```yaml
options:
  rate_limit:
    enabled: true
    requests_per_min: 60      # Per-minute limit
    requests_per_hour: 1000   # Per-hour limit
    burst_size: 10            # Burst allowance
    per_ip: true              # Apply per source IP
    per_route: false          # Apply per route
    whitelist:                # Exempt IPs
      - 10.0.0.0/8
      - 192.168.0.0/16
```

### WAF (Web Application Firewall)

Protect against common attacks:

```yaml
options:
  waf:
    enabled: true
    block_mode: true          # true=block, false=log only
    sensitivity: medium       # low, medium, high
    check_path: true          # Scan URL paths
    check_headers: true       # Scan HTTP headers
    check_query: true         # Scan query parameters
    check_body: true          # Scan request body
    max_body_size: 1048576    # Max body size to inspect (bytes)
    whitelist:                # Trusted IPs
      - 10.0.0.0/8
```

**Detects:**
- SQL injection attempts
- XSS (Cross-Site Scripting)
- Path traversal (../)
- Command injection
- LDAP injection
- XML/XXE attacks

### PII Masking (GDPR)

Anonymize personally identifiable information:

```yaml
options:
  pii:
    enabled: true
    mask_ip_method: last_octet     # last_octet, hash, full
    mask_ipv6_method: last_64      # last_64, hash, full
    strip_headers:                  # Remove sensitive headers
      - Cookie
      - Authorization
      - X-Forwarded-For
    mask_query_params:              # Mask query parameters
      - email
      - phone
      - ssn
    preserve_localhost: true        # Don't mask private IPs
```

**IP Masking Methods:**
- `last_octet` - `192.168.1.100` → `192.168.1.0`
- `hash` - `192.168.1.100` → `sha256(ip + salt)`
- `full` - `192.168.1.100` → `0.0.0.0`

### GeoIP Filtering

Country-based access control:

```yaml
options:
  geoip:
    enabled: true
    database_path: /usr/share/GeoIP/GeoLite2-Country.mmdb
    alert_on_unusual_country: true
    expected_countries:           # Allow only these
      - US
      - CA
      - GB
      - DE
    cache_expiry_minutes: 60
```

### Data Retention

Control log retention (GDPR compliance):

```yaml
options:
  retention:
    enabled: true
    access_log_days: 30          # Access logs
    security_log_days: 90        # Security events
    audit_log_days: 365          # Audit trail
    metrics_days: 90             # Performance metrics
    health_check_days: 7         # Health check results
    websocket_log_days: 30       # WebSocket connections
    policy_type: public          # public, private, custom
```

**Policy Types:**
- `public` - 30/90/365 days (public-facing sites)
- `private` - 7/30/90 days (internal services)
- `custom` - Use specified values

### Compression

Response compression configuration:

```yaml
options:
  compression:
    enabled: true
    algorithms:              # Preference order
      - brotli
      - gzip
    level: 6                 # Compression level (1-11 for brotli, 1-9 for gzip)
    min_size: 1024           # Minimum response size (bytes)
    content_types:           # Compress these MIME types
      - text/html
      - text/css
      - application/javascript
      - application/json
      - application/xml
```

### WebSocket

WebSocket-specific tuning:

```yaml
options:
  websocket:
    enabled: true
    max_connections: 1000        # Max concurrent connections
    max_duration: 24h            # Maximum connection duration
    idle_timeout: 5m             # Idle timeout before ping
    ping_interval: 30s           # Ping frequency
```

### Connection Pooling

HTTP connection management:

```yaml
options:
  connection_pool:
    max_idle_conns: 100          # Total idle connections
    max_idle_conns_per_host: 10  # Per-host idle connections
    max_conns_per_host: 50       # Max connections per host
    idle_timeout: 90s            # Idle connection lifetime
```

### Retry Logic

Automatic retry with backoff:

```yaml
options:
  retry:
    enabled: true
    max_attempts: 3
    backoff: exponential         # exponential or linear
    initial_delay: 100ms
    max_delay: 2s
    retry_on:                    # Retry conditions
      - connection_refused
      - timeout
      - "502"
      - "503"
      - "504"
```

### Slow Request Detection

Monitor and alert on slow backends:

```yaml
options:
  slow_request:
    enabled: true
    warning: 1s                  # Log warning
    critical: 5s                 # Log critical
    timeout: 30s                 # Hard timeout
    alert_webhook: true          # Send webhook alert
```

### Request/Response Limits

Size limits for safety:

```yaml
options:
  limits:
    max_request_body: 10485760   # 10MB request body
    max_response_body: 10485760  # 10MB response body
```

---

## Environment Variables

Override defaults via environment:

| Variable | Default | Description |
|----------|---------|-------------|
| `SITES_PATH` | `/etc/proxy/sites-available` | Site configs directory |
| `GLOBAL_CONFIG` | `/etc/proxy/global.yaml` | Global config file |
| `DB_PATH` | `/data/proxy.db` | SQLite database |
| `BACKUP_DIR` | `/mnt/storagebox/backups/proxy` | Backup location |
| `HTTP_ADDR` | `:80` | HTTP listen address |
| `HTTPS_ADDR` | `:443` | HTTPS listen address |
| `REGISTRY_PORT` | `81` | Service registry port |
| `HEALTH_PORT` | `8080` | Health/metrics port |
| `UPSTREAM_CHECK_TIMEOUT` | `2s` | Upstream health timeout |
| `SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |
| `DEBUG` | `0` | Debug logging (1=on) |
| `TZ` | `UTC` | Timezone |

---

## Examples

### Minimal Configuration

```yaml
enabled: true

service:
  name: simple

routes:
  - domains: [example.com]
    path: /
    backend: http://app:8080
```

### Production Configuration

```yaml
enabled: true

service:
  name: production-app
  maintenance_port: 8081

routes:
  - domains:
      - app.example.com
      - www.app.example.com
    path: /
    backend: http://app:8080

headers:
  X-App-Version: "2.0"

options:
  health_check_path: /health
  health_check_interval: 30s
  timeout: 30s
  
  timeouts:
    connect: 5s
    read: 30s
    write: 30s
    idle: 120s
  
  circuit_breaker:
    enabled: true
    failure_threshold: 5
    timeout: 30s
  
  rate_limit:
    enabled: true
    requests_per_min: 100
    requests_per_hour: 5000
    per_ip: true
  
  waf:
    enabled: true
    block_mode: true
    sensitivity: high
  
  pii:
    enabled: true
    mask_ip_method: last_octet
  
  compression:
    enabled: true
    algorithms: [brotli, gzip]
    level: 6
  
  retry:
    enabled: true
    max_attempts: 3
    backoff: exponential
```

### WebSocket Application

```yaml
enabled: true

service:
  name: realtime

routes:
  - domains: [ws.example.com]
    path: /
    backend: http://websocket-server:3000
    websocket: true

options:
  websocket:
    enabled: true
    max_connections: 5000
    max_duration: 24h
    ping_interval: 30s
  
  timeout: 300s
  
  rate_limit:
    enabled: true
    requests_per_min: 10
    per_ip: true
```

### API Gateway

```yaml
enabled: true

service:
  name: api-gateway

routes:
  - domains: [api.example.com]
    path: /v1
    backend: http://api-v1:8080
  
  - domains: [api.example.com]
    path: /v2
    backend: http://api-v2:9000
    headers:
      X-API-Version: "2.0"
  
  - domains: [api.example.com]
    path: /ws
    backend: http://ws-server:3000
    websocket: true

headers:
  Access-Control-Allow-Origin: "*"
  Access-Control-Allow-Methods: "GET, POST, PUT, DELETE, OPTIONS"
  Access-Control-Allow-Headers: "Content-Type, Authorization"

options:
  timeout: 60s
  max_body_size: 10M
  
  rate_limit:
    enabled: true
    requests_per_min: 1000
    requests_per_hour: 50000
    per_route: true
  
  retry:
    enabled: true
    max_attempts: 3
  
  compression: true
```

---

## Validation

The proxy validates all configuration on startup:

- **Required fields** - `enabled`, `routes`, `domains`, `backend`
- **Format checks** - Duration strings (`30s`, `5m`), size units (`10M`, `1G`)
- **Value ranges** - Ports (1-65535), thresholds (> 0)
- **Path conflicts** - No overlapping route definitions
- **Certificate files** - Must exist and be readable

Invalid configurations prevent startup with detailed error messages.
