# Quick Start Guide

## Prerequisites

Before starting, you need SSL certificates. For testing, you can:

1. **Use self-signed certificates** (for local testing):
   ```bash
   # Create test certificates
   mkdir -p certs/test.local
   openssl req -x509 -newkey rsa:4096 -nodes \
     -keyout certs/test.local/privkey.pem \
     -out certs/test.local/fullchain.pem \
     -days 365 -subj "/CN=*.test.local"
   ```

2. **Use your existing wildcard certificates** (for production)

See [CERTIFICATE_SETUP.md](CERTIFICATE_SETUP.md) for production setup.

## Test the New Proxy Locally

### 1. Configure Certificates

Edit `global.yaml`:
```yaml
tls:
  certificates:
    - domains:
        - "*.test.local"
        - "test.local"
      cert_file: /etc/proxy/certs/test.local/fullchain.pem
      key_file: /etc/proxy/certs/test.local/privkey.pem
```

### 2. Build the Project

```bash
cd /media/chilla55/New\ Volume/__________Docker/docker-images/nginx

# Build Docker image
make docker-build
```

### 3. Start Demo Environment

```bash
# Start proxy + demo backend
docker-compose up -d

# Check logs
docker-compose logs -f proxy
```

You should see:
```
[proxy-manager] Loaded certificate for domains: [*.test.local test.local]
[proxy-manager] Loaded 1 TLS certificate(s)
```

### 4. Test It Works

```bash
# Check health
curl http://localhost:8080/health
# Expected: "healthy"

# Check metrics
curl http://localhost:8080/metrics
# Expected: Prometheus metrics

# Test proxy (add demo.test.local to /etc/hosts first)
echo "127.0.0.1 demo.test.local" | sudo tee -a /etc/hosts

# HTTP request (redirects to HTTPS)
curl -H "Host: demo.test.local" http://localhost/
# Expected: 301 redirect to https://

# HTTPS request (ignore self-signed cert for testing)
curl -k https://demo.test.local/
# Expected: whoami response

# Test unknown domain (should be blackholed)
curl -v -H "Host: unknown.test" http://localhost/
# Expected: Connection closed immediately
```

### 5. Add Your Own Site

```bash
# Create new site config
cat > sites-available/myapp.yaml <<EOF
enabled: true

service:
  name: myapp

routes:
  - domains:
      - myapp.test.local
    path: /
    backend: http://demo-app:80

options:
  timeout: 30s
EOF

# Reload proxy (automatic - just wait a moment)
# Or restart container
docker-compose restart proxy

# Test
curl -H "Host: myapp.localhost" http://localhost/
```

### 5. Test Dynamic Registration

```bash
# Connect to registry and register a service
exec 3<>/dev/tcp/localhost/81

# Register
echo "REGISTER|testapp|testapp|8080|8081" >&3
read -u 3 response
echo "Registration response: $response"

# Extract session ID
SESSION_ID=$(echo "$response" | cut -d'|' -f2)
echo "Session ID: $SESSION_ID"

# Add route
echo "ROUTE|$SESSION_ID|test.localhost|/|http://demo-app:80" >&3
read -u 3 response
echo "Route response: $response"

# Add header
echo "HEADER|$SESSION_ID|X-Custom-Header|HelloWorld" >&3
read -u 3 response
echo "Header response: $response"

# Test it
curl -H "Host: test.localhost" http://localhost/
```

### 6. Monitor

```bash
# Watch logs
docker-compose logs -f proxy

# Check metrics
watch -n 1 'curl -s http://localhost:8080/metrics'

# Check blackhole count
curl -s http://localhost:8080/metrics | grep blackhole
```

### 7. Load Test

```bash
# Install Apache Bench if needed
sudo apt-get install apache2-utils

# Run load test (500 concurrent users)
ab -n 10000 -c 500 -H "Host: demo.localhost" http://localhost/

# Check if everything still works
curl -H "Host: demo.localhost" http://localhost/
```

## Production Deployment

### 1. Update Environment Variables

```bash
# Edit docker-compose.yml or set in environment
SITES_PATH=/etc/proxy/sites-available
GLOBAL_CONFIG=/etc/proxy/global.yaml
DEBUG=0
```

### 2. Configure Global Settings

```yaml
# Edit global.yaml
tls:
  auto_cert: true
  cert_email: your-email@example.com
  cache_dir: /etc/proxy/certs

blackhole:
  unknown_domains: true
  metrics_only: true
```

### 3. Add Real Domains

```yaml
# sites-available/production.yaml
enabled: true

service:
  name: production-app

routes:
  - domains:
      - example.com
      - www.example.com
    path: /
    backend: http://app:8080

headers:
  Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
  X-Frame-Options: DENY

options:
  health_check_path: /health
  timeout: 30s
```

### 4. Deploy

```bash
# Build
docker build -t your-registry.com/proxy-manager:latest .

# Push
docker push your-registry.com/proxy-manager:latest

# Deploy (Docker Swarm)
docker stack deploy -c docker-compose.swarm.yml proxy

# Or (Docker Compose)
docker-compose up -d
```

### 5. Verify

```bash
# Check all services
docker ps | grep proxy

# Check logs
docker logs -f proxy_proxy.1.xxx

# Test domains
curl -I https://example.com

# Check certificate
openssl s_client -connect example.com:443 -servername example.com
```

## Troubleshooting

### Proxy won't start
```bash
# Check logs
docker-compose logs proxy

# Common issues:
# - Port already in use: sudo netstat -tulpn | grep :80
# - Invalid YAML: yamllint sites-available/*.yaml
# - Missing directories: mkdir -p /etc/proxy/certs
```

### Site not loading
```bash
# Check if config is loaded
docker logs proxy | grep "Loaded site config"

# Check if route exists
curl http://localhost:8080/metrics

# Verify backend is reachable
docker exec proxy ping -c 1 myapp
docker exec proxy curl http://myapp:8080/health
```

### HTTP/3 not working
```bash
# Check UDP port is exposed
docker ps | grep 443/udp

# Test with curl (HTTP/3 support)
curl --http3 https://example.com

# Or use browser DevTools → Network → Protocol column
```

### Blackhole count increasing
```bash
# Check which domains are being blackholed
docker logs proxy | grep blackhole

# Common causes:
# - Bots scanning for vulnerabilities
# - Misconfigured DNS
# - Old bookmarks to removed domains
```

## Cleanup

```bash
# Stop everything
docker-compose down

# Remove volumes
docker-compose down -v

# Remove old backups
rm -f *.old
```

## Next Steps

- Read [README.md](./README.md) for full documentation
- Check [MIGRATION_COMPLETE.md](./MIGRATION_COMPLETE.md) for architecture details
- Review [example-*.yaml](./example-simple.yaml) for more config examples
- Explore [Makefile](./Makefile) for available commands
