# Verification Checklist

Use this checklist to verify the certificate refactoring is working correctly.

## ✅ Pre-Flight Checks

- [ ] All Go source files compile without errors
- [ ] No references to `autocert` remain in code
- [ ] `global.yaml` uses new certificate list format
- [ ] Certificate files exist and are readable
- [ ] Docker image builds successfully

## 🔧 Build & Test

### 1. Generate Test Certificates

```bash
cd /media/chilla55/New\ Volume/__________Docker/docker-images/go-proxy

# Generate self-signed cert
make gen-test-cert DOMAIN=test.local

# Verify files were created
ls -lh certs/test.local/
```

Expected output:
```
-rw-r--r-- fullchain.pem  (certificate)
-rw------- privkey.pem    (private key)
```

### 2. Update global.yaml

Ensure `global.yaml` has the certificate configuration:
```yaml
tls:
  certificates:
    - domains:
        - "*.test.local"
        - "test.local"
      cert_file: /etc/proxy/certs/test.local/fullchain.pem
      key_file: /etc/proxy/certs/test.local/privkey.pem
```

### 3. Build Docker Image

```bash
make docker-build
```

Expected:
- ✅ Build completes without errors
- ✅ Image tagged as `proxy-manager:latest`

### 4. Start Services

```bash
# Using docker-compose
docker-compose up -d

# Check logs
docker-compose logs -f proxy
```

Expected log output:
```
[proxy-manager] Starting unified reverse proxy service
[proxy-manager] Loaded certificate for domains: [*.test.local test.local]
[proxy-manager] Loaded 1 TLS certificate(s)
[proxy-manager] Starting HTTP server on :80
[proxy-manager] Starting HTTPS server on :443 (HTTP/2 enabled)
[proxy-manager] Starting HTTP/3 server on :443
[proxy-manager] All services started successfully
```

## 🧪 Functional Tests

### 5. Health Check

```bash
curl http://localhost:8080/health
```

Expected: `healthy`

### 6. Metrics Endpoint

```bash
curl http://localhost:8080/metrics
```

Expected: Prometheus-format metrics including `blackhole_requests_total`

### 7. Certificate Verification

```bash
# Check certificate info
openssl s_client -connect localhost:443 -servername demo.test.local </dev/null 2>/dev/null | \
  openssl x509 -noout -subject -issuer -dates
```

Expected:
```
subject=CN = *.test.local
issuer=CN = *.test.local
notBefore=...
notAfter=...
```

### 8. HTTP → HTTPS Redirect

```bash
curl -I http://localhost/
```

Expected: `301 Moved Permanently` with `Location: https://...`

### 9. HTTPS Request (with test domain)

Add to `/etc/hosts`:
```bash
echo "127.0.0.1 demo.test.local" | sudo tee -a /etc/hosts
```

Test HTTPS (ignore self-signed cert):
```bash
curl -k https://demo.test.local/
```

Expected: Response from whoami container

### 10. HTTP/2 Support

```bash
curl -k --http2 -I https://demo.test.local/
```

Expected: Headers include `HTTP/2 200`

### 11. HTTP/3 Support (if udp port exposed)

```bash
curl -k --http3 https://demo.test.local/
```

Expected: Response via HTTP/3 (requires curl with HTTP/3 support)

### 12. Wildcard Certificate Matching

Test multiple subdomains:
```bash
# Add more test domains
echo "127.0.0.1 api.test.local" | sudo tee -a /etc/hosts
echo "127.0.0.1 app.test.local" | sudo tee -a /etc/hosts

# All should use the same wildcard certificate
for subdomain in demo api app; do
  echo "Testing ${subdomain}.test.local..."
  curl -k -I https://${subdomain}.test.local/ 2>/dev/null | head -1
done
```

Expected: All succeed with same certificate

### 13. Unknown Domain Blackhole

```bash
curl -v -H "Host: unknown.com" http://localhost/ 2>&1 | grep -E "(Empty reply|Connection.*closed)"
```

Expected: Connection closed immediately (blackholed)

### 14. Service Registry (Dynamic Routes)

```bash
# Register a new route dynamically
(
  echo "ROUTE api.test.local:/v1 http://backend:8080"
  sleep 1
) | nc localhost 81
```

Expected: `OK ROUTE_ADDED` or similar response

### 15. Configuration Reload

```bash
# Modify a site config
echo "# test change" >> sites-available/demo.yaml

# Watch logs for reload
docker-compose logs -f proxy | grep -i reload
```

Expected: Site configuration reloaded automatically

## 🔍 Code Verification

### 16. No Autocert References

