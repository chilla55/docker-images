# TLS Certificate Setup

The proxy supports loading existing wildcard SSL certificates from the filesystem. This guide explains how to configure them.

## Configuration Structure

Add your certificates to `global.yaml`:

```yaml
tls:
  certificates:
    - domains:
        - "*.example.com"
        - "example.com"
      cert_file: /etc/proxy/certs/example.com/fullchain.pem
      key_file: /etc/proxy/certs/example.com/privkey.pem
    
    - domains:
        - "*.app.example.com"
        - "app.example.com"
      cert_file: /etc/proxy/certs/app.example.com/fullchain.pem
      key_file: /etc/proxy/certs/app.example.com/privkey.pem
```

## Wildcard Certificate Matching

The proxy intelligently matches domains to certificates:

1. **Exact Match First**: `example.com` matches `example.com` domain in certificate
2. **Wildcard Match**: `sub.example.com` matches `*.example.com` pattern
3. **Fallback**: If no match found, uses the first certificate in the list

### Wildcard Rules

- `*.example.com` matches: `sub.example.com`, `api.example.com`, etc.
- `*.example.com` does NOT match: `example.com` (use both patterns)
- `*.example.com` does NOT match: `deep.sub.example.com` (only one level)

### Examples

```yaml
# Single wildcard covering all subdomains + apex
- domains:
    - "*.chilla55.de"
    - "chilla55.de"
  cert_file: /etc/proxy/certs/chilla55.de/fullchain.pem
  key_file: /etc/proxy/certs/chilla55.de/privkey.pem

# Multiple certificates for different domains
- domains:
    - "*.example.org"
    - "example.org"
  cert_file: /etc/proxy/certs/example.org/cert.pem
  key_file: /etc/proxy/certs/example.org/key.pem

- domains:
    - "*.api.example.com"
    - "api.example.com"
  cert_file: /etc/proxy/certs/api.example.com/cert.pem
  key_file: /etc/proxy/certs/api.example.com/key.pem
```

## Certificate File Paths

### Docker Compose

Mount your certificate directory:

```yaml
volumes:
  - /path/to/your/certs:/etc/proxy/certs:ro
  - ./global.yaml:/etc/proxy/global.yaml:ro
```

### Docker Swarm

Use bind mounts with rslave propagation (required for storagebox mounts):

```yaml
volumes:
  - type: bind
    source: /mnt/storagebox/certs
    target: /etc/proxy/certs
    read_only: true
    bind:
      propagation: rslave
```

The `rslave` propagation ensures that mount changes on the host (like certificate renewals) are visible inside the container.

## Certificate Format

The proxy expects standard PEM format certificates.

### Let's Encrypt Certificate Files

Let's Encrypt (via Certbot) provides four files:
- **cert.pem**: Certificate only (not used by proxy)
- **chain.pem**: Intermediate certificates (not used by proxy)
- **fullchain.pem**: Certificate + intermediate chain ✅ **USE THIS**
- **privkey.pem**: Private key ✅ **USE THIS**

**For the proxy, use only `fullchain.pem` and `privkey.pem`:**

```yaml
tls:
  certificates:
    - domains:
        - "*.chilla55.de"
        - "chilla55.de"
      cert_file: /etc/proxy/certs/chilla55.de/fullchain.pem
      key_file: /etc/proxy/certs/chilla55.de/privkey.pem
```

### Other Certificate Formats

Also compatible with:
- Any CA-issued certificates in PEM format
- Self-signed certificates for testing
- Commercial SSL certificates (convert to PEM if needed)

## File Permissions

Ensure the proxy user can read certificate files:

```bash
# Set ownership (if running as user 1000)
chown -R 1000:1000 /path/to/certs

# Set permissions (certificates: 644, keys: 600)
chmod 644 /path/to/certs/**/fullchain.pem
chmod 600 /path/to/certs/**/privkey.pem
```

## Certificate Renewal

The proxy automatically detects certificate changes and hot-reloads them without restart.

### Automatic Reload (Certbot Compatible)

When Certbot renews certificates, the proxy automatically detects and reloads them:

1. **File Watcher**: Monitors certificate directories for changes
2. **Immediate Detection**: Reloads within 2 seconds of file changes
3. **Periodic Check**: Backup check every 5 minutes
4. **Zero Downtime**: No service interruption during reload

**How it works:**
- Watches directories containing `fullchain.pem` and `privkey.pem`
- Detects WRITE and CREATE events (Certbot file operations)
- Debounces rapid changes (5-second cooldown)
- Reloads all certificates atomically

**No manual intervention needed!** Just let Certbot renew certificates normally.

