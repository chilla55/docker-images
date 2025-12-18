# Certificate Refactoring Summary

## Changes Made

Replaced autocert (Let's Encrypt automatic certificate management) with manual wildcard certificate loading from the filesystem.

## Modified Files

### 1. `global.yaml`
**Before:**
```yaml
tls:
  auto_cert: true
  cert_email: admin@example.com
  cache_dir: /etc/proxy/certs
```

**After:**
```yaml
tls:
  certificates:
    - domains:
        - "*.example.com"
        - "example.com"
      cert_file: /etc/proxy/certs/example.com/fullchain.pem
      key_file: /etc/proxy/certs/example.com/privkey.pem
    
    - domains:
        - "*.chilla55.de"
        - "chilla55.de"
      cert_file: /etc/proxy/certs/chilla55.de/fullchain.pem
      key_file: /etc/proxy/certs/chilla55.de/privkey.pem
```

### 2. `nginx-manager/config/config.go`
- **Removed:** `AutoCert`, `CertEmail`, `CacheDir` fields from TLS struct
- **Added:** `Certificates []CertConfig` array
- **Added:** New `CertConfig` struct with `Domains`, `CertFile`, `KeyFile`

### 3. `nginx-manager/proxy/proxy.go`
- **Removed:** Import of `golang.org/x/crypto/acme/autocert`
- **Added:** Import of `strings` for wildcard matching
- **Removed:** `certManager *autocert.Manager` field
- **Added:** `certificates []CertMapping` field
- **Added:** New `CertMapping` struct with domains and TLS certificate
- **Removed:** `CertEmail`, `CertCacheDir`, `AllowedDomains` from Config
- **Added:** `Certificates []CertMapping` to Config
- **Modified:** `NewServer()` - removed autocert initialization
- **Modified:** `Start()` - removed autocert HTTP handler wrapper
- **Added:** `getCertificate()` - certificate selection with wildcard matching
- **Added:** `matchWildcard()` - wildcard domain pattern matching
- **Modified:** `tlsConfig()` - uses `getCertificate` instead of `certManager.GetCertificate`

### 4. `nginx-manager/main.go`
- **Added:** Import of `crypto/tls` for certificate loading
- **Added:** `loadCertificates()` - loads certs from config using `tls.LoadX509KeyPair()`
- **Modified:** `main()` - calls `loadCertificates()` and validates cert count
- **Modified:** `NewServer()` call - passes `Certificates` instead of cert config
- **Modified:** `getDefaultGlobalConfig()` - removed default autocert config

### 5. `docker-compose.yml`
- **Changed:** Volume mount from named volume to bind mount: `./certs:/etc/proxy/certs:ro`
- **Removed:** `proxy-certs` named volume definition

### 6. `sites-available/demo.yaml`
- **Changed:** Domains from `demo.localhost` to `demo.test.local`

### 7. Documentation Updates

**QUICKSTART.md:**
- Added certificate prerequisites section
- Added self-signed cert generation example
- Updated test domains to `*.test.local`
- Added certificate loading verification steps

**README.md:**
- Changed "Automatic HTTPS" to "Wildcard TLS Certificates"
- Added certificate mount example in docker-compose
- Added TLS configuration example
- Added link to CERTIFICATE_SETUP.md

**New file - CERTIFICATE_SETUP.md:**
- Complete guide for certificate configuration
- Wildcard matching rules and examples
- Certificate file format requirements
- Docker volume mounting examples
- Renewal and rotation procedures
- Troubleshooting section

## Key Features

### Wildcard Certificate Support

The proxy now supports wildcard certificates with intelligent matching:

1. **Exact match**: `example.com` → `example.com`
2. **Wildcard match**: `sub.example.com` → `*.example.com`
3. **Fallback**: Unknown domains use first certificate

### Wildcard Rules
- `*.example.com` matches one-level subdomains only
- Must include both `*.example.com` and `example.com` to cover all
- Pattern matching is case-insensitive

### Certificate Loading
- Uses `tls.LoadX509KeyPair()` to load PEM certificates
- Validates domains array is not empty
- Logs loaded certificates on startup
- Fails fast if certificates can't be loaded

## Benefits

1. **Full Control**: No automatic cert management, use any CA
2. **Wildcard Support**: Single cert covers all subdomains
3. **Multiple Certs**: Support different domains with different certificates
4. **Security**: Read-only certificate mounts
5. **Compatibility**: Works with Let's Encrypt, commercial CAs, self-signed
6. **Simplicity**: No ACME protocol complexity

## Migration from Old Setup

If you were using the old autocert setup:

1. Get wildcard certificates from your CA (e.g., Let's Encrypt with DNS challenge)
2. Place certificates in a directory structure:
   ```
   certs/
   ├── example.com/
   │   ├── fullchain.pem
   │   └── privkey.pem
   └── other.com/
       ├── fullchain.pem
       └── privkey.pem
   ```
3. Update `global.yaml` with certificate list
4. Mount certs directory to `/etc/proxy/certs`
5. Restart proxy

## Testing

Generate self-signed test certificates:
```bash
mkdir -p certs/test.local
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout certs/test.local/privkey.pem \
  -out certs/test.local/fullchain.pem \
  -days 365 -subj "/CN=*.test.local"
```

Configure in `global.yaml`:
```yaml
tls:
  certificates:
    - domains:
        - "*.test.local"
        - "test.local"
      cert_file: /etc/proxy/certs/test.local/fullchain.pem
      key_file: /etc/proxy/certs/test.local/privkey.pem
```

## Verification

After starting, check logs:
```bash
docker logs proxy_nginx 2>&1 | grep certificate
```

Expected output:
```
[proxy-manager] Loaded certificate for domains: [*.example.com example.com]
[proxy-manager] Loaded certificate for domains: [*.chilla55.de chilla55.de]
[proxy-manager] Loaded 2 TLS certificate(s)
```

## Security Considerations

1. **Permissions**: Private keys must be readable by proxy user (1000:1000)
2. **Read-only**: Always mount certificate directory as `:ro`
3. **No Secrets in Config**: Only file paths in config, not actual keys
4. **Certificate Validation**: LoadX509KeyPair validates cert/key pair match
5. **TLS 1.2+**: Enforced minimum TLS version

## No Breaking Changes for Dynamic Registry

The service registry (port 81) continues to work unchanged:
- `ROUTE domain.com,www.domain.com:/path http://backend:8080`
- Certificates are matched automatically based on domain
- No cert configuration needed in registry protocol

## Dependencies

- **Kept:** `golang.org/x/crypto` (for TLS utilities, not autocert)
- **Removed:** No longer using `acme/autocert` package
- All other dependencies unchanged

## Future Enhancements

Possible additions (not implemented):
- Certificate hot-reload without restart
- OCSP stapling
- Certificate expiry warnings in metrics
- Automatic cert validation on startup
