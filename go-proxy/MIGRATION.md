# Migration Guide

Guide for migrating from nginx or other reverse proxies to the Go reverse proxy.

## Table of Contents

- [Migration Strategy](#migration-strategy)
- [From Nginx](#from-nginx)
- [From Traefik](#from-traefik)
- [From HAProxy](#from-haproxy)
- [Zero-Downtime Migration](#zero-downtime-migration)
- [Rollback Plan](#rollback-plan)

---

## Migration Strategy

### Phase 1: Preparation (1-2 days)

1. **Inventory** - Document all current routes, certificates, and configurations
2. **Test Environment** - Set up go-proxy in staging
3. **Convert Configs** - Translate existing configs to YAML format
4. **Validate** - Test all routes in staging
5. **Metrics Baseline** - Record current performance metrics

### Phase 2: Parallel Deployment (1 week)

1. **Deploy Side-by-Side** - Run go-proxy on alternate ports
2. **Gradual Traffic** - Route small percentage to go-proxy
3. **Monitor** - Compare metrics between old and new
4. **Iterate** - Fix issues found during parallel run

### Phase 3: Cutover (1 day)

1. **Final Sync** - Update all configs
2. **DNS/Load Balancer** - Switch traffic to go-proxy
3. **Monitor** - Watch metrics for 24 hours
4. **Decommission** - Remove old proxy after stability confirmed

---

## From Nginx

### Configuration Conversion

#### Nginx Simple Location

**Before (nginx.conf):**
```nginx
server {
    listen 80;
    listen 443 ssl http2;
    server_name app.example.com;

    ssl_certificate /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;

    location / {
        proxy_pass http://backend:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

**After (app.yaml):**
```yaml
enabled: true

service:
  name: app

routes:
  - domains:
      - app.example.com
    path: /
    backend: http://backend:8080

options:
  timeout: 30s
```

#### Nginx with Health Checks

**Before (nginx.conf):**
```nginx
upstream backend {
    server backend1:8080 max_fails=3 fail_timeout=30s;
    server backend2:8080 max_fails=3 fail_timeout=30s;
}

server {
    server_name app.example.com;
    
    location / {
        proxy_pass http://backend;
        proxy_next_upstream error timeout http_502;
    }
    
    location /health {
        access_log off;
        proxy_pass http://backend/health;
    }
}
```

**After (app.yaml):**
```yaml
enabled: true

service:
  name: app

routes:
  - domains:
      - app.example.com
    path: /
    backend: http://backend1:8080

options:
  health_check_path: /health
  health_check_interval: 30s
  
  circuit_breaker:
    enabled: true
    failure_threshold: 3
    timeout: 30s
  
  retry:
    enabled: true
    max_attempts: 3
    retry_on: ["502", "timeout"]
```

**Note:** Load balancing multiple upstreams requires separate route configs or service registry registration.

#### Nginx WebSocket

**Before (nginx.conf):**
```nginx
server {
    server_name ws.example.com;
    
    location / {
        proxy_pass http://websocket:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;
    }
}
```

**After (ws.yaml):**
```yaml
enabled: true

service:
  name: websocket

routes:
  - domains:
      - ws.example.com
    path: /
    backend: http://websocket:3000
    websocket: true

options:
  websocket:
    enabled: true
    max_duration: 24h
    idle_timeout: 5m
  
  timeout: 300s
```

#### Nginx Rate Limiting

**Before (nginx.conf):**
```nginx
limit_req_zone $binary_remote_addr zone=api_limit:10m rate=10r/s;

server {
    server_name api.example.com;
    
    location / {
        limit_req zone=api_limit burst=20 nodelay;
        proxy_pass http://api:8080;
    }
}
```

**After (api.yaml):**
```yaml
enabled: true

service:
  name: api

routes:
  - domains:
      - api.example.com
    path: /
    backend: http://api:8080

options:
  rate_limit:
    enabled: true
    requests_per_min: 600    # 10 req/s = 600/min
    burst_size: 20
    per_ip: true
```

#### Nginx Custom Headers

**Before (nginx.conf):**
```nginx
server {
    server_name app.example.com;
    
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Strict-Transport-Security "max-age=31536000" always;
    
    location / {
        proxy_pass http://backend:8080;
    }
}
```

**After (global.yaml + app.yaml):**

**global.yaml:**
```yaml
defaults:
  headers:
    X-Frame-Options: DENY
    X-Content-Type-Options: nosniff
    Strict-Transport-Security: max-age=31536000; includeSubDomains
```

**app.yaml:**
```yaml
enabled: true

service:
  name: app

routes:
  - domains:
      - app.example.com
    path: /
    backend: http://backend:8080

# Headers from global.yaml are automatically applied
```

### Certificate Migration

#### Copy Let's Encrypt Certificates

```bash
# Nginx typically stores certs in /etc/letsencrypt
cp -r /etc/letsencrypt/live/example.com /mnt/storagebox/certs/

# Update permissions
chown -R root:root /mnt/storagebox/certs/example.com
chmod 644 /mnt/storagebox/certs/example.com/fullchain.pem
chmod 600 /mnt/storagebox/certs/example.com/privkey.pem
```

#### Configure in global.yaml

```yaml
tls:
  certificates:
    - domains:
        - "*.example.com"
        - "example.com"
      cert_file: /etc/proxy/certs/example.com/fullchain.pem
      key_file: /etc/proxy/certs/example.com/privkey.pem
```

### Feature Mapping

| Nginx Feature | Go Proxy Equivalent |
|---------------|---------------------|
| `proxy_pass` | `backend: http://...` |
| `limit_req` | `rate_limit` options |
| `upstream` block | Service registry or multiple configs |
| `proxy_cache` | Not implemented (use external cache) |
| `gzip on` | `compression: true` |
| `ssl_protocols` | Automatic (TLS 1.2+) |
| `proxy_read_timeout` | `timeouts.read` |
| `proxy_connect_timeout` | `timeouts.connect` |
| `client_max_body_size` | `limits.max_request_body` |
| `add_header` | `headers` section |

### Nginx Modules Not Needed

- **nginx-auth** → Use WAF or upstream authentication
- **nginx-cache** → Use Redis/Memcached upstream
- **nginx-geoip** → Built-in `geoip` options
- **nginx-realip** → Automatic X-Forwarded-For handling
- **nginx-limit-conn** → Built-in rate limiting
- **nginx-ssl** → Built-in TLS termination

---

## From Traefik

### Configuration Conversion

#### Traefik Docker Labels

**Before (docker-compose.yml):**
```yaml
services:
  app:
    image: myapp:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.app.rule=Host(`app.example.com`)"
      - "traefik.http.routers.app.entrypoints=websecure"
      - "traefik.http.routers.app.tls.certresolver=letsencrypt"
      - "traefik.http.services.app.loadbalancer.server.port=8080"
```

**After (app.yaml):**
```yaml
enabled: true

service:
  name: app

routes:
  - domains:
      - app.example.com
    path: /
    backend: http://app:8080
```

#### Traefik File Provider

**Before (traefik-config.yml):**
```yaml
http:
  routers:
    api-router:
      rule: "Host(`api.example.com`) && PathPrefix(`/v1`)"
      service: api-service
      middlewares:
        - ratelimit
  
  services:
    api-service:
      loadBalancer:
        servers:
          - url: "http://api:8080"
  
  middlewares:
    ratelimit:
      rateLimit:
        average: 100
        burst: 50
```

**After (api.yaml):**
```yaml
enabled: true

service:
  name: api

routes:
  - domains:
      - api.example.com
    path: /v1
    backend: http://api:8080

options:
  rate_limit:
    enabled: true
    requests_per_min: 100
    burst_size: 50
```

### Middleware Mapping

| Traefik Middleware | Go Proxy Equivalent |
|-------------------|---------------------|
| `rateLimit` | `rate_limit` options |
| `compress` | `compression` options |
| `stripPrefix` | Not needed (backend handles paths) |
| `addPrefix` | Not needed (backend handles paths) |
| `basicAuth` | Use WAF or upstream auth |
| `redirectScheme` | Automatic HTTPS redirect |
| `headers` | `headers` section |
| `retry` | `retry` options |
| `circuitBreaker` | `circuit_breaker` options |

---

## From HAProxy

### Configuration Conversion

#### HAProxy Basic Config

**Before (haproxy.cfg):**
```
frontend http_front
    bind *:80
    bind *:443 ssl crt /etc/haproxy/certs/example.pem
    
    acl is_app hdr(host) -i app.example.com
    use_backend app_backend if is_app

backend app_backend
    balance roundrobin
    server app1 backend1:8080 check inter 2s rise 2 fall 3
    server app2 backend2:8080 check inter 2s rise 2 fall 3
```

**After (app.yaml):**
```yaml
enabled: true

service:
  name: app

routes:
  - domains:
      - app.example.com
    path: /
    backend: http://backend1:8080

options:
  health_check_path: /
  health_check_interval: 2s
  
  circuit_breaker:
    enabled: true
    failure_threshold: 3
```

**Note:** For load balancing multiple servers, register them separately via service registry or create multiple route configs.

#### HAProxy with Timeouts

**Before (haproxy.cfg):**
```
defaults
    timeout connect 5s
    timeout client 30s
    timeout server 30s
    
backend app_backend
    server app1 backend:8080 check
```

**After (app.yaml):**
```yaml
enabled: true

service:
  name: app

routes:
  - domains:
      - app.example.com
    path: /
    backend: http://backend:8080

options:
  timeouts:
    connect: 5s
    read: 30s
    write: 30s
    idle: 120s
```

---

## Zero-Downtime Migration

### Blue-Green Deployment

Run both proxies simultaneously, then switch:

#### Step 1: Deploy Go-Proxy on Alternate Ports

```yaml
# docker-compose.swarm.yml
services:
  proxy-new:
    image: ghcr.io/chilla55/go-proxy:latest
    ports:
      - "8880:80"
      - "8443:443"
    # ... rest of config
```

#### Step 2: Test New Proxy

```bash
# Test via alternate port
curl -H "Host: app.example.com" http://localhost:8880/

# Verify all routes
for domain in app.example.com api.example.com; do
  curl -I -H "Host: $domain" http://localhost:8880/
done
```

#### Step 3: Switch Load Balancer

```bash
# Update load balancer to point to new proxy
# OR update DNS records
# OR swap port bindings in Docker Swarm
```

#### Step 4: Monitor

```bash
# Watch metrics on both proxies
curl http://old-proxy:8080/metrics
curl http://new-proxy:8080/metrics

# Compare error rates, latency
```

#### Step 5: Decommission Old Proxy

After 24-48 hours of stability:

```bash
docker service rm proxy-old
```

### Gradual Rollout (Canary)

Route percentage of traffic to new proxy:

#### Using nginx as Front Load Balancer

```nginx
upstream proxy_pool {
    server old-proxy:80 weight=90;
    server new-proxy:80 weight=10;
}

server {
    listen 80;
    location / {
        proxy_pass http://proxy_pool;
    }
}
```

Gradually increase new proxy weight: 10% → 25% → 50% → 75% → 100%

#### Using DNS Weighted Routing

```bash
# Route 10% to new proxy IP
# Route 90% to old proxy IP
# Gradually shift weights
```

---

## Rollback Plan

### Quick Rollback (< 5 minutes)

If issues detected immediately after cutover:

```bash
# Swap back port bindings
docker service update \
  --publish-rm 80:80 \
  --publish-rm 443:443 \
  proxy-new

docker service update \
  --publish-add 80:80 \
  --publish-add 443:443 \
  proxy-old
```

### Service-Specific Rollback

Revert individual service to old proxy:

```bash
# Update DNS or load balancer for specific domain
# Point app.example.com back to old proxy
```

### Full Rollback (Planned)

If migration needs to be postponed:

1. Stop go-proxy service
2. Restore old proxy to production ports
3. Document issues encountered
4. Plan remediation
5. Schedule new migration date

---

## Validation Checklist

Before cutover, verify:

- [ ] All routes return correct status codes
- [ ] SSL certificates valid and served correctly
- [ ] WebSocket connections work (if applicable)
- [ ] Health checks passing for all backends
- [ ] Rate limiting enforced as expected
- [ ] Custom headers present in responses
- [ ] Compression working (check Content-Encoding)
- [ ] Metrics endpoint accessible
- [ ] Logs being written correctly
- [ ] Alerts configured and tested
- [ ] Backup strategy in place
- [ ] Rollback plan documented and rehearsed

---

## Common Issues and Solutions

### Issue: Missing Headers in Responses

**Problem:** Custom headers not appearing  
**Solution:** Check `global.yaml` defaults and site-specific `headers` section

### Issue: WebSocket Connections Drop

**Problem:** WebSocket timeout too short  
**Solution:** Increase `websocket.max_duration` and `websocket.idle_timeout`

### Issue: High Memory Usage

**Problem:** Too many cached connections  
**Solution:** Tune `connection_pool.max_idle_conns_per_host`

### Issue: Certificate Not Found

**Problem:** Wrong cert path or permissions  
**Solution:** Verify paths in `global.yaml` and check file permissions

```bash
ls -la /mnt/storagebox/certs/example.com/
# Should be readable by proxy container user
```

### Issue: Rate Limiting Too Aggressive

**Problem:** Legitimate users being blocked  
**Solution:** Increase limits or add IP whitelist

```yaml
rate_limit:
  requests_per_min: 120  # Increase
  whitelist:             # Add trusted IPs
    - 203.0.113.0/24
```

### Issue: Backend Health Check Failures

**Problem:** Health check path wrong or backend slow  
**Solution:** Verify path and increase timeout

```yaml
options:
  health_check_path: /api/health  # Fix path
  health_check_timeout: 10s       # Increase timeout
```

---

## Performance Comparison

### Expected Improvements

- **Startup Time:** Nginx ~1s → Go-Proxy ~200ms
- **Memory Usage:** Nginx ~50MB → Go-Proxy ~30MB (base)
- **Config Reload:** Nginx reload ~500ms → Go-Proxy instant (live reload)
- **Binary Size:** Nginx ~100MB (with modules) → Go-Proxy ~15MB

### Monitoring During Migration

Track these metrics:

```bash
# Request rate
proxy_requests_total

# Latency (p50, p95, p99)
proxy_request_duration_seconds

# Error rate
proxy_backend_errors_total / proxy_requests_total

# Active connections
proxy_active_connections

# Circuit breaker trips
proxy_circuit_breaker_state
```

### Acceptance Criteria

Migration successful if:

- Error rate < 0.1%
- p95 latency < old proxy + 10ms
- No increase in backend failures
- All features functional
- Metrics stable for 48 hours

---

## Post-Migration

### Week 1

- Monitor metrics closely
- Keep old proxy available (stopped, not deleted)
- Document any config tweaks needed
- Update runbooks and documentation

### Week 2-4

- Fine-tune performance settings
- Implement any missing features
- Train team on new monitoring/debugging
- Decommission old proxy infrastructure

### Ongoing

- Schedule regular config reviews
- Keep documentation updated
- Monitor for cert expiration
- Review and optimize retention policies

---

## Support Resources

- [README.md](README.md) - Feature overview
- [CONFIGURATION.md](CONFIGURATION.md) - Complete config reference
- [DEPLOYMENT.md](DEPLOYMENT.md) - Docker Swarm deployment
- [SERVICE_REGISTRY.md](SERVICE_REGISTRY.md) - Dynamic service registration

For migration assistance, review logs and metrics:

```bash
# Check proxy logs
docker service logs -f proxy_proxy

# Check health
curl http://localhost:8080/health

# Check metrics
curl http://localhost:8080/metrics
```