```bash
cd proxy-manager
grep -r "autocert" . --exclude-dir=vendor
```

Expected: No results (or only in comments)

### 17. Certificate Loading Code

```bash
cd proxy-manager
grep -n "LoadX509KeyPair" main.go proxy/proxy.go
```

Expected: References to `tls.LoadX509KeyPair` in `main.go`

### 18. Wildcard Matching Logic

```bash
cd proxy-manager
grep -n "matchWildcard" proxy/proxy.go
```

Expected: Implementation of wildcard matching function

## 📊 Performance Tests

### 19. Concurrent Requests

```bash
# Install ab (apache bench) if needed
sudo apt install apache2-utils

# Test with 100 concurrent connections
ab -n 1000 -c 100 -k https://demo.test.local/
```

Expected:
- ✅ All requests succeed
- ✅ No failed requests
- ✅ Low latency

### 20. HTTP/3 Performance

```bash
# If you have h2load with HTTP/3 support
h2load --h3 -n 1000 -c 100 https://demo.test.local/
```

## 🚀 Production Readiness

### 21. Multiple Certificates

Update `global.yaml` with multiple certificates:
```yaml
tls:
  certificates:
    - domains:
        - "*.example.com"
        - "example.com"
      cert_file: /etc/proxy/certs/example.com/fullchain.pem
      key_file: /etc/proxy/certs/example.com/privkey.pem
    
    - domains:
        - "*.test.local"
        - "test.local"
      cert_file: /etc/proxy/certs/test.local/fullchain.pem
      key_file: /etc/proxy/certs/test.local/privkey.pem
```

Restart and verify:
```bash
docker-compose restart proxy
docker-compose logs proxy | grep "Loaded.*certificate"
```

Expected: `Loaded 2 TLS certificate(s)`

### 22. Certificate File Permissions

```bash
ls -l certs/test.local/
```

Expected:
- Certificate: `-rw-r--r--` (644)
- Private key: `-rw-------` (600)

### 23. Read-Only Mount Verification

```bash
# Verify certificates are mounted read-only
docker-compose exec proxy touch /etc/proxy/certs/test.txt 2>&1
```

Expected: `Read-only file system` error

### 24. Non-Root User

```bash
docker-compose exec proxy whoami
```

Expected: `proxy` (not root)

### 25. Docker Swarm Compatibility

```bash
# Build and deploy to swarm (if available)
docker build -t proxy-manager:test .
docker stack deploy -c docker-compose.swarm.yml test-proxy
```

Expected: Services deploy successfully

## 📝 Documentation Checks

- [ ] README.md updated with certificate setup
- [ ] CERTIFICATE_SETUP.md created with detailed guide
- [ ] QUICKSTART.md includes certificate prerequisites
- [ ] CERTIFICATE_REFACTOR.md documents all changes
- [ ] Example configs use new certificate format
- [ ] Makefile includes `gen-test-cert` target

## 🎯 Final Checklist

- [ ] All tests pass
- [ ] No errors in logs
- [ ] Certificates load correctly
- [ ] Wildcard matching works
- [ ] Multiple certificates supported
- [ ] Blackhole functionality works
- [ ] Service registry operational
- [ ] HTTP/2 and HTTP/3 enabled
- [ ] Security headers applied
- [ ] Non-root user enforced
- [ ] Documentation complete

## 🐛 Common Issues

### Issue: "No certificate available for domain"

**Check:**
1. Certificate paths in `global.yaml` are correct
2. Files mounted to `/etc/proxy/certs` correctly
3. Domain patterns match request hostname

### Issue: "Failed to load certificate"

**Check:**
1. Certificate format is PEM
2. Private key matches certificate
3. File permissions allow reading (644/600)
4. Files are not corrupted

### Issue: Container won't start

**Check:**
1. `docker-compose logs proxy` for errors
2. Global.yaml syntax is valid YAML
3. At least one certificate is configured
4. Certificate files exist before starting

### Issue: "Empty reply from server"

**Possible causes:**
1. Domain not registered (blackholed) - expected behavior
2. Backend not available
3. Health check failing

## ✅ Success Criteria

All tests pass and:
- ✅ Proxy starts without errors
- ✅ Certificates load successfully
- ✅ HTTPS works with wildcard domains
- ✅ HTTP/2 and HTTP/3 enabled
- ✅ Blackhole works for unknown domains
- ✅ Service registry accepts dynamic routes
- ✅ Security headers applied correctly
- ✅ Health and metrics endpoints respond
- ✅ No autocert code remains
- ✅ Documentation is complete

---

**Last Updated:** $(date)
**Status:** Ready for Testing ✅
