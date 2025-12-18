# Additional Features for Gaming Community Deployment

## Context
- 20+ international gaming community users
- Public-facing Pterodactyl panel and community sites
- Private Vaultwarden (personal use)
- Hosted in Germany, GDPR compliance required
- Dynamic home IP (no static IP whitelist possible)

---

## Additional Tasks to Implement

### 14. Add Rate Limiting System
**Priority**: CRITICAL (Security)

**Requirements:**
- Per-IP rate limiting with configurable thresholds per route
- Per-route global rate limits
- Path-specific limits (e.g., `/auth/*` = 5 req/min, normal = 100 req/min)
- Failed login tracking and auto-ban
- Temporary bans with configurable duration
- Whitelist/exemption support
- Redis-like sliding window algorithm

**Configuration Example:**
```yaml
# Pterodactyl (public)
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
rate_limit:
  global: 1000  # requests per minute for entire route
  per_ip: 100   # requests per minute per IP
  paths:
    - path: /auth/*
      per_ip: 5
      ban_after: 3  # Ban after 3 violations
      ban_duration: 3600  # 1 hour
    - path: /api/client/*
      per_ip: 60

# Vaultwarden (private, strict)
host: vault.chilla55.de
backend: http://vaultwarden:80
rate_limit:
  per_ip: 10
  paths:
    - path: /identity/*  # Login endpoint
      per_ip: 3
      ban_after: 3
      ban_duration: 3600
      alert_webhook: true  # Discord alert
```

**Database Schema:**
```sql
CREATE TABLE rate_limits (
  ip_address TEXT NOT NULL,
  route_id INTEGER NOT NULL,
  window_start INTEGER NOT NULL,
  request_count INTEGER DEFAULT 0,
  banned_until INTEGER,
  ban_count INTEGER DEFAULT 0,
  PRIMARY KEY (ip_address, route_id, window_start)
);

CREATE TABLE rate_limit_violations (
  violation_id INTEGER PRIMARY KEY,
  ip_address TEXT NOT NULL,
  route_id INTEGER NOT NULL,
  path TEXT NOT NULL,
  timestamp INTEGER NOT NULL,
  request_count INTEGER,
  action TEXT  -- 'throttled', 'banned'
);

CREATE INDEX idx_rate_limits_banned ON rate_limits(banned_until);
```

---

### 15. Add Basic WAF (Web Application Firewall)
**Priority**: CRITICAL (Security)

**Features:**
- SQL injection pattern detection
- XSS attempt blocking
- Path traversal prevention (`../`, `..\\`)
- Suspicious header filtering
- Request size limits
- Per-route enable/disable
- Logging vs blocking modes

**Configuration:**
```yaml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
waf:
  enabled: true
  mode: block  # or 'log' for testing
  rules:
    sql_injection: true
    xss: true
    path_traversal: true
    suspicious_headers: true
  max_request_size: 10485760  # 10 MB
  max_header_size: 8192
```

**Detection Patterns:**
```go
// SQL Injection patterns
- UNION.*SELECT
- SELECT.*FROM
- INSERT.*INTO
- DROP.*TABLE
- --.*
- /\*.*\*/

// XSS patterns
- <script.*>
- javascript:
- onerror=
- onload=

// Path traversal
- ../
- ..\\
- %2e%2e/
- %2e%2e%5c
```

**Database Schema:**
```sql
CREATE TABLE waf_blocks (
  block_id INTEGER PRIMARY KEY,
  timestamp INTEGER NOT NULL,
  ip_address TEXT NOT NULL,
  route_id INTEGER,
  path TEXT NOT NULL,
  rule_triggered TEXT NOT NULL,
  pattern_matched TEXT,
  request_snippet TEXT,
  action TEXT  -- 'blocked', 'logged'
);

CREATE INDEX idx_waf_timestamp ON waf_blocks(timestamp);
CREATE INDEX idx_waf_ip ON waf_blocks(ip_address);
```

