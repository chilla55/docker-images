# Go-Based HTTP/3 Reverse Proxy

**Modern replacement for nginx** - High-performance reverse proxy written entirely in Go with native HTTP/3 (QUIC) support.

## 🚀 Why This Proxy?

- **Pure Go** - No nginx, no C dependencies, single binary
- **HTTP/3 Native** - QUIC protocol with 0-RTT connection establishment
- **Hot-Reload Everything** - Certificates, configurations, routes - all without restart
- **Dynamic Registry** - TCP protocol for runtime service registration
- **Wildcard Certificates** - Automatic detection and reload when Certbot renews certs
- **Blackhole Unknown Domains** - Instant connection drop for security
- **True Zero Downtime** - No interruption during updates

## 📂 Structure

```
go-proxy/
├── proxy-manager/          # Go source code
│   ├── config/            # YAML configuration parser
│   ├── proxy/             # HTTP/2/3 reverse proxy server
│   ├── registry/          # Dynamic service registry
│   └── watcher/           # File & certificate watchers
├── sites-available/       # YAML site configurations
├── global.yaml           # Global proxy configuration
├── docker-compose.swarm.yml  # Production deployment
└── Dockerfile            # Multi-stage Go build
```

## 🎯 Quick Start

See [QUICKSTART.md](QUICKSTART.md) for detailed setup instructions.

```bash
# Generate test certificates
make gen-test-cert DOMAIN=test.local

# Update global.yaml with your certificates
# Build and run
docker-compose up -d

# Check health
curl http://localhost:8080/health
```

## 📖 Documentation

- **[QUICKSTART.md](QUICKSTART.md)** - Get started in 5 minutes
- **[CERTIFICATE_SETUP.md](CERTIFICATE_SETUP.md)** - Wildcard cert configuration
- **[SWARM_DEPLOYMENT.md](SWARM_DEPLOYMENT.md)** - Production deployment guide
- **[VERIFICATION.md](VERIFICATION.md)** - Complete testing checklist
- **[MIGRATION_COMPLETE.md](MIGRATION_COMPLETE.md)** - Architecture deep dive

## 🔧 Features

### HTTP/3 (QUIC) Support
- Native Go implementation using quic-go
- 0-RTT connection establishment
- Multiplexed streams without head-of-line blocking
- UDP port 443 for HTTP/3

### Automatic Certificate Reload
- Watches certificate directories for changes
- Detects Certbot renewals automatically
- Hot-reloads without service restart
- Zero downtime during certificate updates

### Dynamic Service Registry
- TCP protocol on port 81
- Runtime route registration
- No configuration file changes needed
- Maintenance mode support

### Security
- Configurable security headers per domain
- Blackhole unknown domains instantly
- TLS 1.2+ enforced
- HSTS, CSP, X-Frame-Options support

## 🚢 Deployment

### Docker Compose (Testing)
```yaml
services:
  proxy:
    build: .
    ports:
      - "80:80"
      - "443:443/tcp"
      - "443:443/udp"  # HTTP/3
      - "81:81"        # Registry
      - "8080:8080"    # Metrics
    volumes:
      - ./sites-available:/etc/proxy/sites-available:ro
      - ./global.yaml:/etc/proxy/global.yaml:ro
      - /path/to/certs:/etc/proxy/certs:ro
```

### Docker Swarm (Production)
```bash
# Build and push
docker build -t ghcr.io/chilla55/go-proxy:latest .
docker push ghcr.io/chilla55/go-proxy:latest

# Deploy
docker stack deploy -c docker-compose.swarm.yml proxy
```

See [SWARM_DEPLOYMENT.md](SWARM_DEPLOYMENT.md) for complete production setup.

## 📝 Configuration

### Global Config (global.yaml)
```yaml
tls:
  certificates:
    - domains:
        - "*.chilla55.de"
        - "chilla55.de"
      cert_file: /etc/proxy/certs/chilla55.de/fullchain.pem
      key_file: /etc/proxy/certs/chilla55.de/privkey.pem

defaults:
  headers:
    Strict-Transport-Security: max-age=31536000; includeSubDomains
    X-Frame-Options: DENY
    X-Content-Type-Options: nosniff

blackhole:
  unknown_domains: true
```

### Site Config (sites-available/myapp.yaml)
```yaml
enabled: true

service:
  name: myapp

routes:
  - domains:
      - myapp.example.com
      - www.myapp.example.com
    path: /
    backend: http://myapp:8080

headers:
  X-Custom-Header: "value"

options:
  health_check_path: /health
  timeout: 30s
  compression: true
```

## 🔌 Service Registry Protocol

Register routes dynamically via TCP (port 81):

```bash
# Add route
echo "ROUTE domain.com:/path http://backend:8080" | nc localhost 81

# Add custom headers
echo "HEADER domain.com:X-Custom:value" | nc localhost 81

# Configure options
echo "OPTIONS domain.com:timeout=60s,websocket=true" | nc localhost 81
```

See examples in `registry/` documentation.

## 📊 Monitoring

### Health Check
```bash
curl http://localhost:8080/health
# Returns: healthy
```

### Metrics (Prometheus)
```bash
curl http://localhost:8080/metrics
```

Includes:
- `blackhole_requests_total` - Rejected unknown domains
- Certificate reload events (in logs)
- Backend health status

## 🆚 vs Legacy Nginx

| Feature | Go Proxy | Legacy Nginx |
|---------|----------|--------------|
| HTTP/3 | ✅ Native | ✅ Compiled module |
| Config Format | YAML | nginx.conf |
| Cert Reload | ✅ Automatic | ⚠️ Manual restart |
| Dynamic Routes | ✅ TCP Registry | ❌ File only |
| Binary Size | ~20MB | ~200MB+ |
| Dependencies | None | libc, PCRE, OpenSSL |
| Language | Go | C/Lua |
| Hot Reload | Everything | Config only |

## 🔄 Migration from Legacy Nginx

The legacy nginx container is preserved in `../nginx/` for backward compatibility.

To migrate:
1. Convert nginx.conf sites to YAML format
2. Update service registry calls (new protocol)
3. Configure certificates in global.yaml
4. Test with docker-compose
5. Deploy to swarm

See [MIGRATION_COMPLETE.md](MIGRATION_COMPLETE.md) for detailed migration guide.

## 🛠️ Development

### Build Locally
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

### Build Docker Image
```bash
make docker-build
# or
docker build -t go-proxy:latest .
```

## 📦 Requirements

- **Runtime**: Alpine Linux (Docker)
- **Build**: Go 1.21+
- **Certificates**: PEM format (Let's Encrypt compatible)
- **Ports**: 80, 443/tcp, 443/udp, 81, 8080

## 🤝 Contributing

This is a custom implementation for the docker-images infrastructure. For changes:

1. Test locally with `docker-compose`
2. Verify with checklist in `VERIFICATION.md`
3. Update documentation
4. Test in staging swarm
5. Deploy to production

## 📄 License

Internal use for chilla55 infrastructure.

## 🎓 Learn More

- **HTTP/3**: Uses [quic-go](https://github.com/quic-go/quic-go)
- **YAML Parsing**: Uses [yaml.v3](https://github.com/go-yaml/yaml)
- **File Watching**: Uses [fsnotify](https://github.com/fsnotify/fsnotify)

## 📞 Support

See documentation files or check logs:
```bash
# Docker Compose
docker-compose logs -f proxy

# Docker Swarm
docker service logs -f proxy_nginx
```

---

**Built with ❤️ in Go** - Replacing nginx one connection at a time.