### Manual Reload (Optional)

If you need to force a certificate reload:

```bash
# For docker-compose
docker-compose restart nginx

# For swarm
docker service update --force nginx_nginx
```

### Certbot Auto-Renewal

Standard Certbot setup works automatically:

```bash
# Certbot auto-renewal (systemd timer or cron)
certbot renew --quiet

# The proxy will detect and reload certificates automatically
```

## Troubleshooting

### "No certificate available for domain"

Check:
1. Certificate paths in `global.yaml` are correct
2. Files exist and are readable by proxy user (1000:1000)
3. Domain patterns match your actual domains

### "Failed to load certificate"

Check:
1. Certificate is valid PEM format
2. Private key matches certificate
3. File permissions allow reading

### Verify Certificate Loading

Check logs on startup:
```bash
docker logs proxy_nginx 2>&1 | grep certificate
```

Expected output:
```
[proxy-manager] Loaded certificate for domains: [*.example.com example.com]
[proxy-manager] Loaded 2 TLS certificate(s)
```

## Example: Let's Encrypt Wildcard

If using Certbot for wildcard certificates:

```bash
# Get wildcard certificate
certbot certonly --manual --preferred-challenges dns \
  -d "*.chilla55.de" -d "chilla55.de"

# Certificates will be in:
# /etc/letsencrypt/live/chilla55.de/fullchain.pem  ← USE THIS
# /etc/letsencrypt/live/chilla55.de/privkey.pem    ← USE THIS
# /etc/letsencrypt/live/chilla55.de/cert.pem       (not needed)
# /etc/letsencrypt/live/chilla55.de/chain.pem      (not needed)
```

Configure in `global.yaml`:
```yaml
tls:
  certificates:
    - domains:
        - "*.chilla55.de"
        - "chilla55.de"
      cert_file: /etc/letsencrypt/live/chilla55.de/fullchain.pem
      key_file: /etc/letsencrypt/live/chilla55.de/privkey.pem
```

### For Swarm with Storagebox

If certificates are stored on `/mnt/storagebox/certs`:

```yaml
tls:
  certificates:
    - domains:
        - "*.chilla55.de"
        - "chilla55.de"
      cert_file: /etc/proxy/certs/chilla55.de/fullchain.pem
      key_file: /etc/proxy/certs/chilla55.de/privkey.pem
```

Mount in docker-compose.swarm.yml:
```yaml
volumes:
  - type: bind
    source: /mnt/storagebox/certs
    target: /etc/proxy/certs
    read_only: true
    bind:
      propagation: rslave
```

## Security Best Practices

1. **Read-only mounts**: Mount certificate directories as `:ro`
2. **Restrict permissions**: Private keys should be 600
3. **Separate directories**: Keep different domain certs in subdirectories
4. **Regular renewal**: Automate certificate renewal
5. **Backup certificates**: Keep secure backups of private keys

## Advanced: Certificate Rotation

The proxy supports zero-downtime certificate rotation:

1. **Automatic Detection**: File watcher detects certificate changes
2. **Hot Reload**: New certificates loaded without restart
3. **Atomic Update**: All certificates updated together
4. **Connection Preservation**: Existing connections continue on old cert
5. **New Connections**: Immediately use new certificate

### Monitoring Reloads

Watch the logs to see automatic reloads:

```bash
docker service logs -f nginx_nginx | grep cert-watcher
```

Expected output:
```
[cert-watcher] Started watching 1 certificate directories
[cert-watcher] Watching certificate directory: /etc/proxy/certs/chilla55.de
[cert-watcher] Certificate file changed: /etc/proxy/certs/chilla55.de/fullchain.pem
[cert-watcher] Reloading certificates from disk...
[cert-watcher] Loaded certificate for domains: [*.chilla55.de chilla55.de]
[cert-watcher] Successfully reloaded 1 certificate(s)
[proxy] Certificates updated: 1 certificate(s) loaded
```

## No Auto-Renewal - But Auto-Reload!

This proxy does NOT automatically renew certificates (unlike the old autocert setup). However, it **automatically reloads** certificates when they change.

**You must:**
1. ✅ Manage certificate renewal externally (Certbot, acme.sh, etc.)
2. ✅ Certificates are stored in mounted directory

**Proxy automatically:**
1. ✅ Detects when certificates are renewed
2. ✅ Reloads them without restart
3. ✅ Zero downtime during reload

**Perfect for:**
- Certbot with systemd timer or cron
- Any external certificate renewal system
- Manual certificate updates

This gives you full control over certificate management while providing automatic hot-reload for zero downtime.
