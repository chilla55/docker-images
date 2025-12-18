# Go Proxy Implementation Plan

## Table of Contents
1. [Project Overview](#project-overview)
2. [Feature Summary](#feature-summary)
3. [Implementation Roadmap](#implementation-roadmap)
4. [Phase 0: Essential Reliability](#phase-0-essential-reliability)
5. [Phase 1: Security & Compliance](#phase-1-security--compliance)
6. [Phase 2: Core Monitoring](#phase-2-core-monitoring)
7. [Phase 3: Advanced Analytics](#phase-3-advanced-analytics)
8. [Phase 4: Performance & Optimization](#phase-4-performance--optimization)
9. [Phase 5: Dashboard & UX](#phase-5-dashboard--ux)
10. [Phase 6: Operations & Maintenance](#phase-6-operations--maintenance)
11. [Database Schema](#database-schema)
12. [Deployment & Testing](#deployment--testing)

---

## Project Overview

### Current State
This Go-based reverse proxy (`proxy-manager`) replaces nginx for HTTP/HTTPS traffic routing with enhanced observability and security features.

**Technical Stack:**
- Pure Go with HTTP/2 and HTTP/3 (QUIC) support
- TLS certificate hot-reload via fsnotify
- Automatic site discovery from `/etc/nginx/sites-enabled/` YAML files
- Certificate auto-reload from `/mnt/storagebox/certs/` (Certbot renewals)
- Docker Swarm deployment with storagebox mounts (rslave propagation)
- Backend services identified by Docker service names (not IPs)

### Deployment Context
- **Environment**: Hetzner dedicated server in Germany
- **User Base**: 20+ international gaming community members (EU-based, worldwide access)
- **Public Services**: Pterodactyl panel, community websites/forums
- **Private Services**: Vaultwarden (personal use only)
- **Administrator**: Single admin managing all infrastructure
- **Compliance**: GDPR required for EU gaming community users
- **Traffic**: HTTP/HTTPS only (game servers bypass proxy)
- **Backend Architecture**: Single-instance containers per service

### Architecture Decisions
| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Dashboard Access** | localhost:8080 via SSH tunnel | Security without auth complexity |
| **Data Storage** | SQLite (modernc.org/sqlite) | Embedded, secure, no CGO required |
| **Service Tracking** | Docker name + port | Persistent across IP changes |
| **Backup Strategy** | Three-tier (full/diff/incr) | ~53.5 MB storage, 4-week retention |
| **Proxy Scope** | HTTP/HTTPS only | WebSockets supported, no TCP proxy |
| **Security Model** | Per-route configuration | Public vs private service separation |
| **Logging** | GDPR-compliant PII masking | 7-90 day retention per route type |

### Security & Compliance Requirements

**GDPR Compliance (Germany + EU Users):**
- ✅ **Data Minimization**: Only log necessary data
- ✅ **Purpose Limitation**: Document collection purposes
- ✅ **Storage Limitation**: Automatic retention (7-90 days)
- ✅ **Right to Access**: API for data export
- ✅ **Right to Erasure**: Purge capability
- ✅ **Security Measures**: Encryption, access control
- ✅ **Breach Notification**: Alerts within 72h via webhooks

**Per-Route Security Tiers:**
- **Public Gaming Services** (Pterodactyl, community sites): Strict WAF, rate limiting, PII masking, 7-day retention
- **Private Services** (Vaultwarden): Aggressive rate limiting, optional GeoIP, 30-day retention
- **Admin Routes**: Tightest security, optional IP restrictions, full audit logging, 365-day retention

---

## Feature Summary

### Total Features: 30 Tasks
- **Phase 0 (Essential)**: 3 tasks - Timeouts, size limits, headers
- **Phase 1 (Security)**: 5 tasks - Rate limiting, WAF, PII filtering, audit logs, retention
- **Phase 2 (Monitoring)**: 6 tasks - Metrics, health checks, logging, certificates
- **Phase 3 (Analytics)**: 4 tasks - Traffic analytics, GeoIP, webhooks, tracing
- **Phase 4 (Performance)**: 5 tasks - WebSockets, compression, pooling, slow detection, retries
- **Phase 5 (Dashboard)**: 4 tasks - Web UI, AI context, error pages, maintenance
- **Phase 6 (Operations)**: 3 tasks - Circuit breaker, backups, deployment


---

## Implementation Roadmap

```
Timeline: 9-10 weeks total

Phase 0: Essential Reliability (Week 1)
├─ Timeouts, size limits, security headers
└─ Foundation for all other features

Phase 1: Security & Compliance (Week 2-3)
├─ Rate limiting, WAF, PII filtering
├─ Audit logging, retention policies
└─ GDPR compliance baseline

Phase 2: Core Monitoring (Week 3-4)
├─ SQLite storage, metrics collection
├─ Health checks, certificate monitoring
└─ Request/error logging

Phase 3: Advanced Analytics (Week 5-6)
├─ Traffic analytics, GeoIP tracking
├─ Webhook notifications, request tracing
└─ AI-ready context export

Phase 4: Performance & Optimization (Week 6-7)
├─ WebSocket tracking, compression
├─ Connection pooling, slow request detection
└─ Retry logic

Phase 5: Dashboard & UX (Week 8)
├─ Web dashboard, custom error pages
└─ Maintenance mode

Phase 6: Operations & Maintenance (Week 9-10)
├─ Circuit breaker, backup system
└─ Final deployment updates
```

### Priority Matrix

| Priority | Features | Rationale |
|----------|----------|-----------|
| **P0 (Critical)** | Timeouts, size limits, headers | Prevents outages and attacks |
| **P1 (High)** | Rate limiting, WAF, health checks, logging | Security and reliability |
| **P2 (Medium)** | Analytics, webhooks, compression, dashboard | Operational visibility |
| **P3 (Low)** | Maintenance pages, retry logic | Nice-to-have improvements |

---

## Phase 0: Essential Reliability
**Timeline**: Week 1 (3-4 days)  
**Priority**: CRITICAL - Must implement before everything else

### Task #23: Timeout Configuration
**Why Critical**: Prevents hanging connections that exhaust resources and cause cascading failures.

**Configuration:**
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
type TimeoutConfig struct {
  Connect time.Duration `yaml:"connect"`  // Default: 5s
  Read    time.Duration `yaml:"read"`     // Default: 30s
  Write   time.Duration `yaml:"write"`    // Default: 30s
  Idle    time.Duration `yaml:"idle"`     // Default: 120s
}

transport := &http.Transport{
  DialContext: (&net.Dialer{
    Timeout: config.Timeouts.Connect,
  }).DialContext,
  IdleConnTimeout: config.Timeouts.Idle,
}

client := &http.Client{
  Transport: transport,
  Timeout: config.Timeouts.Read + config.Timeouts.Write,
}
```

**Metrics Tracking:**
- Count timeout errors by type (connection/read/write)
- Include timeout type in error logs
- Alert on timeout rate >5%

---

### Task #24: Request/Response Size Limits
**Why Critical**: Protects against memory exhaustion attacks and accidental large uploads.

**Configuration:**
```yaml
# Pterodactyl (file uploads for server backups)
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
limits:
  max_request_body: 104857600   # 100 MB
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

**Error Page (413.html):**
```html
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

### Task #25: Header Manipulation
**Why Critical**: Security headers are essential for GDPR compliance and preventing common web attacks.

**Configuration:**
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

## Phase 1: Security & Compliance
**Timeline**: Week 2-3 (7-10 days)  
**Priority**: CRITICAL for gaming community deployment

### Task #14: Rate Limiting System
**Why Critical**: Prevents brute-force attacks on Pterodactyl and Vaultwarden login endpoints.

**Features:**
- Per-IP rate limiting with configurable thresholds per route
- Per-route global rate limits
- Path-specific limits (e.g., `/auth/*` = 5 req/min, normal = 100 req/min)
- Failed login tracking and auto-ban
- Temporary bans with configurable duration
- Whitelist/exemption support
- Sliding window algorithm

**Configuration:**
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

### Task #15: Basic WAF (Web Application Firewall)
**Why Critical**: Protects public gaming services from common web attacks (SQLi, XSS, path traversal).

**Features:**
- SQL injection pattern detection
- XSS attempt blocking
- Path traversal prevention (`../`, `..\\`)
- Suspicious header filtering
- Request size limits (combined with Task #24)
- Per-route enable/disable
- Logging vs blocking modes (test before enforce)

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
```

**Detection Patterns:**
```go
// SQL Injection
var sqlPatterns = []string{
  `(?i)UNION.*SELECT`,
  `(?i)SELECT.*FROM`,
  `(?i)INSERT.*INTO`,
  `(?i)DROP.*TABLE`,
  `(?i)--`,
  `/\*.*\*/`,
}

// XSS
var xssPatterns = []string{
  `(?i)<script.*>`,
  `(?i)javascript:`,
  `(?i)onerror=`,
  `(?i)onload=`,
}

// Path Traversal
var pathTraversalPatterns = []string{
  `\.\.\/`,
  `\.\.\\`,
  `%2e%2e/`,
  `%2e%2e%5c`,
}
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

### Task #16: Sensitive Data Filtering (PII Masking)
**Why Critical**: GDPR requirement - must mask personally identifiable information in logs.

**Features:**
- Strip sensitive headers from logs (Authorization, Cookie, Set-Cookie, X-API-Key)
- Redact sensitive URL paths (password reset tokens, API keys)
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

// Email masking
func maskEmail(email string) string {
  parts := strings.Split(email, "@")
  if len(parts) != 2 {
    return "[EMAIL]"
  }
  username := parts[0]
  if len(username) > 2 {
    return username[:2] + "***@" + parts[1]
  }
  return "*****@" + parts[1]
}
```

---

### Task #21: Audit Log for Config Changes
**Why Critical**: Accountability and compliance - track who changed what and when.

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

**Implementation:**
```go
func logAuditEvent(user, action, resourceType, resourceID, oldValue, newValue, source string) {
  _, err := db.Exec(`
    INSERT INTO audit_log (timestamp, user, action, resource_type, resource_id, old_value, new_value, source)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
    time.Now().Unix(), user, action, resourceType, resourceID, oldValue, newValue, source)
  
  if err != nil {
    log.Error().Err(err).Msg("Failed to log audit event")
  }
}
```

---

### Task #22: Per-Route Data Retention Policies
**Why Critical**: GDPR compliance - automatic data deletion after retention period.

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
-- Add retention column to routes table
ALTER TABLE routes ADD COLUMN retention_access_logs INTEGER DEFAULT 604800;  -- 7 days
ALTER TABLE routes ADD COLUMN retention_error_logs INTEGER DEFAULT 2592000;  -- 30 days
ALTER TABLE routes ADD COLUMN retention_security_logs INTEGER DEFAULT 7776000;  -- 90 days
ALTER TABLE routes ADD COLUMN retention_metrics INTEGER DEFAULT 2592000;  -- 30 days

-- Automatic cleanup job (run daily)
DELETE FROM request_logs 
WHERE timestamp < (strftime('%s', 'now') - 
  (SELECT retention_access_logs FROM routes WHERE route_id = request_logs.route_id));

DELETE FROM waf_blocks
WHERE timestamp < (strftime('%s', 'now') - 
  (SELECT retention_security_logs FROM routes WHERE route_id = waf_blocks.route_id));
```

**Cleanup Cron:**
```bash
# Daily retention cleanup at 03:00
0 3 * * * /usr/local/bin/cleanup-retention.sh >> /var/log/retention-cleanup.log 2>&1
```

---

## Phase 2: Core Monitoring
**Timeline**: Week 3-4 (7-10 days)  
**Priority**: HIGH - Foundation for observability

### Task #10: Persistent Data Storage with SQLite
**Why Important**: Foundation for all metrics, logs, and analytics features.

**Database Location:** `/data/proxy.db`

**Go Driver:** `modernc.org/sqlite` (pure Go, no CGO)

**Complete Schema:** See [Database Schema](#database-schema) section below.

**Retention Policies:**
```sql
-- Run daily cleanup (managed by Task #22)
DELETE FROM metrics WHERE timestamp < (strftime('%s', 'now') - 2592000);  -- 30 days
DELETE FROM request_logs WHERE timestamp < (strftime('%s', 'now') - retention_period);  -- Per-route
DELETE FROM health_checks WHERE timestamp < (strftime('%s', 'now') - 2592000);  -- 30 days
```

---

### Task #1: Design Metrics Collection System
**Priority**: High (Foundation for all other features)

**Requirements:**
- Track per-request metrics: timestamp, route/domain, backend service, status code, response time, bytes sent/received
- Track system metrics: active connections, goroutine count, memory usage, uptime
- Track backend health: success rate, average response time, last successful request
- Store certificate metadata: expiry dates, days remaining, SANs
- Track security events: rate limit violations, WAF blocks, failed auth attempts, banned IPs
- Track analytics: client IP (GDPR-masked), user agent, GeoIP location, referrer

**Storage Strategy:**
- In-memory ring buffer for recent data (last 1000 requests, last 24h metrics)
- SQLite for historical data (30-day retention with auto-cleanup)
- Thread-safe access with RWMutex for concurrent reads/writes

**Database Schema (Initial):**
```sql
CREATE TABLE services (
  service_id INTEGER PRIMARY KEY,
  service_name TEXT NOT NULL,
  port INTEGER NOT NULL,
  first_seen INTEGER NOT NULL,
  last_seen INTEGER NOT NULL,
  total_requests INTEGER DEFAULT 0,
  total_errors INTEGER DEFAULT 0,
  avg_response_time_ms REAL DEFAULT 0,
  UNIQUE(service_name, port)
);

CREATE TABLE routes (
  route_id INTEGER PRIMARY KEY,
  domain TEXT NOT NULL UNIQUE,
  service_id INTEGER NOT NULL,
  config_yaml TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

CREATE TABLE metrics (
  metric_id INTEGER PRIMARY KEY,
  timestamp INTEGER NOT NULL,
  service_id INTEGER NOT NULL,
  status_code INTEGER NOT NULL,
  response_time_ms INTEGER NOT NULL,
  bytes_sent INTEGER NOT NULL,
  bytes_received INTEGER NOT NULL,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

CREATE INDEX idx_metrics_timestamp ON metrics(timestamp);
CREATE INDEX idx_metrics_service ON metrics(service_id);
```

**Retention Policy:**
- Metrics older than 30 days: DELETE automatically
- Services not seen in 90 days: Mark as inactive (keep for history)
- Aggregate hourly/daily stats for long-term trends

---

### 2. Implement Metrics API Endpoints
**Priority**: High (Required for dashboard and AI context)

**Endpoints to Create:**

**GET /api/stats**
```json
{
  "uptime_seconds": 259200,
  "active_connections": 42,
  "goroutines": 1247,
  "memory_mb": 156,
  "requests_last_24h": 128453,
  "errors_last_24h": 25,
  "error_rate": 0.019,
  "avg_response_time_ms": 67
}
```

**GET /api/routes**
```json
{
  "routes": [
    {
      "domain": "gpanel.chilla55.de",
      "backend": "pterodactyl_panel:8080",
      "health": "healthy",
      "circuit_breaker": "closed",
      "requests_24h": 45231,
      "errors_24h": 2,
      "avg_response_time_ms": 45
    }
  ]
}
```

**GET /api/certs**
```json
{
  "certificates": [
    {
      "domain": "*.chilla55.de",
      "expires_at": "2025-03-15T00:00:00Z",
      "days_remaining": 87,
      "status": "valid",
      "san_count": 1
    }
  ]
}
```

**GET /api/logs?limit=100&status=500**
```json
{
  "logs": [
    {
      "timestamp": 1734523234,
      "domain": "gpanel.chilla55.de",
      "method": "GET",
      "path": "/api/servers",
      "status": 502,
      "response_time_ms": 30012,
      "backend": "pterodactyl_panel:8080",
      "error": "backend timeout"
    }
  ]
}
```

**Security:**
- Bind to `127.0.0.1:8080` only (localhost)
- No authentication needed (SSH tunnel required for access)
- CORS disabled (not needed for same-origin)

---

### 3. Build AI-Ready Context Export
**Priority**: Medium (Troubleshooting tool)

**Endpoint:** `GET /api/ai-context?format=markdown`

**Output Format:**
```markdown
# Proxy Status Report - 2025-12-18 14:32:15 UTC

## System Overview
- Uptime: 3d 14h 23m
- Active Connections: 42
- Total Requests (24h): 128,453
- Error Rate (24h): 0.02%
- Memory Usage: 156 MB / 512 MB
- Goroutines: 1,247

## Routes Configuration

### gpanel.chilla55.de (HTTPS)
- Backend: pterodactyl_panel:8080
- Status: ✅ Healthy (last check: 2s ago)
- Certificate: Valid until 2025-03-15 (87 days remaining)
- Traffic (24h): 45,231 requests, 2.3 GB transferred
- Avg Response Time: 45ms
- Error Rate: 0.01%

### api.example.com (HTTPS + HTTP/3)
- Backend: api_service:3000
- Status: ⚠️ Degraded (50% success rate, last 1min)
- Certificate: Valid until 2025-02-01 (45 days remaining)
- Traffic (24h): 83,222 requests, 890 MB transferred
- Avg Response Time: 234ms
- Error Rate: 0.05%

## Recent Errors (Last 100)
1. [14:31:42] 502 Bad Gateway - api.example.com - Backend timeout after 30s
2. [14:29:15] TLS Handshake failed - client from 203.0.113.45
3. [14:12:03] 502 Bad Gateway - api.example.com - Connection refused

## Certificate Expiry Warnings
- ⚠️ api.example.com expires in 45 days (2025-02-01)
- ⚠️ old.site.com expires in 12 days (2025-12-30) ⚠️ URGENT

## Active Alerts
- Backend health check failing: api.example.com (50% success rate)
- High error rate on old.site.com (5 errors/min)

## Configuration Summary
- Sites Loaded: 12 routes from /etc/nginx/sites-enabled/
- Certificates: 8 domains, 6 wildcards
- Last Config Reload: 2025-12-18 12:15:33 (2h 17m ago)

---
## Suggested Analysis Questions:
- Why is api.example.com showing 50% health check failures?
- Should I be concerned about the 502 errors?
- What's causing the TLS handshake failures from that IP?
- Which certificates need renewal attention this week?

Generated for AI analysis. Current timestamp: 2025-12-18T14:32:15Z
```

**Format Parameters:**
- `format=markdown` - Human/LLM readable (default)
- `format=json` - Machine readable
- `format=prompt` - Includes suggested analysis questions

**Use Case:**
```bash
# Copy context for troubleshooting with AI
ssh -L 8080:localhost:8080 srv1
curl http://localhost:8080/api/ai-context | pbcopy
# Paste into ChatGPT/Claude with "What's wrong?"
```

---

### 4. Create Embedded Web Dashboard
**Priority**: Medium (User interface)

**Tech Stack:**
- Pure HTML/CSS/JavaScript (no framework dependencies)
- Lightweight charting library (Chart.js or similar, <50KB)
- Auto-refresh every 5 seconds via fetch API
- Responsive design (works on mobile)

**Dashboard Layout:**
```
┌─────────────────────────────────────────────┐
│  Proxy Dashboard          🔄 Auto-refresh   │
├─────────────────────────────────────────────┤
│  System Stats:                              │
│  • Uptime: 3d 14h 23m                       │
│  • Active Connections: 42                   │
│  • Requests/sec: 15.3                       │
│  • Error Rate: 0.02%                        │
├─────────────────────────────────────────────┤
│  Routes                                     │
│  ✅ gpanel.chilla55.de → pterodactyl:8080   │
│     45K req/24h | 45ms avg | 0.01% errors  │
│  ⚠️  api.example.com → api_service:3000     │
│     83K req/24h | 234ms avg | 0.05% errors │
│  📊 [View Details]                          │
├─────────────────────────────────────────────┤
│  Certificate Status                         │
│  ⚠️  api.example.com - 45 days remaining    │
│  ⚠️  old.site.com - 12 days (URGENT!)       │
├─────────────────────────────────────────────┤
│  Recent Errors (Last 10)                    │
│  [14:31] 502 - api.example.com - timeout   │
│  [14:29] TLS error - 203.0.113.45          │
├─────────────────────────────────────────────┤
│  📋 Copy AI Context | 📊 Full Metrics       │
└─────────────────────────────────────────────┘
```

**Features:**
- Real-time metrics update (WebSocket or polling)
- Status indicators (✅ healthy, ⚠️ degraded, ❌ down)
- One-click copy AI context to clipboard
- Filter logs by status code, time range
- Mobile-friendly responsive layout

**Access:**
```bash
# SSH tunnel to access dashboard
ssh -L 8080:localhost:8080 srv1
# Open browser: http://localhost:8080/dashboard
```

---

### 5. Add Backend Health Checking
**Priority**: High (Critical for circuit breaker)

**Implementation:**
- Periodic health checks every 10 seconds (configurable per route)
- HTTP HEAD or GET request to backend
- Timeout: 5 seconds (configurable)
- Track success/failure rate over sliding 1-minute window

**Health States:**
- **Healthy**: 90%+ success rate
- **Degraded**: 50-90% success rate
- **Down**: <50% success rate

**YAML Configuration:**
```yaml
# sites-available/gpanel.chilla55.de.yml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:8080
health_check:
  enabled: true
  interval: 10s
  timeout: 5s
  path: /health  # Optional: custom health endpoint
  expected_status: 200  # Or 2xx range
```

**Database Storage:**
```sql
CREATE TABLE health_checks (
  check_id INTEGER PRIMARY KEY,
  service_id INTEGER NOT NULL,
  timestamp INTEGER NOT NULL,
  success BOOLEAN NOT NULL,
  response_time_ms INTEGER,
  error_message TEXT,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

CREATE INDEX idx_health_timestamp ON health_checks(timestamp);
```

**Goroutine per Backend:**
- Each backend gets dedicated health check goroutine
- Updates service health status in real-time
- Exposes metrics to dashboard and AI context

---

### 6. Implement Request/Error Logging
**Priority**: High (Required for troubleshooting)

**Log Format (Structured JSON):**
```json
{
  "timestamp": 1734523234,
  "level": "info",
  "domain": "gpanel.chilla55.de",
  "method": "GET",
  "path": "/api/servers",
  "status": 200,
  "response_time_ms": 45,
  "backend": "pterodactyl_panel:8080",
  "client_ip": "203.0.113.45",
  "user_agent": "Mozilla/5.0...",
  "bytes_sent": 2048,
  "bytes_received": 512
}
```

**Storage Strategy:**
- In-memory ring buffer: Last 1000 requests (for /api/logs endpoint)
- SQLite: All requests for 7 days (auto-cleanup)
- Separate error log file: `/data/logs/errors.log` (rotated daily)

**Log Rotation:**
- Daily rotation at midnight
- Compress old logs (gzip)
- Keep 30 days of compressed logs
- Delete logs older than 30 days

**Access Logs Table:**
```sql
CREATE TABLE request_logs (
  log_id INTEGER PRIMARY KEY,
  timestamp INTEGER NOT NULL,
  service_id INTEGER NOT NULL,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  status_code INTEGER NOT NULL,
  response_time_ms INTEGER NOT NULL,
  client_ip TEXT,
  bytes_sent INTEGER,
  bytes_received INTEGER,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

CREATE INDEX idx_logs_timestamp ON request_logs(timestamp);
CREATE INDEX idx_logs_status ON request_logs(status_code);
```

---

### 7. Add Certificate Expiry Monitoring
**Priority**: Medium (Operational safety)

**Implementation:**
- Parse all loaded certificates on startup
- Extract expiry date from X.509 certificate
- Calculate days remaining
- Check daily for expiry warnings

**Warning Thresholds:**
- 30 days: ⚠️ Warning
- 14 days: ⚠️⚠️ Urgent
- 7 days: 🚨 Critical

**Database Storage:**
```sql
CREATE TABLE certificates (
  cert_id INTEGER PRIMARY KEY,
  domain TEXT NOT NULL UNIQUE,
  cert_path TEXT NOT NULL,
  key_path TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  san_domains TEXT,  -- JSON array
  last_checked INTEGER NOT NULL
);
```

**API Endpoint Enhancements:**
```json
GET /api/certs?warn_days=30

{
  "certificates": [
    {
      "domain": "*.chilla55.de",
      "expires_at": "2025-03-15T00:00:00Z",
      "days_remaining": 87,
      "status": "valid",
      "warning_level": "none"
    },
    {
      "domain": "old.site.com",
      "expires_at": "2025-12-30T00:00:00Z",
      "days_remaining": 12,
      "status": "expiring_soon",
      "warning_level": "urgent"
    }
  ]
}
```

**Dashboard Display:**
- Red badge for <7 days
- Orange badge for <14 days
- Yellow badge for <30 days
- Green for >30 days

---

### 8. Add Circuit Breaker for Backends
**Priority**: High (Reliability feature)

**Circuit Breaker States:**
```
CLOSED (Normal)
  ↓ (5 failures in 60s)
OPEN (Fail fast)
  ↓ (Wait 30s)
HALF-OPEN (Test recovery)
  ↓ Success → CLOSED
  ↓ Failure → OPEN
```

**Configuration:**
```yaml
# sites-available/gpanel.chilla55.de.yml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:8080
circuit_breaker:
  enabled: true
  failure_threshold: 5      # Open after N failures
  success_threshold: 2      # Close after N successes
  timeout: 30s             # Stay open for duration
  window: 60s              # Count failures in window
```

**Implementation:**
- Track failures per backend service
- Atomic counter with sliding window
- When OPEN: Return 503 immediately (custom maintenance page)
- When HALF-OPEN: Allow one test request
- Thread-safe state management

**Database Storage:**
```sql
CREATE TABLE circuit_breaker_state (
  service_id INTEGER PRIMARY KEY,
  state TEXT NOT NULL,  -- 'closed', 'open', 'half-open'
  failure_count INTEGER DEFAULT 0,
  last_failure INTEGER,
  opened_at INTEGER,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);
```

**Dashboard Indicators:**
- 🟢 CLOSED - Normal operation
- 🔴 OPEN - Circuit open (failing fast)
- 🟡 HALF-OPEN - Testing recovery

---

### 9. Add Custom Error Pages
**Priority**: Medium (User experience)

**Implementation:**
- Per-route or global error pages
- Template support with variables
- Fallback to default error pages

**Configuration:**
```yaml
# sites-available/gpanel.chilla55.de.yml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:8080
error_pages:
  404: /etc/nginx/error-pages/404.html
  502: /etc/nginx/error-pages/maintenance.html
  503: /etc/nginx/error-pages/maintenance.html
```

**Template Variables:**
```html
<!DOCTYPE html>
<html>
<head>
  <title>{{.StatusCode}} - {{.StatusText}}</title>
</head>
<body>
  <h1>{{.StatusCode}} {{.StatusText}}</h1>
  <p>{{.Message}}</p>
  <p>Route: {{.Route}}</p>
  <p>Timestamp: {{.Timestamp}}</p>
</body>
</html>
```

**Database Storage:**
```sql
CREATE TABLE error_pages (
  page_id INTEGER PRIMARY KEY,
  route_id INTEGER,  -- NULL = global
  status_code INTEGER NOT NULL,
  html_content TEXT NOT NULL,
  last_modified INTEGER NOT NULL,
  FOREIGN KEY(route_id) REFERENCES routes(route_id),
  UNIQUE(route_id, status_code)
);
```

**Default Pages Embedded:**
- Include default HTML templates in binary (go:embed)
- Allow override via filesystem or database

---

### 10. Add Persistent Data Storage with SQLite
**Priority**: High (Foundation)

**Database Location:** `/data/proxy.db`

**Complete Schema:**
```sql
-- Service registry
CREATE TABLE services (
  service_id INTEGER PRIMARY KEY,
  service_name TEXT NOT NULL,
  port INTEGER NOT NULL,
  first_seen INTEGER NOT NULL,
  last_seen INTEGER NOT NULL,
  total_requests INTEGER DEFAULT 0,
  total_errors INTEGER DEFAULT 0,
  avg_response_time_ms REAL DEFAULT 0,
  UNIQUE(service_name, port)
);

-- Route configurations
CREATE TABLE routes (
  route_id INTEGER PRIMARY KEY,
  domain TEXT NOT NULL UNIQUE,
  service_id INTEGER NOT NULL,
  config_yaml TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

-- Time-series metrics
CREATE TABLE metrics (
  metric_id INTEGER PRIMARY KEY,
  timestamp INTEGER NOT NULL,
  service_id INTEGER NOT NULL,
  status_code INTEGER NOT NULL,
  response_time_ms INTEGER NOT NULL,
  bytes_sent INTEGER NOT NULL,
  bytes_received INTEGER NOT NULL,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

-- Request logs
CREATE TABLE request_logs (
  log_id INTEGER PRIMARY KEY,
  timestamp INTEGER NOT NULL,
  service_id INTEGER NOT NULL,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  status_code INTEGER NOT NULL,
  response_time_ms INTEGER NOT NULL,
  client_ip TEXT,
  bytes_sent INTEGER,
  bytes_received INTEGER,
  error_message TEXT,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

-- Health checks
CREATE TABLE health_checks (
  check_id INTEGER PRIMARY KEY,
  service_id INTEGER NOT NULL,
  timestamp INTEGER NOT NULL,
  success BOOLEAN NOT NULL,
  response_time_ms INTEGER,
  error_message TEXT,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

-- Certificates
CREATE TABLE certificates (
  cert_id INTEGER PRIMARY KEY,
  domain TEXT NOT NULL UNIQUE,
  cert_path TEXT NOT NULL,
  key_path TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  san_domains TEXT,
  last_checked INTEGER NOT NULL
);

-- Error pages
CREATE TABLE error_pages (
  page_id INTEGER PRIMARY KEY,
  route_id INTEGER,
  status_code INTEGER NOT NULL,
  html_content TEXT NOT NULL,
  last_modified INTEGER NOT NULL,
  FOREIGN KEY(route_id) REFERENCES routes(route_id),
  UNIQUE(route_id, status_code)
);

-- Maintenance pages
CREATE TABLE maintenance_pages (
  page_id INTEGER PRIMARY KEY,
  route_id INTEGER NOT NULL,
  html_content TEXT NOT NULL,
  scheduled_start INTEGER,
  scheduled_end INTEGER,
  is_active BOOLEAN DEFAULT 0,
  created_at INTEGER NOT NULL,
  FOREIGN KEY(route_id) REFERENCES routes(route_id)
);

-- Circuit breaker state
CREATE TABLE circuit_breaker_state (
  service_id INTEGER PRIMARY KEY,
  state TEXT NOT NULL,
  failure_count INTEGER DEFAULT 0,
  last_failure INTEGER,
  opened_at INTEGER,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

-- Rate limiting (future)
CREATE TABLE rate_limits (
  limit_id INTEGER PRIMARY KEY,
  ip_address TEXT NOT NULL UNIQUE,
  request_count INTEGER DEFAULT 0,
  window_start INTEGER NOT NULL,
  blocked_until INTEGER
);

-- Indexes for performance
CREATE INDEX idx_metrics_timestamp ON metrics(timestamp);
CREATE INDEX idx_metrics_service ON metrics(service_id);
CREATE INDEX idx_logs_timestamp ON request_logs(timestamp);
CREATE INDEX idx_logs_status ON request_logs(status_code);
CREATE INDEX idx_health_timestamp ON health_checks(timestamp);
```

**Retention Policies:**
```sql
-- Run daily cleanup
DELETE FROM metrics WHERE timestamp < (strftime('%s', 'now') - 2592000);  -- 30 days
DELETE FROM request_logs WHERE timestamp < (strftime('%s', 'now') - 604800);  -- 7 days
DELETE FROM health_checks WHERE timestamp < (strftime('%s', 'now') - 2592000);  -- 30 days
```

**Go Driver:** Use `modernc.org/sqlite` (pure Go, no CGO)

---

### 11. Add Maintenance Page Storage
**Priority**: Low (Nice to have)

**Use Cases:**
- Scheduled maintenance windows
- Backend down (circuit breaker open)
- Manual maintenance mode

**Configuration:**
```yaml
# Via API or dashboard
POST /api/maintenance
{
  "route": "gpanel.chilla55.de",
  "html_content": "<h1>Maintenance in progress</h1>",
  "scheduled_start": "2025-12-20T02:00:00Z",
  "scheduled_end": "2025-12-20T04:00:00Z"
}
```

**Automatic Activation:**
- When circuit breaker opens: Show maintenance page instead of 503
- When scheduled time arrives: Activate maintenance mode
- When scheduled end time: Deactivate automatically

**Database:**
```sql
CREATE TABLE maintenance_pages (
  page_id INTEGER PRIMARY KEY,
  route_id INTEGER NOT NULL,
  html_content TEXT NOT NULL,
  scheduled_start INTEGER,
  scheduled_end INTEGER,
  is_active BOOLEAN DEFAULT 0,
  created_at INTEGER NOT NULL,
  FOREIGN KEY(route_id) REFERENCES routes(route_id)
);
```

---

### 12. Add Three-Tier SQLite Backup System
**Priority**: High (Data protection)

**Backup Strategy:**

**1. Full Backup (Weekly - Sunday 02:00)**
```bash
#!/bin/bash
# backup-full.sh

BACKUP_DIR="/mnt/storagebox/backups/proxy"
DATE=$(date +%Y%m%d_%H%M%S)

# Hot backup using SQLite VACUUM INTO
sqlite3 /data/proxy.db "VACUUM INTO '$BACKUP_DIR/full-$DATE.db'"

# Compress
gzip "$BACKUP_DIR/full-$DATE.db"

# Keep last 4 weekly full backups
ls -t "$BACKUP_DIR"/full-*.db.gz | tail -n +5 | xargs rm -f

echo "Full backup completed: full-$DATE.db.gz"
```

**2. Differential Backup (Mid-Week - Wednesday 02:00)**
```bash
#!/bin/bash
# backup-differential.sh

BACKUP_DIR="/mnt/storagebox/backups/proxy"
LAST_FULL=$(ls -t "$BACKUP_DIR"/full-*.db.gz | head -1)
DATE=$(date +%Y%m%d_%H%M%S)

# Extract last full backup for comparison
gunzip -c "$LAST_FULL" > /tmp/last-full.db

# Create differential using rsync with hardlinks
rsync -a --link-dest=/tmp/last-full.db \
  /data/proxy.db "$BACKUP_DIR/diff-$DATE.db"

# Compress
gzip "$BACKUP_DIR/diff-$DATE.db"

# Keep last 8 differential backups
ls -t "$BACKUP_DIR"/diff-*.db.gz | tail -n +9 | xargs rm -f

rm /tmp/last-full.db
echo "Differential backup completed: diff-$DATE.db.gz"
```

**3. Incremental Backup (Hourly - If Changed)**
```bash
#!/bin/bash
# backup-incremental.sh

BACKUP_DIR="/mnt/storagebox/backups/proxy"
DATE=$(date +%Y%m%d_%H%M%S)
HASH_FILE="/data/.db-hash"

# Calculate current hash
CURRENT_HASH=$(md5sum /data/proxy.db | awk '{print $1}')

# Read last hash
if [ -f "$HASH_FILE" ]; then
  LAST_HASH=$(cat "$HASH_FILE")
else
  LAST_HASH=""
fi

# Only backup if changed
if [ "$CURRENT_HASH" != "$LAST_HASH" ]; then
  # Find last backup (full, diff, or incr)
  LAST_BACKUP=$(ls -t "$BACKUP_DIR"/{full,diff,incr}-*.db.gz 2>/dev/null | head -1)
  
  if [ -n "$LAST_BACKUP" ]; then
    gunzip -c "$LAST_BACKUP" > /tmp/last-backup.db
    
    # Create incremental using rsync
    rsync -a --link-dest=/tmp/last-backup.db \
      /data/proxy.db "$BACKUP_DIR/incr-$DATE.db"
    
    gzip "$BACKUP_DIR/incr-$DATE.db"
    rm /tmp/last-backup.db
  else
    # No previous backup, create full
    sqlite3 /data/proxy.db "VACUUM INTO '$BACKUP_DIR/incr-$DATE.db'"
    gzip "$BACKUP_DIR/incr-$DATE.db"
  fi
  
  # Update hash
  echo "$CURRENT_HASH" > "$HASH_FILE"
  
  # Keep last 168 hourly backups (7 days)
  ls -t "$BACKUP_DIR"/incr-*.db.gz | tail -n +169 | xargs rm -f
  
  echo "Incremental backup completed: incr-$DATE.db.gz"
else
  echo "No changes detected, skipping backup"
fi
```

**Cron Schedule:**
```cron
# Full backup - Every Sunday at 02:00
0 2 * * 0 /usr/local/bin/backup-full.sh >> /var/log/backup.log 2>&1

# Differential backup - Every Wednesday at 02:00
0 2 * * 3 /usr/local/bin/backup-differential.sh >> /var/log/backup.log 2>&1

# Incremental backup - Every hour
0 * * * * /usr/local/bin/backup-incremental.sh >> /var/log/backup.log 2>&1
```

**Restore Script:**
```bash
#!/bin/bash
# backup-restore.sh

BACKUP_DIR="/mnt/storagebox/backups/proxy"
RESTORE_DATE="$1"  # Format: 20251218_143000

if [ -z "$RESTORE_DATE" ]; then
  echo "Usage: $0 <restore_date>"
  echo "Available backups:"
  ls -lh "$BACKUP_DIR"/{full,diff,incr}-*.db.gz
  exit 1
fi

# Find the backup chain needed
# 1. Find last full backup before restore date
FULL_BACKUP=$(ls -t "$BACKUP_DIR"/full-*.db.gz | awk -v date="$RESTORE_DATE" '$0 ~ date || $0 < date' | head -1)

if [ -z "$FULL_BACKUP" ]; then
  echo "No full backup found before $RESTORE_DATE"
  exit 1
fi

echo "Using full backup: $FULL_BACKUP"
gunzip -c "$FULL_BACKUP" > /tmp/restore.db

# 2. Find differential backup if exists
DIFF_BACKUP=$(ls -t "$BACKUP_DIR"/diff-*.db.gz | awk -v date="$RESTORE_DATE" '$0 ~ date || $0 < date' | head -1)

if [ -n "$DIFF_BACKUP" ]; then
  echo "Applying differential: $DIFF_BACKUP"
  # Apply differential changes
  sqlite3 /tmp/restore.db ".restore $DIFF_BACKUP"
fi

# 3. Apply incremental backups in order
for INCR in $(ls -t "$BACKUP_DIR"/incr-*.db.gz | awk -v date="$RESTORE_DATE" '$0 ~ date || $0 < date'); do
  echo "Applying incremental: $INCR"
  sqlite3 /tmp/restore.db ".restore $INCR"
done

# 4. Replace current database
echo "Stopping proxy service..."
docker service scale proxy=0

echo "Restoring database..."
cp /data/proxy.db /data/proxy.db.backup-$(date +%Y%m%d_%H%M%S)
cp /tmp/restore.db /data/proxy.db

echo "Starting proxy service..."
docker service scale proxy=1

rm /tmp/restore.db
echo "Restore completed!"
```

**Storage Calculation:**
```
Weekly:
- 1 full backup: 5 MB
- 1 differential: 2.5 MB
- 84 hourly incrementals (50% with changes): 42 × 80 KB = 3.4 MB
Total per week: ~11 MB

Retention (4 weeks):
- 4 full backups: 20 MB
- 8 differential backups: 20 MB
- 168 hourly backups: 13.5 MB
Total storage: ~53.5 MB
```

---

### 13. Update Dockerfile and Deployment
**Priority**: High (Infrastructure)

**Dockerfile Updates:**
```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o proxy-manager .

FROM alpine:latest

# Install SQLite and backup tools
RUN apk add --no-cache \
    ca-certificates \
    sqlite \
    rsync \
    gzip \
    tzdata

# Create proxy user
RUN addgroup -g 1000 proxy && \
    adduser -D -u 1000 -G proxy proxy

# Create directories
RUN mkdir -p /data /etc/nginx/sites-enabled /etc/nginx/error-pages && \
    chown -R proxy:proxy /data

COPY --from=builder /build/proxy-manager /usr/local/bin/
COPY --chmod=755 backup-*.sh /usr/local/bin/

USER proxy

# Metrics API port (localhost only)
EXPOSE 8080

CMD ["/usr/local/bin/proxy-manager"]
```

**docker-compose.swarm.yml:**
```yaml
version: '3.8'

services:
  proxy:
    image: ghcr.io/chilla55/proxy-manager:latest
    networks:
      - web
    ports:
      - target: 80
        published: 80
        mode: host
      - target: 443
        published: 443
        mode: host
      - target: 443
        published: 443
        protocol: udp
        mode: host
      # Metrics port NOT published (localhost only inside container)
    volumes:
      - type: bind
        source: /mnt/storagebox/certs
        target: /certs
        read_only: true
        bind:
          propagation: rslave
      - type: bind
        source: /mnt/storagebox/sites
        target: /etc/nginx/sites-enabled
        read_only: true
        bind:
          propagation: rslave
      - type: volume
        source: proxy-data
        target: /data
    environment:
      - TZ=Europe/Berlin
    deploy:
      mode: replicated
      replicas: 1
      placement:
        constraints:
          - node.labels.web == true
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
      resources:
        limits:
          memory: 512M
        reservations:
          memory: 256M
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/api/stats"]
      interval: 10s
      timeout: 5s
      retries: 3

volumes:
  proxy-data:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: /mnt/storagebox/proxy-data

networks:
  web:
    external: true
```

**Accessing Dashboard:**
```bash
# Create SSH tunnel to access metrics API
ssh -L 8080:localhost:8080 srv1

# Open browser
open http://localhost:8080/dashboard

# Or curl API
curl http://localhost:8080/api/stats
curl http://localhost:8080/api/ai-context
```

**Backup Cron Setup (on Docker host):**
```bash
# Add to host crontab
sudo crontab -e

# Full backup - Every Sunday at 02:00
0 2 * * 0 docker exec proxy_proxy.1.$(docker service ps -q proxy | head -1) /usr/local/bin/backup-full.sh

# Differential - Every Wednesday at 02:00
0 2 * * 3 docker exec proxy_proxy.1.$(docker service ps -q proxy | head -1) /usr/local/bin/backup-differential.sh

# Incremental - Every hour
0 * * * * docker exec proxy_proxy.1.$(docker service ps -q proxy | head -1) /usr/local/bin/backup-incremental.sh
```

---

## Implementation Priority Order

### Phase 1: Foundation (Week 1-2)
1. **Persistent Storage (Task 10)** - SQLite setup, schema creation
2. **Metrics Collection (Task 1)** - In-memory + SQLite storage
3. **API Endpoints (Task 2)** - Basic stats, routes, certs APIs
4. **Request Logging (Task 6)** - Structured logging to SQLite

### Phase 2: Monitoring (Week 3-4)
5. **Backend Health Checking (Task 5)** - Periodic health probes
6. **Certificate Monitoring (Task 7)** - Expiry tracking and warnings
7. **AI Context Export (Task 3)** - LLM-ready status reports
8. **Web Dashboard (Task 4)** - Simple HTML/JS interface

### Phase 3: Reliability (Week 5-6)
9. **Circuit Breaker (Task 8)** - Fail-fast pattern
10. **Custom Error Pages (Task 9)** - Per-route error handling
11. **Maintenance Pages (Task 11)** - Scheduled maintenance mode

### Phase 4: Operations (Week 7)
12. **Backup System (Task 12)** - Three-tier backup strategy
13. **Deployment (Task 13)** - Dockerfile, docker-compose, cron

---

## Testing Strategy

### Unit Tests
- SQLite operations (CRUD, queries, retention cleanup)
- Metrics collection and aggregation
- Circuit breaker state transitions
- Health check logic

### Integration Tests
- Full request flow with metrics collection
- Database persistence across restarts
- Backup and restore procedures
- API endpoints with real data

### Load Tests
- Measure performance impact of logging
- Test SQLite under concurrent writes
- Verify metrics collection at scale (10k req/s)

### Manual Tests
- Dashboard UI in browser
- SSH tunnel access
- AI context export with real scenarios
- Certificate expiry warnings

---

## Security Considerations

1. **Metrics API**: Localhost-only binding (127.0.0.1:8080)
2. **Database**: File permissions 600, owned by proxy user
3. **Backups**: Encrypted at rest on storagebox
4. **Logs**: PII redaction (IP addresses optional, no auth tokens)
5. **Error Pages**: No sensitive information leakage
6. **Dashboard**: No authentication (SSH tunnel required)

---

## Future Enhancements (Post-MVP)

- **Rate Limiting**: Per-IP request throttling
- **GeoIP**: Location tracking for access logs
- **Prometheus Export**: `/metrics` endpoint for Grafana
- **WebSocket Tracking**: Active WS connection monitoring
- **Request Replay**: Test backend with captured requests
- **Grafana Dashboards**: Pre-built visualization templates
- **Alerting**: Email/webhook notifications for critical events
- **Multi-proxy Support**: PostgreSQL for distributed deployments

---

## Documentation Requirements

1. **API Documentation**: OpenAPI/Swagger spec for all endpoints
2. **Database Schema**: ER diagram and table descriptions
3. **Backup/Restore Guide**: Step-by-step recovery procedures
4. **Dashboard User Guide**: Screenshots and usage instructions
5. **Troubleshooting Guide**: Common issues and solutions
6. **Performance Tuning**: Optimization tips for large deployments

---

## Success Criteria

- ✅ Dashboard accessible via SSH tunnel
- ✅ Real-time metrics updating every 5 seconds
- ✅ AI context export generates useful troubleshooting reports
- ✅ Circuit breaker prevents cascading failures
- ✅ Certificate expiry warnings 30 days in advance
- ✅ Backup/restore tested successfully
- ✅ No performance degradation (<5ms overhead per request)
- ✅ SQLite database stays under 100 MB with 30-day retention
- ✅ All tests passing (unit, integration, load)
- ✅ Documentation complete and reviewed

---

## Contact & Support

- Repository: `github.com/chilla55/docker-images`
- Project Path: `/go-proxy/proxy-manager/`
- Documentation: See `README.md`, `QUICKSTART.md`
- Issues: GitHub Issues or direct communication

---

*This implementation plan was generated through collaborative brainstorming on December 18, 2025. All architectural decisions, technology choices, and implementation details have been carefully considered for the specific deployment environment (Docker Swarm, Hetzner Storagebox, production web services).*