---

### 16. Add Sensitive Data Filtering
**Priority**: CRITICAL (GDPR Compliance)

**Features:**
- Strip sensitive headers from logs (Authorization, Cookie, Set-Cookie, X-API-Key)
- Redact sensitive URL paths (password reset tokens, API keys in URLs)
- PII masking (IP addresses, email addresses)
- Per-route configuration
- Configurable redaction patterns

**Configuration:**
```yaml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
logging:
  strip_headers:
    - Authorization
    - Cookie
    - Set-Cookie
    - X-API-Key
    - X-Auth-Token
  redact_paths:
    - pattern: /api/client/account/password
      replacement: /api/client/account/[REDACTED]
    - pattern: /auth/password/reset/[^/]+
      replacement: /auth/password/reset/[TOKEN]
  mask_pii: true  # Mask IP addresses, emails
  mask_ip_method: last_octet  # 203.0.113.45 -> 203.0.113.xxx
```

**Implementation:**
```go
// IP masking for GDPR
func maskIP(ip string) string {
  if strings.Contains(ip, ":") {
    // IPv6: Keep first 4 groups
    parts := strings.Split(ip, ":")
    return strings.Join(parts[:4], ":") + ":xxxx:xxxx:xxxx:xxxx"
  }
  // IPv4: Mask last octet
  parts := strings.Split(ip, ".")
  return strings.Join(parts[:3], ".") + ".xxx"
}
```

---

### 17. Add Traffic Analytics
**Priority**: HIGH (Operational Visibility)

**Features:**
- Top routes by traffic volume
- Peak traffic hours heatmap (hourly/daily/weekly)
- Per-route bandwidth tracking
- Request distribution by status code
- Average response time trends
- User agent breakdown (bot vs browser vs API)
- GeoIP distribution

**API Endpoints:**
```
GET /api/analytics/top-routes?period=24h
GET /api/analytics/heatmap?period=7d
GET /api/analytics/bandwidth?route=gpanel.chilla55.de
GET /api/analytics/user-agents?period=24h
GET /api/analytics/geoip?period=7d
```

**Database Schema:**
```sql
CREATE TABLE analytics_hourly (
  hour_timestamp INTEGER NOT NULL,
  route_id INTEGER NOT NULL,
  request_count INTEGER DEFAULT 0,
  error_count INTEGER DEFAULT 0,
  bytes_sent INTEGER DEFAULT 0,
  bytes_received INTEGER DEFAULT 0,
  avg_response_time_ms REAL DEFAULT 0,
  PRIMARY KEY (hour_timestamp, route_id)
);

CREATE TABLE analytics_user_agents (
  timestamp INTEGER NOT NULL,
  route_id INTEGER NOT NULL,
  user_agent_type TEXT NOT NULL,  -- 'bot', 'browser', 'api', 'mobile'
  count INTEGER DEFAULT 0
);

CREATE TABLE analytics_geoip (
  timestamp INTEGER NOT NULL,
  route_id INTEGER NOT NULL,
  country_code TEXT NOT NULL,
  city TEXT,
  count INTEGER DEFAULT 0
);
```

---

### 18. Add GeoIP Tracking
**Priority**: MEDIUM (Analytics & Security)

**Features:**
- IP to country/city lookup
- Store location data in analytics
- Optional country-based access control
- Detect unusual login locations
- Dashboard visualization

**Implementation:**
```go
// Use MaxMind GeoLite2 database (free)
import "github.com/oschwald/geoip2-golang"

db, _ := geoip2.Open("/data/GeoLite2-City.mmdb")
record, _ := db.City(net.ParseIP("203.0.113.45"))

country := record.Country.IsoCode  // "DE"
city := record.City.Names["en"]     // "Berlin"
```

