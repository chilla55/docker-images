# Go Proxy Implementation Plan

**Version**: 2.0  
**Last Updated**: December 18, 2025  
**Total Features**: 30 tasks across 6 phases
**Status**: Phases 0–6 implemented (circuit breaker + backups landed)

---

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Implementation Roadmap](#2-implementation-roadmap)
3. [Phase 0: Essential Reliability](#3-phase-0-essential-reliability-week-1)
4. [Phase 1: Security & Compliance](#4-phase-1-security--compliance-weeks-2-3)
5. [Phase 2: Core Monitoring](#5-phase-2-core-monitoring-weeks-3-4)
6. [Phase 3: Advanced Analytics](#6-phase-3-advanced-analytics-weeks-5-6)
7. [Phase 4: Performance & Optimization](#7-phase-4-performance--optimization-weeks-6-7)
8. [Phase 5: Dashboard & UX](#8-phase-5-dashboard--ux-week-8)
9. [Phase 6: Operations & Maintenance](#9-phase-6-operations--maintenance-weeks-9-10)
10. [Complete Database Schema](#10-complete-database-schema)
11. [Deployment Guide](#11-deployment-guide)
12. [Testing Strategy](#12-testing-strategy)

---

## 1. Project Overview

### 1.1 Current State

This Go-based reverse proxy (`proxy-manager`) replaces nginx for HTTP/HTTPS traffic routing with enhanced observability and security features.

**Technical Stack:**
- **Language**: Go 1.21+ (pure Go, no CGO)
- **Protocols**: HTTP/1.1, HTTP/2, HTTP/3 (QUIC)
- **TLS**: Hot-reload via fsnotify watching `/mnt/storagebox/certs/`
- **Configuration**: YAML files in `/etc/proxy/sites-available/`
- **Deployment**: Docker Swarm on Hetzner dedicated server (Germany)
- **Storage**: SQLite (modernc.org/sqlite) at `/data/proxy.db`
- **Backend Discovery**: Docker service names (persistent across IP changes)

### 1.2 Deployment Context

| Aspect | Details |
|--------|---------|
| **Location** | Hetzner dedicated server, Germany |
| **User Base** | 20+ international gaming community members |
| **Public Services** | Pterodactyl panel, community websites/forums |
| **Private Services** | Vaultwarden (personal use only) |
| **Administrator** | Single admin managing infrastructure |
| **Compliance** | GDPR required for EU users |
| **Traffic** | HTTP/HTTPS only (game servers bypass proxy) |
| **Backend Architecture** | Single-instance containers |

### 1.3 Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Dashboard Access** | `localhost:8080` via SSH tunnel | Security without auth complexity |
| **Data Storage** | SQLite embedded database | No external dependencies, simple backups |
| **Service Tracking** | Docker name + port | Stable across container restarts |
| **Backup Strategy** | Three-tier (full/diff/incr) | ~53.5 MB storage, 4-week retention |
| **Proxy Scope** | HTTP/HTTPS only | WebSockets supported, no raw TCP |
| **Security Model** | Per-route configuration | Public vs private service separation |
| **Logging** | GDPR-compliant with PII masking | 7-90 day retention per route type |

### 1.4 Security & Compliance Requirements

**GDPR Compliance Checklist:**
- ✅ **Data Minimization**: Only log necessary data
- ✅ **Purpose Limitation**: Document what data is collected and why
- ✅ **Storage Limitation**: Automatic retention policies (7-90 days)
- ✅ **Right to Access**: Users can export their data via API
- ✅ **Right to Erasure**: Purge user data on request
- ✅ **Security Measures**: Encryption at rest, access controls
- ✅ **Breach Notification**: Alert procedures within 72h via webhooks

**Security Tier Matrix:**

| Service Type | Examples | Rate Limit | WAF | PII Masking | Retention | Audit Logs |
|--------------|----------|------------|-----|-------------|-----------|------------|
| **Public Gaming** | Pterodactyl, community sites | Moderate | Strict | Required | 7 days | Standard |
| **Private** | Vaultwarden | Aggressive | Moderate | Required | 30 days | Enhanced |
| **Admin** | Dashboard, management | Tightest | Strict | Optional | 365 days | Full |

---

## 2. Implementation Roadmap

### 2.1 Timeline Overview

```
Total Duration: 9-10 weeks

Week 1: Phase 0 - Essential Reliability
├─ Timeouts, size limits, security headers
└─ Foundation for all features

Weeks 2-3: Phase 1 - Security & Compliance
├─ Rate limiting, WAF, PII filtering
├─ Audit logging, retention policies
└─ GDPR compliance complete

Weeks 3-4: Phase 2 - Core Monitoring  
├─ SQLite storage, metrics collection
├─ Health checks, certificate monitoring
└─ Request/error logging

Weeks 5-6: Phase 3 - Advanced Analytics
├─ Traffic analytics, GeoIP tracking
├─ Webhook notifications, request tracing
└─ AI-ready context export

Weeks 6-7: Phase 4 - Performance & Optimization
├─ WebSocket tracking, compression
├─ Connection pooling, slow request detection
└─ Retry logic

Week 8: Phase 5 - Dashboard & UX
├─ Web dashboard, custom error pages
└─ Maintenance mode

Weeks 9-10: Phase 6 - Operations & Maintenance
├─ Circuit breaker, backup system
└─ Final deployment & testing
```

### 2.2 Priority Matrix

| Priority | Phase | Features | Rationale |
|----------|-------|----------|-----------|
| **P0 (Critical)** | 0 | Timeouts, size limits, headers | Prevents outages and basic attacks |
| **P1 (High)** | 1, 2 | Security, health checks, logging | Core security and reliability |
| **P2 (Medium)** | 3, 4 | Analytics, compression, webhooks | Operational visibility & performance |
| **P3 (Low)** | 5, 6 | Dashboard, maintenance, backups | Nice-to-have improvements |

### 2.3 Feature Summary

**Total: 30 Tasks**

- **Phase 0**: 3 tasks - Essential reliability foundations
- **Phase 1**: 5 tasks - Security and GDPR compliance
- **Phase 2**: 6 tasks - Core monitoring and storage
- **Phase 3**: 4 tasks - Advanced analytics and notifications
- **Phase 4**: 5 tasks - Performance optimizations
- **Phase 5**: 4 tasks - User interface and experience
- **Phase 6**: 3 tasks - Operations and maintenance

**Critical Missing Items (To Add):**
- ✅ Graceful shutdown handling (SIGTERM/SIGINT)
- ✅ Configuration validation on startup
- ✅ Environment variable support
- ✅ Structured logging with levels
- ✅ Prometheus metrics export (optional)
- ✅ Hot-reload configuration changes (already partially implemented)

---

## 3. Phase 0: Essential Reliability (Week 1)

**Duration**: 3-4 days  
**Priority**: P0 - CRITICAL  
**Why First**: Prevents catastrophic failures before adding features

### Task #0: Foundation Setup (NEW)

**Required Before Implementation:**

#### 1. Graceful Shutdown Handling
```go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"
)

func main() {
    // Create context for shutdown
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // Setup signal handlers
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
    
    // Start proxy server
    srv := &http.Server{Addr: ":80"}
    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatal().Err(err).Msg("Server failed")
        }
    }()
    
    // Wait for shutdown signal
    <-sigChan
    log.Info().Msg("Shutdown signal received, gracefully stopping...")
    
    // Graceful shutdown with timeout
    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer shutdownCancel()
    
    // Close active connections gracefully
    if err := srv.Shutdown(shutdownCtx); err != nil {
        log.Error().Err(err).Msg("Forced shutdown")
    }
    
    // Close database connections
    if db != nil {
        db.Close()
    }
    
    log.Info().Msg("Shutdown complete")
}
```

#### 2. Configuration Validation on Startup
```go
type Config struct {
    ListenAddr      string        `yaml:"listen_addr"`
    MetricsAddr     string        `yaml:"metrics_addr"`
    DatabasePath    string        `yaml:"database_path"`
    SitesDir        string        `yaml:"sites_dir"`
    CertsDir        string        `yaml:"certs_dir"`
    ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

func validateConfig(cfg *Config) error {
    // Check required directories exist
    if _, err := os.Stat(cfg.SitesDir); os.IsNotExist(err) {
        return fmt.Errorf("sites directory does not exist: %s", cfg.SitesDir)
    }
    
    if _, err := os.Stat(cfg.CertsDir); os.IsNotExist(err) {
        return fmt.Errorf("certs directory does not exist: %s", cfg.CertsDir)
    }
    
    // Check database directory is writable
    dbDir := filepath.Dir(cfg.DatabasePath)
    if _, err := os.Stat(dbDir); os.IsNotExist(err) {
        return fmt.Errorf("database directory does not exist: %s", dbDir)
    }
    
    // Validate listen addresses
    if cfg.ListenAddr == "" {
        cfg.ListenAddr = ":80"
    }
    
    if cfg.MetricsAddr == "" {
        cfg.MetricsAddr = "127.0.0.1:8080"
    }
    
    return nil
}

func main() {
    // Load config
    cfg, err := loadConfig("/etc/proxy/config.yml")
    if err != nil {
        log.Fatal().Err(err).Msg("Failed to load config")
    }
    
    // Validate config
    if err := validateConfig(cfg); err != nil {
        log.Fatal().Err(err).Msg("Invalid configuration")
    }
    
    log.Info().Msg("Configuration validated successfully")
    
    // Continue with startup...
}
```

#### 3. Environment Variable Support
```go
func loadConfig(path string) (*Config, error) {
    cfg := &Config{
        // Defaults
        ListenAddr:      getEnv("PROXY_LISTEN_ADDR", ":80"),
        MetricsAddr:     getEnv("PROXY_METRICS_ADDR", "127.0.0.1:8080"),
        DatabasePath:    getEnv("PROXY_DB_PATH", "/data/proxy.db"),
        SitesDir:        getEnv("PROXY_SITES_DIR", "/etc/proxy/sites-available"),
        CertsDir:        getEnv("PROXY_CERTS_DIR", "/mnt/storagebox/certs"),
        ShutdownTimeout: getDurationEnv("PROXY_SHUTDOWN_TIMEOUT", 30*time.Second),
    }
    
    // Override with config file if exists
    if _, err := os.Stat(path); err == nil {
        data, err := os.ReadFile(path)
        if err != nil {
            return nil, err
        }
        if err := yaml.Unmarshal(data, cfg); err != nil {
            return nil, err
        }
    }
    
    return cfg, nil
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
    if value := os.Getenv(key); value != "" {
        if d, err := time.ParseDuration(value); err == nil {
            return d
        }
    }
    return defaultValue
}
```

#### 4. Structured Logging with Levels
```go
import "github.com/rs/zerolog"

func setupLogging() {
    // Set log level from environment
    logLevel := getEnv("LOG_LEVEL", "info")
    
    switch logLevel {
    case "debug":
        zerolog.SetGlobalLevel(zerolog.DebugLevel)
    case "info":
        zerolog.SetGlobalLevel(zerolog.InfoLevel)
    case "warn":
        zerolog.SetGlobalLevel(zerolog.WarnLevel)
    case "error":
        zerolog.SetGlobalLevel(zerolog.ErrorLevel)
    default:
        zerolog.SetGlobalLevel(zerolog.InfoLevel)
    }
    
    // Pretty logging for development
    if getEnv("LOG_FORMAT", "json") == "pretty" {
        log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
    }
    
    log.Info().
        Str("version", version).
        Str("log_level", logLevel).
        Msg("Proxy manager starting")
}
```

#### 5. Startup Health Checks
```go
func performStartupChecks(cfg *Config) error {
    log.Info().Msg("Performing startup health checks...")
    
    // 1. Check database connection
    db, err := sql.Open("sqlite", cfg.DatabasePath)
    if err != nil {
        return fmt.Errorf("database connection failed: %w", err)
    }
    if err := db.Ping(); err != nil {
        return fmt.Errorf("database ping failed: %w", err)
    }
    db.Close()
    
    // 2. Check sites configuration
    sites, err := loadSites(cfg.SitesDir)
    if err != nil {
        return fmt.Errorf("failed to load sites: %w", err)
    }
    if len(sites) == 0 {
        log.Warn().Msg("No sites configured")
    } else {
        log.Info().Int("count", len(sites)).Msg("Sites loaded")
    }
    
    // 3. Check certificates
    certs, err := loadCertificates(cfg.CertsDir)
    if err != nil {
        return fmt.Errorf("failed to load certificates: %w", err)
    }
    if len(certs) == 0 {
        log.Warn().Msg("No certificates found")
    } else {
        log.Info().Int("count", len(certs)).Msg("Certificates loaded")
    }
    
    // 4. Check port availability
    listener, err := net.Listen("tcp", cfg.ListenAddr)
    if err != nil {
        return fmt.Errorf("cannot bind to %s: %w", cfg.ListenAddr, err)
    }
    listener.Close()
    
    log.Info().Msg("Startup health checks passed")
    return nil
}
```

**Environment Variables:**
```bash
# Docker environment variables
PROXY_LISTEN_ADDR=:80
PROXY_METRICS_ADDR=127.0.0.1:8080
PROXY_DB_PATH=/data/proxy.db
PROXY_SITES_DIR=/etc/proxy/sites-available
PROXY_CERTS_DIR=/mnt/storagebox/certs
PROXY_SHUTDOWN_TIMEOUT=30s
LOG_LEVEL=info
LOG_FORMAT=json
TZ=Europe/Berlin
```

---

### Task #23: Timeout Configuration

**Problem**: Without timeouts, a slow backend can exhaust connections and cause cascading failures.

**Configuration:**
```yaml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
timeouts:
  connect: 5s      # Backend connection timeout
  read: 30s        # Read from backend
  write: 30s       # Write to backend
  idle: 120s       # Keep-alive idle timeout

# Slower backend example
host: api.example.com
backend: http://slow_service:3000
timeouts:
  connect: 10s
  read: 60s
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

**Metrics:**
- Count timeout errors by type (connection/read/write)
- Alert on timeout rate >5%

---

### Task #24: Request/Response Size Limits

**Problem**: Large requests can exhaust memory; protects against DoS attacks.

**Configuration:**
```yaml
# Pterodactyl (allows server backup uploads)
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
limits:
  max_request_body: 104857600   # 100 MB
  max_request_headers: 16384    # 16 KB
  max_response_body: 52428800   # 50 MB
  
# Vaultwarden (strict)
host: vault.chilla55.de
backend: http://vaultwarden:80
limits:
  max_request_body: 10485760    # 10 MB
  max_request_headers: 8192     # 8 KB
  max_response_body: 10485760   # 10 MB
```

**Implementation:**
```go
// Request body limit
limitedReader := io.LimitReader(r.Body, config.Limits.MaxRequestBody)
if limitedReader.(*io.LimitedReader).N == 0 {
  http.Error(w, "413 Payload Too Large", http.StatusRequestEntityTooLarge)
  return
}

// Response body limit
responseReader := io.LimitReader(resp.Body, config.Limits.MaxResponseBody)
```

---

### Task #25: Header Manipulation

**Problem**: Security headers prevent XSS, clickjacking, and other attacks. Required for GDPR compliance.

**Configuration:**
```yaml
# Pterodactyl (public, security headers required)
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
headers:
  request:
    add:
      X-Forwarded-Proto: https
      X-Real-IP: ${client_ip}
  response:
    add:
      Strict-Transport-Security: "max-age=31536000; includeSubDomains"
      X-Frame-Options: "SAMEORIGIN"
      X-Content-Type-Options: "nosniff"
      X-XSS-Protection: "1; mode=block"
      Referrer-Policy: "strict-origin-when-cross-origin"
    remove:
      - Server
      - X-Powered-By
```

**Security Presets:**
```yaml
headers:
  preset: security-strict
  # Expands to: HSTS, CSP, X-Frame-Options: DENY, etc.
```

---

## 4. Phase 1: Security & Compliance (Weeks 2-3)

**Duration**: 7-10 days  
**Priority**: P1 - HIGH  
**Goal**: Protect gaming community from attacks, achieve GDPR compliance

### Task #14: Rate Limiting System

**Problem**: Brute-force attacks on Pterodactyl and Vaultwarden login endpoints.

**Configuration:**
```yaml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:80
rate_limit:
  global: 1000  # req/min for entire route
  per_ip: 100   # req/min per IP
  paths:
    - path: /auth/*
      per_ip: 5
      ban_after: 3      # Ban after 3 violations
      ban_duration: 3600 # 1 hour
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
```

---

### Task #15: Basic WAF (Web Application Firewall)

**Problem**: Public gaming services vulnerable to SQLi, XSS, path traversal attacks.

**Detection Patterns:**
```go
// SQL Injection
UNION.*SELECT, SELECT.*FROM, INSERT.*INTO, DROP.*TABLE, --, /\*.*\*/

// XSS
<script.*>, javascript:, onerror=, onload=

// Path Traversal
../, ..\\, %2e%2e/, %2e%2e%5c
```

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
  action TEXT  -- 'blocked', 'logged'
);
```

---

### Task #16: Sensitive Data Filtering (PII Masking)

**Problem**: GDPR requires masking personally identifiable information in logs.

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
  redact_paths:
    - pattern: /auth/password/reset/[^/]+
      replacement: /auth/password/reset/[TOKEN]
  mask_pii: true
  mask_ip_method: last_octet  # 203.0.113.45 -> 203.0.113.xxx
```

**Implementation:**
```go
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

### Task #21: Audit Log for Config Changes

**Problem**: Need accountability for who changed what configuration and when.

**Database Schema:**
```sql
CREATE TABLE audit_log (
  audit_id INTEGER PRIMARY KEY,
  timestamp INTEGER NOT NULL,
  user TEXT NOT NULL,  -- 'admin', 'system', IP
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT,
  old_value TEXT,  -- JSON/YAML
  new_value TEXT,
  source TEXT,  -- 'dashboard', 'api', 'file_watcher'
  notes TEXT
);
```

**API Endpoint:**
```
GET /api/audit?limit=100&action=config_reload
```

---

### Task #22: Per-Route Data Retention Policies

**Problem**: GDPR requires automatic data deletion after retention period.

**Configuration:**
```yaml
# Public gaming routes (data minimization)
host: gpanel.chilla55.de
data_retention:
  access_logs: 7d
  error_logs: 30d
  security_logs: 90d
  audit_logs: 365d

# Private Vaultwarden (personal use exemption)
host: vault.chilla55.de
data_retention:
  access_logs: 30d
  error_logs: 90d
  security_logs: 365d
```

**Cleanup Implementation:**
```sql
-- Daily cleanup job at 03:00
DELETE FROM request_logs 
WHERE timestamp < (strftime('%s', 'now') - 
  (SELECT retention_access_logs FROM routes WHERE route_id = request_logs.route_id));
```

---

## 5. Phase 2: Core Monitoring (Weeks 3-4)

**Duration**: 7-10 days  
**Priority**: P1 - HIGH  
**Goal**: Foundation for observability and troubleshooting

### Task #10: Persistent Data Storage with SQLite

**Database Location:** `/data/proxy.db`  
**Go Driver:** `modernc.org/sqlite` (pure Go, no CGO)

See [Complete Database Schema](#10-complete-database-schema) section for full schema.

---

### Task #1: Metrics Collection System

**Metrics to Track:**
- **Per-Request**: timestamp, route, backend, status code, response time, bytes sent/received
- **System**: active connections, goroutines, memory usage, uptime
- **Backend Health**: success rate, avg response time, last successful request
- **Certificates**: expiry dates, days remaining, SANs
- **Security**: rate limit violations, WAF blocks, failed auth, banned IPs

**Storage Strategy:**
- In-memory ring buffer: Last 1000 requests (for `/api/logs`)
- SQLite: Historical data with automatic cleanup

---

### Task #2: Metrics API Endpoints

**GET /api/stats**
```json
{
  "uptime_seconds": 259200,
  "active_connections": 42,
  "requests_last_24h": 128453,
  "error_rate": 0.019
}
```

**GET /api/routes**
```json
{
  "routes": [{
    "domain": "gpanel.chilla55.de",
    "backend": "pterodactyl_panel:8080",
    "status": "healthy",
    "requests_24h": 45231
  }]
}
```

**Security:** Bind to `127.0.0.1:8080` only (SSH tunnel required)

---

### Task #5: Backend Health Checking

**Implementation:**
- Periodic health checks every 10 seconds (configurable)
- HTTP HEAD or GET request to backend
- Track success/failure rate over 1-minute sliding window

**Health States:**
- **Healthy**: 90%+ success rate
- **Degraded**: 50-90% success rate
- **Down**: <50% success rate

**Configuration:**
```yaml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:8080
health_check:
  enabled: true
  interval: 10s
  timeout: 5s
  path: /health
  expected_status: 200
```

---

### Task #6: Request/Error Logging

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
  "client_ip": "203.0.113.xxx",  // Masked per GDPR
  "bytes_sent": 2048
}
```

**Storage:**
- In-memory ring buffer: Last 1000 requests
- SQLite: Per-route retention (7-90 days)
- Separate error log: `/data/logs/errors.log` (rotated daily)

---

### Task #7: Certificate Expiry Monitoring

**Implementation:**
- Parse certificates on startup
- Extract expiry date from X.509
- Check daily for warnings

**Warning Thresholds:**
- 30 days: ⚠️ Warning
- 14 days: ⚠️⚠️ Urgent  
- 7 days: 🚨 Critical

**API Endpoint:**
```
GET /api/certs?warn_days=30
```

---

## 6. Phase 3: Advanced Analytics (Weeks 5-6)

**Duration**: 7-10 days  
**Priority**: P2 - MEDIUM  
**Goal**: Deep insights into traffic patterns and threats

### Task #17: Traffic Analytics

**Metrics:**
- Top routes by traffic volume
- Peak traffic hours heatmap
- Per-route bandwidth tracking
- Request distribution by status code
- Average response time trends
- User agent breakdown (bot vs browser vs API)

**API Endpoints:**
```
GET /api/analytics/top-routes?period=24h
GET /api/analytics/heatmap?period=7d
GET /api/analytics/bandwidth?route=gpanel.chilla55.de
```

---

### Task #18: GeoIP Tracking

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
geoip:
  enabled: true
  alert_on_unusual_country: true  # Alert on unexpected login location
```

---

### Task #19: Webhook Notifications

**Configuration:**
```yaml
webhooks:
  - name: discord-alerts
    url: https://discord.com/api/webhooks/...
    events:
      - service_down
      - cert_expiring_7d
      - high_error_rate    # >5% errors
      - failed_login_spike # >10/min
      - waf_block_spike    # >50/min
    throttle: 300  # Max one alert per 5 min per event
```

**Discord Webhook Format:**
```json
{
  "embeds": [{
    "title": "🚨 Service Down Alert",
    "description": "Pterodactyl panel not responding",
    "color": 15158332,
    "fields": [
      {"name": "Service", "value": "gpanel.chilla55.de"},
      {"name": "Last Seen", "value": "2 minutes ago"}
    ]
  }]
}
```

---

### Task #20: Request Tracing

**Implementation:**
```go
// Generate unique request ID
requestID := uuid.New().String()

// Add to response headers
w.Header().Set("X-Request-ID", requestID)

// Include in all logs
log.Printf("[%s] Request: %s %s", requestID, r.Method, r.URL.Path)
```

**docker-compose.swarm.yml:**
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
      - target: 80
        published: 80
        protocol: tcp
        mode: host
      - target: 443
        published: 443
        protocol: tcp
        mode: host
      - target: 443
        published: 443
        protocol: udp
        mode: host
      - target: 81
        published: 81
        protocol: tcp
        mode: host
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

configs:
  proxy_global_config:
    file: ./global.yaml

networks:
  web-net:
    external: true

volumes:
  proxy-data:
```
**Metrics:**
- Active WebSocket connections per route
- Connection duration tracking
- Connection rate
- Alert on connection leaks

**Configuration:**
```yaml
host: gpanel.chilla55.de
websocket:
  enabled: true
  max_connections: 100
  max_duration: 86400  # 24 hours
  idle_timeout: 300    # 5 minutes
  ping_interval: 30    # Ping every 30s
```

**Database Schema:**
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
  close_reason TEXT
);
```

---

### Task #27: Compression Support

**Configuration:**
```yaml
host: community.example.com
compression:
  enabled: true
  algorithms: [brotli, gzip]
  level: 6              # 1-9 for gzip, 0-11 for brotli
  min_size: 1024        # Don't compress <1KB
  content_types:
    - text/html
    - text/css
    - application/javascript
    - application/json
```

**Implementation:**
```go
// Check Accept-Encoding
if strings.Contains(acceptEncoding, "br") {
  compressor = brotli.NewWriterLevel(w, config.Level)
  w.Header().Set("Content-Encoding", "br")
} else if strings.Contains(acceptEncoding, "gzip") {
  compressor = gzip.NewWriterLevel(w, config.Level)
  w.Header().Set("Content-Encoding", "gzip")
}
```

---

### Task #28: Connection Pooling

**Configuration:**
```yaml
host: gpanel.chilla55.de
connection_pool:
  max_idle_conns: 100          # Total
  max_idle_conns_per_host: 10  # Per backend
  max_conns_per_host: 50       # Max concurrent
  idle_timeout: 90s
```

**Implementation:**
```go
transport := &http.Transport{
  MaxIdleConns:        config.Pool.MaxIdleConns,
  MaxIdleConnsPerHost: config.Pool.MaxIdleConnsPerHost,
  MaxConnsPerHost:     config.Pool.MaxConnsPerHost,
  IdleConnTimeout:     config.Pool.IdleTimeout,
  DisableKeepAlives:   false,  // Enable connection reuse
}
```

---

### Task #29: Slow Request Detection

**Configuration:**
```yaml
host: gpanel.chilla55.de
slow_request:
  enabled: true
  thresholds:
    warning: 5s
    critical: 10s
    timeout: 30s
  alert_webhook: true
```

**Dashboard Display:**
```
Slow Requests (Last 24h):
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Endpoint                    Count   P95      P99
────────────────────────────────────────────────
/api/servers                  42   9.8s    12.3s
/slow-query                   15   15.2s   18.9s
```

---

### Task #30: Request Retry Logic

**Configuration:**
```yaml
host: gpanel.chilla55.de
retry:
  enabled: true
  max_attempts: 3
  backoff: exponential
  initial_delay: 100ms
  max_delay: 2s
  retry_on:
    - connection_refused
    - timeout
    - 502  # Bad Gateway
    - 503  # Service Unavailable
```

**Implementation:**
```go
for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
  resp, err = client.Do(req)
  
  if err == nil && !shouldRetry(resp.StatusCode, config) {
    return resp, nil
  }
  
  if attempt < config.MaxAttempts {
    delay := calculateBackoff(attempt, config)
    time.Sleep(delay)
  }
}
```

---

## 8. Phase 5: Dashboard & UX (Week 8)

**Duration**: 5-7 days  
**Priority**: P2 - MEDIUM  
**Goal**: User-friendly interface for monitoring

### Task #4: Embedded Web Dashboard

**Tech Stack:**
- Pure HTML/CSS/JavaScript (no frameworks)
- Chart.js for visualizations (<50KB)
- Auto-refresh every 5 seconds
- Responsive design

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
├─────────────────────────────────────────────┤
│  Certificate Status                         │
│  ⚠️  api.example.com - 45 days remaining    │
├─────────────────────────────────────────────┤
│  Recent Errors (Last 10)                    │
│  [14:31] 502 - api.example.com - timeout   │
├─────────────────────────────────────────────┤
│  📋 Copy AI Context | 📊 Full Metrics       │
└─────────────────────────────────────────────┘
```

**Access:**
```bash
ssh -L 8080:localhost:8080 srv1
open http://localhost:8080/dashboard
```

---

### Task #9: Custom Error Pages

**Configuration:**
```yaml
host: gpanel.chilla55.de
error_pages:
  404: /etc/proxy/error-pages/404.html
  502: /etc/proxy/error-pages/maintenance.html
  503: /etc/proxy/error-pages/maintenance.html
```

**Template Variables:**
```html
<!DOCTYPE html>
<html>
<head><title>{{.StatusCode}} - {{.StatusText}}</title></head>
<body>
  <h1>{{.StatusCode}} {{.StatusText}}</h1>
  <p>{{.Message}}</p>
  <p>Route: {{.Route}}</p>
  <p>Request ID: {{.RequestID}}</p>
  <p>Timestamp: {{.Timestamp}}</p>
</body>
</html>
```

---

### Task #11: Maintenance Page Storage

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
- When circuit breaker opens: Show maintenance page
- When scheduled time arrives: Activate automatically
- When scheduled end time: Deactivate automatically

---

## 9. Phase 6: Operations & Maintenance (Weeks 9-10)

**Duration**: 7-10 days  
**Priority**: P1 - HIGH  
**Goal**: Production-ready reliability and disaster recovery

### Task #8: Circuit Breaker for Backends

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

**Configuration (site YAML):**
```yaml
enabled: true
service:
  name: gpanel

routes:
  - domains: ["gpanel.chilla55.de"]
    path: /
    backend: http://gpanel:8080

options:
  circuit_breaker:
    enabled: true
    failure_threshold: 5      # Open after N failures
    success_threshold: 2      # Close after N successes
    timeout: 30s              # Stay open for duration
    window: 60s               # Count failures in window
```

**Dashboard Indicators:**
- 🟢 CLOSED - Normal operation
- 🔴 OPEN - Circuit open (failing fast)
- 🟡 HALF-OPEN - Testing recovery

---

### Task #12: Three-Tier SQLite Backup System

**Backup Strategy (DB_PATH=/data/proxy.db, BACKUP_DIR=/mnt/storagebox/backups/proxy):**

**1. Full Backup (Weekly - Sunday 02:00)**
```bash
#!/bin/bash
# backup-full.sh
BACKUP_DIR="/mnt/storagebox/backups/proxy"
DATE=$(date +%Y%m%d_%H%M%S)

sqlite3 /data/proxy.db "VACUUM INTO '$BACKUP_DIR/full-$DATE.db'"
gzip "$BACKUP_DIR/full-$DATE.db"

# Keep last 4 weekly full backups
ls -t "$BACKUP_DIR"/full-*.db.gz | tail -n +5 | xargs rm -f
```

**2. Differential Backup (Mid-Week - Wednesday 02:00)**
```bash
#!/bin/bash
# backup-differential.sh
BACKUP_DIR="/mnt/storagebox/backups/proxy"
LAST_FULL=$(ls -t "$BACKUP_DIR"/full-*.db.gz | head -1)
DATE=$(date +%Y%m%d_%H%M%S)

gunzip -c "$LAST_FULL" > /tmp/last-full.db
rsync -a --link-dest=/tmp/last-full.db /data/proxy.db "$BACKUP_DIR/diff-$DATE.db"
gzip "$BACKUP_DIR/diff-$DATE.db"

# Keep last 8 differential backups
ls -t "$BACKUP_DIR"/diff-*.db.gz | tail -n +9 | xargs rm -f
```

**3. Incremental Backup (Hourly - If Changed)**
```bash
#!/bin/bash
# backup-incremental.sh
BACKUP_DIR="/mnt/storagebox/backups/proxy"
DATE=$(date +%Y%m%d_%H%M%S)
HASH_FILE="/data/.db-hash"

CURRENT_HASH=$(md5sum /data/proxy.db | awk '{print $1}')
LAST_HASH=$(cat "$HASH_FILE" 2>/dev/null)

if [ "$CURRENT_HASH" != "$LAST_HASH" ]; then
  sqlite3 /data/proxy.db "VACUUM INTO '$BACKUP_DIR/incr-$DATE.db'"
  gzip "$BACKUP_DIR/incr-$DATE.db"
  echo "$CURRENT_HASH" > "$HASH_FILE"
  
  # Keep last 168 hourly backups (7 days)
  ls -t "$BACKUP_DIR"/incr-*.db.gz | tail -n +169 | xargs rm -f
fi
```

**Cron Schedule (Container-Internal - Recommended):**

Add a cron daemon to the container and manage schedules internally:

```dockerfile
# In Dockerfile
RUN apk add --no-cache dcron

# Add crontab file
COPY crontab /etc/crontabs/proxy
```

```cron
# /etc/crontabs/proxy
# Full backup - Every Sunday at 02:00
0 2 * * 0 /usr/local/bin/backup-full.sh >> /var/log/backup.log 2>&1

# Differential - Every Wednesday at 02:00
0 2 * * 3 /usr/local/bin/backup-differential.sh >> /var/log/backup.log 2>&1

# Incremental - Every hour
0 * * * * /usr/local/bin/backup-incremental.sh >> /var/log/backup.log 2>&1

# Daily retention cleanup at 03:00
0 3 * * * /usr/local/bin/cleanup-retention.sh >> /var/log/retention-cleanup.log 2>&1
```

```bash
# Update entrypoint.sh to start crond
#!/bin/sh
set -e

# Start cron daemon in background
crond -b -l 2 -L /var/log/cron.log

# Start proxy manager
exec /usr/local/bin/proxy-manager "$@"
```

**Alternative: Host-Based Cron (If preferred):**
```cron
# On Docker host - sudo crontab -e
0 2 * * 0 docker exec $(docker ps -qf name=proxy_proxy) /usr/local/bin/backup-full.sh
0 2 * * 3 docker exec $(docker ps -qf name=proxy_proxy) /usr/local/bin/backup-differential.sh
0 * * * * docker exec $(docker ps -qf name=proxy_proxy) /usr/local/bin/backup-incremental.sh
0 3 * * * docker exec $(docker ps -qf name=proxy_proxy) /usr/local/bin/cleanup-retention.sh
```

**Comparison:**

| Approach | Pros | Cons |
|----------|------|------|
| **Container-Internal** | Portable, survives container restarts, easier to deploy | Requires crond in container |
| **Host-Based** | No extra dependencies, easier to debug | Requires host access, breaks on service rename |

**Recommended**: Container-internal for production deployments.

**Storage Calculation:**
```
Weekly:
- 1 full backup: 5 MB
- 1 differential: 2.5 MB
- 84 hourly incrementals (50% changed): 42 × 80 KB = 3.4 MB
Total per week: ~11 MB

Retention (4 weeks):
- 4 full backups: 20 MB
- 8 differentials: 20 MB
- 168 hourly backups: 13.5 MB
Total storage: ~53.5 MB
```

**Restore Script:**
```bash
#!/bin/bash
# backup-restore.sh
RESTORE_DATE="$1"

# Find full backup before restore date
FULL_BACKUP=$(ls -t "$BACKUP_DIR"/full-*.db.gz | awk -v date="$RESTORE_DATE" '$0 ~ date || $0 < date' | head -1)
gunzip -c "$FULL_BACKUP" > /tmp/restore.db

# Apply differential if exists
DIFF_BACKUP=$(ls -t "$BACKUP_DIR"/diff-*.db.gz | awk -v date="$RESTORE_DATE" '$0 ~ date || $0 < date' | head -1)
if [ -n "$DIFF_BACKUP" ]; then
  sqlite3 /tmp/restore.db ".restore $DIFF_BACKUP"
fi

# Apply incremental backups in order
for INCR in $(ls -t "$BACKUP_DIR"/incr-*.db.gz | awk -v date="$RESTORE_DATE" '$0 ~ date || $0 < date'); do
  sqlite3 /tmp/restore.db ".restore $INCR"
done

# Replace database
docker service scale proxy=0
cp /data/proxy.db /data/proxy.db.backup-$(date +%Y%m%d_%H%M%S)
cp /tmp/restore.db /data/proxy.db
docker service scale proxy=1
```

---

### Task #13: Update Dockerfile and Deployment

**Dockerfile:**
```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o proxy-manager .

FROM alpine:latest

# Install dependencies
RUN apk add --no-cache \
    ca-certificates \
    sqlite \
    rsync \
    gzip \
    tzdata \
    dcron

# Create proxy user
RUN addgroup -g 1000 proxy && \
    adduser -D -u 1000 -G proxy proxy

# Create directories
RUN mkdir -p /data /etc/proxy/sites-available /etc/proxy/error-pages /var/log && \
  chown -R proxy:proxy /data /var/log

COPY --from=builder /build/proxy-manager /usr/local/bin/
COPY --chmod=755 backup-*.sh cleanup-retention.sh /usr/local/bin/
COPY --chmod=755 entrypoint.sh /usr/local/bin/
COPY --chown=proxy:proxy crontab /etc/crontabs/proxy

USER proxy

# Metrics API port (localhost only)
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD []
```

**entrypoint.sh:**
```bash
#!/bin/sh
set -e

echo "Starting proxy-manager with internal cron..."

# Validate configuration before starting
if [ ! -d "$PROXY_SITES_DIR" ]; then
    echo "ERROR: Sites directory not found: $PROXY_SITES_DIR"
    exit 1
fi

if [ ! -d "$PROXY_CERTS_DIR" ]; then
    echo "ERROR: Certs directory not found: $PROXY_CERTS_DIR"
    exit 1
fi

# Start cron daemon in background
crond -b -l 2 -L /var/log/cron.log

# Wait a moment for crond to initialize
sleep 1

echo "Cron daemon started, jobs scheduled:"
crontab -l

echo "Starting proxy manager..."

# Start proxy manager in foreground
# It will handle SIGTERM/SIGINT for graceful shutdown
exec /usr/local/bin/proxy-manager "$@"
```

**crontab:**
```cron
# Proxy Manager Backup Schedule
# Logs go to /var/log/backup.log and /var/log/retention-cleanup.log

# Full backup - Every Sunday at 02:00 UTC
0 2 * * 0 /usr/local/bin/backup-full.sh >> /var/log/backup.log 2>&1

# Differential backup - Every Wednesday at 02:00 UTC
0 2 * * 3 /usr/local/bin/backup-differential.sh >> /var/log/backup.log 2>&1

# Incremental backup - Every hour
0 * * * * /usr/local/bin/backup-incremental.sh >> /var/log/backup.log 2>&1

# Daily retention cleanup at 03:00 UTC
0 3 * * * /usr/local/bin/cleanup-retention.sh >> /var/log/retention-cleanup.log 2>&1
```

**docker-compose.swarm.yml:**
```yaml
version: '3.8'

services:
  proxy:
    image: ghcr.io/chilla55/go-proxy:latest
    networks:
      - web-net
    ports:
      - target: 80
        published: 80
        mode: host
      - target: 443
        published: 443
        mode: host
      # Metrics port published on 8080 for health/debug
    volumes:
      - type: bind
        source: /mnt/storagebox/certs
        target: /etc/proxy/certs
        read_only: true
        bind:
          propagation: rslave
      - type: bind
        source: /mnt/storagebox/sites
        target: /etc/proxy/sites-available
        read_only: true
        bind:
          propagation: rslave
      - proxy-data:/data
    environment:
      - TZ=UTC
      - SITES_PATH=/etc/proxy/sites-available
      - GLOBAL_CONFIG=/etc/proxy/global.yaml
      - DB_PATH=/data/proxy.db
      - BACKUP_DIR=/mnt/storagebox/backups/proxy
    deploy:
      mode: replicated
      replicas: 1
      placement:
        constraints:
          - node.labels.web.node == web
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 3
      update_config:
        parallelism: 1
        delay: 10s
        failure_action: rollback
        order: start-first
    stop_grace_period: 35s  # Slightly longer than shutdown timeout
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8080/api/stats"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s

volumes:
  proxy-data:
    driver: local

networks:
  web:
    external: true
```

**Accessing Dashboard:**
```bash
# SSH tunnel
ssh -L 8080:localhost:8080 srv1

# Open browser
open http://localhost:8080/dashboard

# Or curl API
curl http://localhost:8080/api/stats
curl http://localhost:8080/api/ai-context
```

---

## 10. Complete Database Schema

```sql
-- ============================================
-- Service Registry
-- ============================================
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

-- ============================================
-- Route Configurations
-- ============================================
CREATE TABLE routes (
  route_id INTEGER PRIMARY KEY,
  domain TEXT NOT NULL UNIQUE,
  service_id INTEGER NOT NULL,
  config_yaml TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  retention_access_logs INTEGER DEFAULT 604800,    -- 7 days
  retention_error_logs INTEGER DEFAULT 2592000,    -- 30 days
  retention_security_logs INTEGER DEFAULT 7776000, -- 90 days
  retention_metrics INTEGER DEFAULT 2592000,       -- 30 days
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

-- ============================================
-- Time-Series Metrics
-- ============================================
CREATE TABLE metrics (
  metric_id INTEGER PRIMARY KEY,
  timestamp INTEGER NOT NULL,
  service_id INTEGER NOT NULL,
  status_code INTEGER NOT NULL,
  response_time_ms INTEGER NOT NULL,
  bytes_sent INTEGER NOT NULL,
  bytes_received INTEGER NOT NULL,
  bytes_saved_compression INTEGER DEFAULT 0,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

CREATE INDEX idx_metrics_timestamp ON metrics(timestamp);
CREATE INDEX idx_metrics_service ON metrics(service_id);

-- ============================================
-- Request Logs
-- ============================================
CREATE TABLE request_logs (
  log_id INTEGER PRIMARY KEY,
  timestamp INTEGER NOT NULL,
  service_id INTEGER NOT NULL,
  method TEXT NOT NULL,
  path TEXT NOT NULL,
  status_code INTEGER NOT NULL,
  response_time_ms INTEGER NOT NULL,
  client_ip TEXT,  -- GDPR masked
  user_agent TEXT,
  bytes_sent INTEGER,
  bytes_received INTEGER,
  error_message TEXT,
  request_id TEXT,  -- For tracing
  retry_count INTEGER DEFAULT 0,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

CREATE INDEX idx_logs_timestamp ON request_logs(timestamp);
CREATE INDEX idx_logs_status ON request_logs(status_code);
CREATE INDEX idx_logs_request_id ON request_logs(request_id);

-- ============================================
-- Health Checks
-- ============================================
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

-- ============================================
-- Certificates
-- ============================================
CREATE TABLE certificates (
  cert_id INTEGER PRIMARY KEY,
  domain TEXT NOT NULL UNIQUE,
  cert_path TEXT NOT NULL,
  key_path TEXT NOT NULL,
  expires_at INTEGER NOT NULL,
  san_domains TEXT,  -- JSON array
  last_checked INTEGER NOT NULL
);

-- ============================================
-- Security: Rate Limiting
-- ============================================
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

-- ============================================
-- Security: WAF
-- ============================================
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

-- ============================================
-- Analytics: Traffic
-- ============================================
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

-- ============================================
-- WebSocket Connections
-- ============================================
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

-- ============================================
-- Error Pages
-- ============================================
CREATE TABLE error_pages (
  page_id INTEGER PRIMARY KEY,
  route_id INTEGER,  -- NULL = global
  status_code INTEGER NOT NULL,
  html_content TEXT NOT NULL,
  last_modified INTEGER NOT NULL,
  FOREIGN KEY(route_id) REFERENCES routes(route_id),
  UNIQUE(route_id, status_code)
);

-- ============================================
-- Maintenance Pages
-- ============================================
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

-- ============================================
-- Circuit Breaker State
-- ============================================
CREATE TABLE circuit_breaker_state (
  service_id INTEGER PRIMARY KEY,
  state TEXT NOT NULL,  -- 'closed', 'open', 'half-open'
  failure_count INTEGER DEFAULT 0,
  last_failure INTEGER,
  opened_at INTEGER,
  FOREIGN KEY(service_id) REFERENCES services(service_id)
);

-- ============================================
-- Audit Log
-- ============================================
CREATE TABLE audit_log (
  audit_id INTEGER PRIMARY KEY,
  timestamp INTEGER NOT NULL,
  user TEXT NOT NULL,  -- 'admin', 'system', IP
  action TEXT NOT NULL,
  resource_type TEXT NOT NULL,
  resource_id TEXT,
  old_value TEXT,
  new_value TEXT,
  source TEXT,  -- 'dashboard', 'api', 'file_watcher'
  notes TEXT
);

CREATE INDEX idx_audit_timestamp ON audit_log(timestamp DESC);
CREATE INDEX idx_audit_action ON audit_log(action);
```

---

## 11. Deployment Guide

### 11.1 Prerequisites

- Docker Swarm initialized
- `web` network created: `docker network create --driver overlay --attachable web`
- Node label set: `docker node update --label-add web=true <node-name>`
- Storagebox mounted at `/mnt/storagebox`

### 11.2 Build and Push

```bash
cd /path/to/go-proxy/proxy-manager

# Build image
docker build -t ghcr.io/chilla55/proxy-manager:latest .

# Push to GitHub Container Registry
echo $GITHUB_TOKEN | docker login ghcr.io -u chilla55 --password-stdin
docker push ghcr.io/chilla55/proxy-manager:latest
```

### 11.3 Deploy to Swarm

```bash
# Deploy service
docker stack deploy -c docker-compose.swarm.yml proxy

# Check status
docker service ls
docker service logs -f proxy_proxy

# Check health
curl http://localhost:8080/api/stats
```

### 11.4 Verify Backup Schedule

```bash
# Check if cron is running in container
docker exec $(docker ps -qf name=proxy_proxy) ps aux | grep crond

# View cron schedule
docker exec $(docker ps -qf name=proxy_proxy) crontab -l

# Check backup logs
docker exec $(docker ps -qf name=proxy_proxy) tail -f /var/log/backup.log

# Manually trigger a backup (for testing)
docker exec $(docker ps -qf name=proxy_proxy) /usr/local/bin/backup-incremental.sh

# Check if backups are being created
docker exec $(docker ps -qf name=proxy_proxy) ls -lh /mnt/storagebox/backups/proxy/
```

**Backup Schedule (Container-Internal):**
- ✅ Full backup: Every Sunday 02:00 UTC
- ✅ Differential: Every Wednesday 02:00 UTC  
- ✅ Incremental: Every hour (if DB changed)
- ✅ Retention cleanup: Daily at 03:00 UTC

All schedules managed by crond inside the container.

### 11.5 Access Dashboard

```bash
# Create SSH tunnel
ssh -L 8080:localhost:8080 srv1

# Open browser
open http://localhost:8080/dashboard

# Export AI context
curl http://localhost:8080/api/ai-context > status-report.md
```

---

## 12. Testing Strategy

### 12.1 Unit Tests

```go
// Test timeout configuration
func TestTimeoutConfig(t *testing.T) {
  config := TimeoutConfig{Connect: 5*time.Second}
  assert.Equal(t, 5*time.Second, config.Connect)
}

// Test rate limiting
func TestRateLimiter(t *testing.T) {
  limiter := NewRateLimiter(10, time.Minute)
  for i := 0; i < 10; i++ {
    assert.True(t, limiter.Allow("192.168.1.1"))
  }
  assert.False(t, limiter.Allow("192.168.1.1"))  // 11th request blocked
}

// Test WAF patterns
func TestWAFDetection(t *testing.T) {
  waf := NewWAF()
  assert.True(t, waf.DetectSQLi("SELECT * FROM users"))
  assert.True(t, waf.DetectXSS("<script>alert(1)</script>"))
}
```

### 12.2 Integration Tests

```bash
# Test full request flow
curl -v http://localhost/test-route
# Verify metrics stored in SQLite
sqlite3 /data/proxy.db "SELECT COUNT(*) FROM request_logs;"

# Test health checks
sqlite3 /data/proxy.db "SELECT * FROM health_checks WHERE success = 0;"

# Test certificate monitoring
curl http://localhost:8080/api/certs
```

### 12.3 Load Tests

```bash
# Apache Bench
ab -n 10000 -c 100 http://gpanel.chilla55.de/

# Vegeta
echo "GET http://gpanel.chilla55.de/" | vegeta attack -duration=60s -rate=100 | vegeta report

# Monitor during load
watch -n 1 'curl -s http://localhost:8080/api/stats | jq .'
```

### 12.4 Manual Tests

- [ ] Dashboard accessible via SSH tunnel
- [ ] Metrics update every 5 seconds
- [ ] AI context export generates useful reports
- [ ] Circuit breaker opens on backend failure
- [ ] Certificate warnings appear 30 days before expiry
- [ ] Backup and restore procedures work
- [ ] WAF blocks SQL injection attempts
- [ ] Rate limiting triggers after threshold
- [ ] GeoIP tracking shows correct countries
- [ ] WebSocket connections tracked properly

---

## Success Criteria

- ✅ Dashboard accessible via SSH tunnel
- ✅ Real-time metrics updating every 5 seconds
- ✅ AI context export generates useful troubleshooting reports
- ✅ Circuit breaker prevents cascading failures
- ✅ Certificate expiry warnings 30 days in advance
- ✅ Backup/restore tested and documented
- ✅ No performance degradation (<5ms overhead per request)
- ✅ SQLite database stays under 100 MB with retention policies
- ✅ GDPR compliance verified (PII masking, retention, audit logs)
- ✅ Security features tested (rate limiting, WAF, headers)
- ✅ All tests passing (unit, integration, load)
- ✅ Documentation complete and reviewed

---

## Appendix A: Optional Features (Future Enhancements)

### A.1 Prometheus Metrics Export

For integration with Prometheus/Grafana stacks:

```go
import "github.com/prometheus/client_golang/prometheus"

var (
    requestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "proxy_requests_total",
            Help: "Total number of requests",
        },
        []string{"route", "status_code", "method"},
    )
    
    requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "proxy_request_duration_seconds",
            Help: "Request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"route"},
    )
    
    activeConnections = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "proxy_active_connections",
            Help: "Number of active connections",
        },
    )
)

func init() {
    prometheus.MustRegister(requestsTotal)
    prometheus.MustRegister(requestDuration)
    prometheus.MustRegister(activeConnections)
}

// Expose at /metrics endpoint
http.Handle("/metrics", promhttp.Handler())
```

**Configuration:**
```yaml
# docker-compose.swarm.yml
environment:
  - PROMETHEUS_ENABLED=true
  - PROMETHEUS_PORT=9090
```

---

### A.2 Log Shipping to External Systems

For centralized logging (Loki, Elasticsearch, etc.):

```go
// Support JSON logs for easy parsing
type LogEntry struct {
    Timestamp   time.Time `json:"timestamp"`
    Level       string    `json:"level"`
    Service     string    `json:"service"`
    Route       string    `json:"route"`
    Method      string    `json:"method"`
    Path        string    `json:"path"`
    StatusCode  int       `json:"status_code"`
    Duration    int64     `json:"duration_ms"`
    ClientIP    string    `json:"client_ip"`
    UserAgent   string    `json:"user_agent"`
    RequestID   string    `json:"request_id"`
    Error       string    `json:"error,omitempty"`
}

// Log to stdout in JSON format for container log drivers
log.Info().
    Str("service", "proxy-manager").
    Str("route", route).
    Int("status_code", statusCode).
    Int64("duration_ms", duration).
    Msg("request completed")
```

**Docker logging driver:**
```yaml
services:
  proxy:
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
        labels: "service=proxy-manager"
```

---

### A.3 Dynamic Configuration Reload

Already partially implemented via file watching, but add API support:

```go
// POST /api/admin/reload
func handleReloadConfig(w http.ResponseWriter, r *http.Request) {
    log.Info().Msg("Manual config reload requested")
    
    // Reload sites
    if err := reloadSites(); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // Reload certificates
    if err := reloadCertificates(); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // Log to audit
    logAuditEvent("admin", "config_reload", "system", "", "", "", "api")
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{
        "status": "ok",
        "message": "Configuration reloaded successfully",
    })
}
```

---

### A.4 Advanced Health Check Endpoints

Beyond the basic health check:

```go
// GET /health/live - Kubernetes liveness probe
func handleLiveness(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

// GET /health/ready - Kubernetes readiness probe
func handleReadiness(w http.ResponseWriter, r *http.Request) {
    // Check database
    if err := db.Ping(); err != nil {
        http.Error(w, "Database not ready", http.StatusServiceUnavailable)
        return
    }
    
    // Check at least one backend is healthy
    healthyBackends := getHealthyBackendCount()
    if healthyBackends == 0 {
        http.Error(w, "No healthy backends", http.StatusServiceUnavailable)
        return
    }
    
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status": "ready",
        "healthy_backends": healthyBackends,
    })
}

// GET /health/startup - Kubernetes startup probe
func handleStartup(w http.ResponseWriter, r *http.Request) {
    if !isFullyInitialized() {
        http.Error(w, "Still initializing", http.StatusServiceUnavailable)
        return
    }
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}
```

---

## Appendix B: Configuration Reference

### Full Example: Pterodactyl Panel

```yaml
# /etc/proxy/sites-available/gpanel.chilla55.de.yaml
host: gpanel.chilla55.de
backend: http://pterodactyl_panel:8080

# Phase 0: Essential Reliability
timeouts:
  connect: 5s
  read: 30s
  write: 30s
  idle: 120s

limits:
  max_request_body: 104857600  # 100 MB
  max_request_headers: 16384
  max_response_body: 52428800  # 50 MB

headers:
  preset: security-strict
  response:
    add:
      Strict-Transport-Security: "max-age=31536000; includeSubDomains"

# Phase 1: Security & Compliance
rate_limit:
  global: 1000
  per_ip: 100
  paths:
    - path: /auth/*
      per_ip: 5
      ban_after: 3
      ban_duration: 3600

waf:
  enabled: true
  mode: block
  rules:
    sql_injection: true
    xss: true
    path_traversal: true

logging:
  strip_headers: [Authorization, Cookie]
  mask_pii: true
  mask_ip_method: last_octet

data_retention:
  access_logs: 7d
  error_logs: 30d
  security_logs: 90d

# Phase 2: Core Monitoring
health_check:
  enabled: true
  interval: 10s
  timeout: 5s
  path: /health

# Phase 4: Performance
websocket:
  enabled: true
  max_connections: 100
  max_duration: 86400

compression:
  enabled: true
  algorithms: [gzip]
  level: 6
  min_size: 1024

connection_pool:
  max_idle_conns_per_host: 10
  max_conns_per_host: 50

# Phase 6: Reliability
circuit_breaker:
  enabled: true
  failure_threshold: 5
  timeout: 30s

error_pages:
  502: /etc/proxy/error-pages/maintenance.html
  503: /etc/proxy/error-pages/maintenance.html
```

---

**End of Implementation Plan v2.0**

*Generated December 18, 2025 for gaming community deployment on Hetzner dedicated server (Germany). All features designed for 20+ international users with GDPR compliance requirements.*