**Optional Configuration:**
```yaml
host: vault.chilla55.de
backend: http://vaultwarden:80
geoip:
  enabled: true
  allowed_countries:  # Optional whitelist
    - DE
    - AT
    - CH
  alert_on_unusual_country: true  # Alert if login from unexpected country
```

---

### 19. Add Webhook Notifications
**Priority**: HIGH (Operations)

**Features:**
- Discord/Slack/generic webhook integration
- Configurable alert conditions
- Alert types: service down, certificate expiring, high error rate, failed logins, WAF blocks
- Per-route webhook configuration
- Rate-limited notifications (don't spam)

**Configuration:**
```yaml
webhooks:
  - name: discord-alerts
    url: https://discord.com/api/webhooks/...
    events:
      - service_down
      - cert_expiring_30d
      - cert_expiring_7d
      - high_error_rate  # >5% errors
      - failed_login_spike  # >10 failed logins/min
      - waf_block_spike  # >50 blocks/min
    routes:  # Optional: specific routes only
      - gpanel.chilla55.de
      - vault.chilla55.de
    throttle: 300  # Max one alert per 5 minutes per event type
```

**Discord Webhook Format:**
```json
{
  "embeds": [{
    "title": "🚨 Service Down Alert",
    "description": "Pterodactyl panel is not responding",
    "color": 15158332,
    "fields": [
      {"name": "Service", "value": "gpanel.chilla55.de", "inline": true},
      {"name": "Backend", "value": "pterodactyl_panel:80", "inline": true},
      {"name": "Last Seen", "value": "2 minutes ago", "inline": true}
    ],
    "timestamp": "2025-12-18T14:32:00Z"
  }]
}
```

---

### 20. Add Request Tracing
**Priority**: MEDIUM (Debugging)

**Features:**
- Unique request ID for each request
- Trace ID in headers: `X-Request-ID`
- Include request ID in all logs
- Search logs by request ID
- Include request ID in error pages

**Implementation:**
```go
// Generate unique request ID
requestID := uuid.New().String()

// Add to request context
ctx := context.WithValue(r.Context(), "request_id", requestID)

// Add to response headers
w.Header().Set("X-Request-ID", requestID)

// Include in all logs
log.Printf("[%s] Request: %s %s", requestID, r.Method, r.URL.Path)
```

**User Experience:**
```html
<!-- Error page shows request ID -->
<h1>502 Bad Gateway</h1>
<p>Request ID: abc123-def456-789</p>
<p>Please include this ID when reporting the issue.</p>
```

---

### 21. Add Audit Log for Config Changes
**Priority**: CRITICAL (Accountability)

**Features:**
- Track all configuration changes
- Track dashboard actions (maintenance mode, route changes)
- Store who, what, when
- Diff view (before/after)
- Immutable audit trail

**Database Schema:**
```sql
CREATE TABLE audit_log (
  audit_id INTEGER PRIMARY KEY,
  timestamp INTEGER NOT NULL,
  user TEXT NOT NULL,  -- 'admin', 'system', IP address
  action TEXT NOT NULL,  -- 'config_reload', 'route_added', 'maintenance_enabled'
  resource_type TEXT NOT NULL,  -- 'route', 'certificate', 'setting'
  resource_id TEXT,
  old_value TEXT,  -- JSON or YAML
  new_value TEXT,  -- JSON or YAML
  source TEXT,  -- 'dashboard', 'api', 'file_watcher'
  notes TEXT
);

CREATE INDEX idx_audit_timestamp ON audit_log(timestamp DESC);
CREATE INDEX idx_audit_action ON audit_log(action);
```

**API Endpoint:**
```
GET /api/audit?limit=100&action=config_reload
```

---

### 22. Add Per-Route Data Retention Policies
**Priority**: HIGH (GDPR Compliance)

**Features:**
- Configurable retention per route
- Different retention for different log types
- Automatic purging on schedule
- Proof of deletion logs

**Configuration:**
```yaml
# Public gaming community routes
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
data_retention:
  access_logs: 7d      # GDPR data minimization
  error_logs: 30d      # Keep errors longer
  security_logs: 90d   # Failed auth, WAF blocks
  audit_logs: 365d     # Keep for accountability
  metrics: 30d

# Private Vaultwarden
host: vault.chilla55.de
backend: http://vaultwarden:80
data_retention:
  access_logs: 30d     # Personal service, can keep longer
  error_logs: 90d
  security_logs: 365d  # Security events kept longer
  metrics: 90d
```

**Implementation:**
```sql
-- Automatic cleanup job (run daily)
DELETE FROM request_logs 
WHERE timestamp < (strftime('%s', 'now') - 
  (SELECT retention_days * 86400 FROM routes WHERE route_id = request_logs.route_id));
```

---

## Integration with Existing Tasks

These new features integrate with existing todo items:

**Task 1 (Metrics Collection)** - Add security events, GeoIP, user agent tracking
**Task 2 (API Endpoints)** - Add analytics, audit log APIs
**Task 3 (AI Context)** - Include security events, top attacks, traffic patterns
**Task 4 (Dashboard)** - Add analytics charts, heatmaps, security alerts
**Task 6 (Request Logging)** - Add filtering, PII masking, request tracing
**Task 10 (SQLite Storage)** - Add rate limiting, WAF, analytics, audit tables

---

## Additional Reliability & Performance Features

### 23. Add Timeout Configuration
**Priority**: CRITICAL (Reliability)

**Requirements:**
- Per-route timeout configuration
- Backend connection timeout
- Read/write timeouts
- Idle connection timeout
- Graceful shutdown timeout

**Configuration Example:**
```yaml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
timeouts:
  connect: 5s      # Backend connection timeout
  read: 30s        # Read from backend timeout
  write: 30s       # Write to backend timeout
  idle: 120s       # Keep-alive idle timeout
  
# Slower backend example
host: api.example.com
backend: http://slow_service:3000
timeouts:
  connect: 10s
  read: 60s        # Allow longer processing time
  write: 60s
  idle: 300s
```

**Implementation:**
```go
// Per-route timeout configuration
type TimeoutConfig struct {
  Connect time.Duration `yaml:"connect"`  // Default: 5s
  Read    time.Duration `yaml:"read"`     // Default: 30s
  Write   time.Duration `yaml:"write"`    // Default: 30s
  Idle    time.Duration `yaml:"idle"`     // Default: 120s
}

// Apply timeouts to backend transport
transport := &http.Transport{
  DialContext: (&net.Dialer{
    Timeout: config.Timeouts.Connect,
  }).DialContext,
  IdleConnTimeout: config.Timeouts.Idle,
}

// Apply to client
client := &http.Client{
  Transport: transport,
  Timeout: config.Timeouts.Read + config.Timeouts.Write,
}
```

**Metrics Tracking:**
- Count timeout errors separately (connection vs read vs write)
- Include in error logs with timeout type
- Alert on timeout rate >5%

---

### 24. Add Request/Response Size Limits
**Priority**: CRITICAL (Security)

**Requirements:**
- Max request body size per route
- Max request header size
- Max response size (prevent backend from sending huge responses)
- Graceful rejection with 413 Payload Too Large

**Configuration Example:**
```yaml
# Pterodactyl (file uploads)
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
limits:
  max_request_body: 104857600   # 100 MB (server backups)
  max_request_headers: 16384    # 16 KB
  max_response_body: 52428800   # 50 MB
  
# API endpoint (small payloads only)
host: api.example.com
backend: http://api_service:3000
limits:
  max_request_body: 1048576     # 1 MB
  max_request_headers: 8192     # 8 KB
  max_response_body: 10485760   # 10 MB

# Vaultwarden (strict)
host: vault.chilla55.de
backend: http://vaultwarden:80
limits:
  max_request_body: 10485760    # 10 MB (vault attachments)
  max_request_headers: 8192
  max_response_body: 10485760
```

**Implementation:**
```go
// Wrap request body reader
limitedReader := io.LimitReader(r.Body, config.Limits.MaxRequestBody)

// Check if limit exceeded
if limitedReader.(*io.LimitedReader).N == 0 {
  http.Error(w, "413 Payload Too Large", http.StatusRequestEntityTooLarge)
  return
}

// Wrap response body
responseReader := io.LimitReader(resp.Body, config.Limits.MaxResponseBody)
```

**Error Pages:**
```html
<!-- 413.html -->
<!DOCTYPE html>
<html>
<head><title>413 Payload Too Large</title></head>
<body>
  <h1>Request Too Large</h1>
  <p>Maximum allowed size: {{.MaxSize}}</p>
  <p>Your request size: {{.RequestSize}}</p>
</body>
</html>
```

---

### 25. Add Header Manipulation
**Priority**: HIGH (Security & Compliance)

**Requirements:**
- Add/remove/modify request headers (to backend)
- Add/remove/modify response headers (to client)
- Security headers: HSTS, X-Frame-Options, CSP, X-Content-Type-Options
- Remove server version headers
- CORS configuration

**Configuration Example:**
```yaml
# Pterodactyl (public, needs security headers)
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
headers:
  request:
    add:
      X-Forwarded-Proto: https
      X-Real-IP: ${client_ip}
    remove:
      - Cookie  # Don't send cookies to backend
  response:
    add:
      Strict-Transport-Security: "max-age=31536000; includeSubDomains"
      X-Frame-Options: "SAMEORIGIN"
      X-Content-Type-Options: "nosniff"
      X-XSS-Protection: "1; mode=block"
      Referrer-Policy: "strict-origin-when-cross-origin"
      Permissions-Policy: "geolocation=(), microphone=(), camera=()"
    remove:
      - Server
      - X-Powered-By
      
# API with CORS
host: api.example.com
backend: http://api_service:3000
headers:
  response:
    add:
      Access-Control-Allow-Origin: "https://example.com"
      Access-Control-Allow-Methods: "GET, POST, PUT, DELETE"
      Access-Control-Allow-Headers: "Content-Type, Authorization"
      Access-Control-Max-Age: "86400"
      
# Vaultwarden (strict CSP)
host: vault.chilla55.de
backend: http://vaultwarden:80
headers:
  response:
    add:
      Content-Security-Policy: "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'"
      Strict-Transport-Security: "max-age=31536000; includeSubDomains; preload"
      X-Frame-Options: "DENY"
```

**Implementation:**
```go
// Request headers (to backend)
for key, value := range config.Headers.Request.Add {
  // Support variable substitution
  value = strings.ReplaceAll(value, "${client_ip}", clientIP)
  proxyReq.Header.Set(key, value)
}

for _, key := range config.Headers.Request.Remove {
  proxyReq.Header.Del(key)
}

// Response headers (to client)
for key, value := range config.Headers.Response.Add {
  w.Header().Set(key, value)
}

for _, key := range config.Headers.Response.Remove {
  w.Header().Del(key)
}
```

**Security Presets:**
```yaml
# Simple preset system
headers:
  preset: security-strict  # or 'security-basic', 'cors-permissive', 'api'
  
# Presets expand to full configuration
# security-strict = HSTS, CSP, X-Frame-Options: DENY, etc.
# security-basic = HSTS, X-Frame-Options: SAMEORIGIN, etc.
# cors-permissive = CORS headers with wildcard
# api = CORS + JSON content-type enforcement
```

---

### 26. Add WebSocket Connection Tracking
**Priority**: HIGH (Operational Visibility)

**Requirements:**
- Track active WebSocket connections per route
- Connection duration tracking
- Connection rate metrics
- Alert on connection leaks (long-lived connections)
- Graceful shutdown handling (close WS connections)

**Metrics to Track:**
```sql
CREATE TABLE websocket_connections (
  conn_id INTEGER PRIMARY KEY,
  route_id INTEGER NOT NULL,
  client_ip TEXT NOT NULL,
  connected_at INTEGER NOT NULL,
  disconnected_at INTEGER,
  duration_seconds INTEGER,
  bytes_sent INTEGER DEFAULT 0,
  bytes_received INTEGER DEFAULT 0,
  close_reason TEXT,
  FOREIGN KEY(route_id) REFERENCES routes(route_id)
);

CREATE INDEX idx_ws_active ON websocket_connections(disconnected_at) 
  WHERE disconnected_at IS NULL;
```

**Dashboard Display:**
```
WebSocket Connections:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Route                    Active    Peak (24h)   Avg Duration
────────────────────────────────────────────────────────────
gpanel.chilla55.de         12         28         45m 23s
api.example.com             3          8         12m 05s
────────────────────────────────────────────────────────────
Total:                     15         36         

⚠️ Long-lived connections:
  • gpanel.chilla55.de - 203.0.113.45 - 4h 23m (Pterodactyl console)
  • api.example.com - 198.51.100.22 - 2h 15m
```

**Configuration:**
```yaml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
websocket:
  enabled: true
  max_connections: 100        # Per route limit
  max_duration: 86400         # 24 hours max
  idle_timeout: 300           # 5 minutes idle = close
  ping_interval: 30           # Send ping every 30s
```

**Implementation:**
```go
// Detect WebSocket upgrade
if r.Header.Get("Upgrade") == "websocket" {
  // Track connection
  connID := trackWebSocketConnection(routeID, clientIP)
  
  // Proxy WebSocket with bidirectional copy
  backend, _ := upgradeBackend(backendConn)
  client, _ := upgradeClient(w, r)
  
  go copyWebSocket(backend, client, connID, "backend->client")
  go copyWebSocket(client, backend, connID, "client->backend")
  
  // Wait for close
  <-closeChan
  recordWebSocketClose(connID, closeReason)
}
```

---

### 27. Add Compression Support
**Priority**: MEDIUM (Performance)

**Requirements:**
- gzip and brotli compression
- Automatic content-type detection
- Configurable compression level per route
- Minimum size threshold (don't compress <1KB)
- Respect Accept-Encoding header

**Configuration Example:**
```yaml
# Community site (compress HTML, CSS, JS)
host: community.example.com
backend: http://web_server:80
compression:
  enabled: true
  algorithms:
    - brotli  # Prefer brotli
    - gzip
  level: 6              # 1-9 for gzip, 0-11 for brotli
  min_size: 1024        # Don't compress <1KB
  content_types:
    - text/html
    - text/css
    - text/javascript
    - application/javascript
    - application/json
    - text/plain
    - application/xml
    
# Pterodactyl (compress API responses)
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
compression:
  enabled: true
  algorithms: [gzip]  # Simpler config
  level: 6
  min_size: 512
  content_types:
    - application/json
```

**Implementation:**
```go
// Check if compression supported
acceptEncoding := r.Header.Get("Accept-Encoding")
var compressor io.WriteCloser

if strings.Contains(acceptEncoding, "br") && config.Compression.SupportsBrotli() {
  compressor = brotli.NewWriterLevel(w, config.Compression.Level)
  w.Header().Set("Content-Encoding", "br")
} else if strings.Contains(acceptEncoding, "gzip") {
  compressor, _ = gzip.NewWriterLevel(w, config.Compression.Level)
  w.Header().Set("Content-Encoding", "gzip")
}

// Only compress if content-type matches and size > threshold
contentType := resp.Header.Get("Content-Type")
if shouldCompress(contentType, responseSize) {
  io.Copy(compressor, resp.Body)
  compressor.Close()
} else {
  io.Copy(w, resp.Body)
}
```

**Metrics:**
```sql
-- Add to metrics table
ALTER TABLE metrics ADD COLUMN bytes_saved_compression INTEGER DEFAULT 0;

-- Track compression ratio
SELECT 
  route_id,
  SUM(bytes_sent) as uncompressed,
  SUM(bytes_sent - bytes_saved_compression) as compressed,
  ROUND(100.0 * SUM(bytes_saved_compression) / SUM(bytes_sent), 2) as ratio_pct
FROM metrics
WHERE compression_used = 1
GROUP BY route_id;
```

---

### 28. Add Connection Pooling
**Priority**: MEDIUM (Performance)

**Requirements:**
- HTTP connection pooling per backend
- Configurable pool size and idle connections
- Connection lifetime management
- Metrics on pool utilization

**Configuration:**
```yaml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
connection_pool:
  max_idle_conns: 100          # Max idle connections total
  max_idle_conns_per_host: 10  # Max idle per backend
  max_conns_per_host: 50       # Max total per backend
  idle_timeout: 90s            # Close idle after duration
```

**Implementation:**
```go
// Create transport with pooling
transport := &http.Transport{
  MaxIdleConns:        config.Pool.MaxIdleConns,
  MaxIdleConnsPerHost: config.Pool.MaxIdleConnsPerHost,
  MaxConnsPerHost:     config.Pool.MaxConnsPerHost,
  IdleConnTimeout:     config.Pool.IdleTimeout,
  
  // Connection reuse
  DisableKeepAlives: false,
  
  // TLS optimization
  TLSHandshakeTimeout: 10 * time.Second,
}

// Reuse transport across requests
client := &http.Client{Transport: transport}
```

**Metrics:**
```
GET /api/stats/connection-pools

{
  "pools": [
    {
      "backend": "pterodactyl_panel:80",
      "active": 5,
      "idle": 8,
      "max": 50,
      "reuse_rate": 0.92,  // 92% of requests reused connection
      "avg_conn_age_seconds": 45
    }
  ]
}
```

---

### 29. Add Slow Request Detection
**Priority**: MEDIUM (Troubleshooting)

**Requirements:**
- Detect requests exceeding time thresholds
- Multiple threshold levels (warning, critical)
- Include in logs and metrics
- Alert on patterns (same endpoint always slow)

**Configuration:**
```yaml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
slow_request:
  enabled: true
  thresholds:
    warning: 5s    # Log warning
    critical: 10s  # Log error + alert
    timeout: 30s   # Abort request
  alert_webhook: true  # Send to Discord
```

**Implementation:**
```go
start := time.Now()
resp, err := client.Do(proxyReq)
duration := time.Since(start)

if duration > config.SlowRequest.Critical {
  log.Error().
    Str("route", route.Host).
    Str("path", r.URL.Path).
    Dur("duration", duration).
    Msg("CRITICAL: Slow request detected")
    
  // Send webhook alert
  sendSlowRequestAlert(route, r.URL.Path, duration)
  
} else if duration > config.SlowRequest.Warning {
  log.Warn().
    Str("route", route.Host).
    Str("path", r.URL.Path).
    Dur("duration", duration).
    Msg("WARNING: Slow request")
}
```

**Dashboard Display:**
```
Slow Requests (Last 24h):
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Endpoint                          Count   Avg      P95      P99
────────────────────────────────────────────────────────────────
gpanel.chilla55.de/api/servers      42    6.2s     9.8s    12.3s
api.example.com/slow-query          15   11.5s    15.2s    18.9s
────────────────────────────────────────────────────────────────

⚠️ Endpoints consistently slow:
  • /api/servers - 42 slow requests in 24h (consider caching)
  • /slow-query - P99 = 18.9s (investigate backend)
```

---

### 30. Add Request Retry Logic
**Priority**: LOW (Reliability)

**Requirements:**
- Auto-retry on network errors (connection refused, timeout)
- Only retry idempotent methods (GET, HEAD, OPTIONS)
- Configurable retry count and backoff
- Track retry metrics

**Configuration:**
```yaml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
retry:
  enabled: true
  max_attempts: 3
  backoff: exponential  # or 'linear', 'constant'
  initial_delay: 100ms
  max_delay: 2s
  retry_on:
    - connection_refused
    - timeout
    - 502  # Bad Gateway
    - 503  # Service Unavailable
    - 504  # Gateway Timeout
```

**Implementation:**
```go
func doRequestWithRetry(req *http.Request, config RetryConfig) (*http.Response, error) {
  var resp *http.Response
  var err error
  
  for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
    resp, err = client.Do(req)
    
    if err == nil && !shouldRetry(resp.StatusCode, config) {
      return resp, nil
    }
    
    if attempt < config.MaxAttempts {
      delay := calculateBackoff(attempt, config)
      log.Warn().
        Int("attempt", attempt).
        Dur("retry_in", delay).
        Msg("Request failed, retrying")
      time.Sleep(delay)
    }
  }
  
  return resp, err
}
```

**Metrics:**
```sql
ALTER TABLE request_logs ADD COLUMN retry_count INTEGER DEFAULT 0;

-- Query for retry patterns
SELECT 
  path,
  COUNT(*) as total_requests,
  SUM(CASE WHEN retry_count > 0 THEN 1 ELSE 0 END) as retried_requests,
  AVG(retry_count) as avg_retries
FROM request_logs
WHERE timestamp > (strftime('%s', 'now') - 86400)
GROUP BY path
HAVING retried_requests > 0
ORDER BY retried_requests DESC;
```

---

## Implementation Priority (Updated)

### Phase 0: Essential Reliability (IMMEDIATE - Before Phase 1)
1. **Timeout configuration (#23)** - Prevent hanging connections
2. **Request/response size limits (#24)** - Security basics
3. **Header manipulation (#25)** - Security headers (HSTS, CSP)

### Phase 1: Security & Compliance (Week 1-2)
4. Rate limiting system (#14)
5. Basic WAF (#15)
6. Sensitive data filtering (#16)
7. Audit logging (#21)
8. Per-route retention policies (#22)

### Phase 2: Monitoring & Analytics (Week 3-4)
9. Traffic analytics (#17)
10. GeoIP tracking (#18)
11. Webhook notifications (#19)
12. Request tracing (#20)
13. WebSocket tracking (#26)
14. Slow request detection (#29)
15. Existing: Backend health checking (Task #5)
16. Existing: Certificate monitoring (Task #7)

### Phase 3: Performance & Optimization (Week 5-6)
17. Compression support (#27)
18. Connection pooling (#28)
19. Request retry logic (#30)

### Phase 4: Dashboard & UX (Week 7-8)
20. Existing: Web dashboard (Task #4)
21. Existing: AI context export (Task #3)
22. Existing: Custom error pages (Task #9)
23. Existing: Maintenance mode (Task #11)

### Phase 5: Reliability & Operations (Week 9+)
24. Existing: Circuit breaker (Task #8)
25. Existing: Backup system (Task #12)
26. Existing: Deployment updates (Task #13)

---

## GDPR Compliance Summary

**Required for Gaming Community:**
- ✅ PII masking in logs (IP addresses, emails)
- ✅ Data minimization (7-day retention for access logs)
- ✅ Purpose limitation (document what's collected)
- ✅ Right to erasure (ability to purge user data)
- ✅ Security measures (encryption, access control)
- ✅ Audit trail (who accessed/changed what)
- ✅ Breach notification procedures (webhook alerts)

**Implementation Notes:**
- Public routes (Pterodactyl, community): Strict GDPR compliance
- Private routes (Vaultwarden): Relaxed (personal use exemption)
- Admin routes: Full audit logging, no PII exemptions
