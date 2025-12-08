# Certbot + Hetzner Storage Box Architecture

## System Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         Docker Swarm Cluster                             │
│                                                                          │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │                    Certbot Service                              │    │
│  │                                                                 │    │
│  │  ┌──────────────────────────────────────────────────────┐     │    │
│  │  │  1. Pull existing certs from Storage Box (startup)   │     │    │
│  │  │  2. Check if certificates exist                      │     │    │
│  │  │  3. Obtain/Renew via Cloudflare DNS challenge        │◄────┼────┼──► Cloudflare API
│  │  │  4. Fix permissions (gid 1001 for nginx)            │     │    │
│  │  │  5. Sync to Storage Box via rclone                   │◄────┼────┼──► Hetzner Storage Box
│  │  │  6. Signal nginx containers to reload                │     │    │         (SFTP/WebDAV)
│  │  │  7. Sleep for RENEW_INTERVAL (default 12h)          │     │    │
│  │  │  8. Repeat from step 2                               │     │    │
│  │  └──────────────────────────────────────────────────────┘     │    │
│  │                                                                 │    │
│  │  Volumes: /etc/letsencrypt (certbot_data)                     │    │
│  │  Secrets: cloudflare_credentials, rclone_config                │    │
│  └────────────────────────────────────────────────────────────────┘    │
│                                  │                                       │
│                                  │ (shared volume)                       │
│                                  ▼                                       │
│  ┌────────────────────────────────────────────────────────────────┐    │
│  │                    Nginx Service (replicas: 2)                 │    │
│  │                                                                 │    │
│  │  Mounts: /etc/nginx/certs:ro (from certbot_data)              │    │
│  │                                                                 │    │
│  │  Uses:                                                          │    │
│  │    ssl_certificate /etc/nginx/certs/live/domain/fullchain.pem │    │
│  │    ssl_certificate_key /etc/nginx/certs/live/domain/privkey.pem│   │
│  └────────────────────────────────────────────────────────────────┘    │
│                                  │                                       │
└──────────────────────────────────┼───────────────────────────────────────┘
                                   │
                                   ▼
                            Public Internet (HTTPS)
```

## Data Flow

### Certificate Acquisition Flow

```
User Domain (example.com)
        │
        ▼
   Cloudflare DNS
        │
        ├──► Certbot requests certificate
        │       │
        │       └──► Let's Encrypt validates via DNS-01 challenge
        │              │
        │              └──► Cloudflare API receives TXT record
        │                     │
        ▼                     │
   Certificate issued  ◄──────┘
        │
        ├──► Saved to /etc/letsencrypt/
        │
        ├──► Synced to Hetzner Storage Box
        │
        └──► Nginx reloaded to use new cert
```

### Storage Box Sync Flow

```
Certbot Container                    Hetzner Storage Box
/etc/letsencrypt/                    (SFTP: u123456.your-storagebox.de:23)
                                     /certs/
    │                                    │
    ├─ accounts/                         ├─ accounts/
    ├─ archive/                          ├─ archive/
    │  └─ example.com/                   │  └─ example.com/
    │     ├─ cert1.pem                   │     ├─ cert1.pem
    │     ├─ chain1.pem                  │     ├─ chain1.pem
    │     ├─ fullchain1.pem              │     ├─ fullchain1.pem
    │     └─ privkey1.pem                │     └─ privkey1.pem
    ├─ live/                             ├─ live/
    │  └─ example.com/                   │  └─ example.com/
    │     ├─ cert.pem → ../../archive... │     ├─ cert.pem (symlink)
    │     ├─ chain.pem → ../../archive...│     ├─ chain.pem (symlink)
    │     ├─ fullchain.pem → ...         │     ├─ fullchain.pem (symlink)
    │     └─ privkey.pem → ...           │     └─ privkey.pem (symlink)
    └─ renewal/                          └─ renewal/
       └─ example.com.conf                  └─ example.com.conf

              │                                    │
              └────────► rclone sync ─────────────┘
                        (bidirectional)
```

## Component Interactions

### Startup Sequence

```
1. Container Start
   ├─► Load Cloudflare credentials from secret
   ├─► Load rclone config from secret
   └─► Setup complete

2. Pull from Storage Box
   ├─► Check if remote path exists
   ├─► If exists: rclone sync from Storage Box → local
   └─► If not: OK (first-time setup)

3. Certificate Check
   ├─► Check if certificate exists locally
   ├─► If yes: Continue to renewal loop
   └─► If no: Obtain new certificate
          ├─► Certbot certonly with Cloudflare DNS
          ├─► Fix permissions
          └─► Push to Storage Box

4. Renewal Loop
   └─► Every RENEW_INTERVAL:
       ├─► certbot renew
       ├─► If renewed:
       │   ├─► Fix permissions
       │   ├─► Sync to Storage Box
       │   └─► Reload nginx
       └─► Sleep until next check
```

### Nginx Reload Mechanism

```
Certbot Container
    │
    ├─► Access Docker socket (/var/run/docker.sock)
    │
    ├─► Find nginx container by service name pattern
    │   (docker ps --filter "name=nginx_nginx")
    │
    ├─► Get container ID
    │
    └─► Execute reload command
        (docker exec <container_id> nginx -s reload)
            │
            ▼
        Nginx Container
            │
            └─► Reloads config and picks up new certificates
```

## Volume Sharing

### Certbot creates volume:

```yaml
volumes:
  certbot_data:
    driver: local
```

### Nginx references it:

```yaml
volumes:
  certbot_certs:
    external: true
    name: certbot_certbot_data  # Stack name + service name + volume name
```

### Both mount it:

**Certbot:**
```yaml
volumes:
  - certbot_data:/etc/letsencrypt  # Read-write
```

**Nginx:**
```yaml
volumes:
  - certbot_certs:/etc/nginx/certs:ro  # Read-only
```

## Security Layers

```
┌─────────────────────────────────────────────────┐
│ Docker Secrets (encrypted at rest)              │
│  ├─ cloudflare_credentials                      │
│  │   └─ Contains: API token                     │
│  └─ rclone_config                               │
│      └─ Contains: Encrypted password            │
└─────────────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────────┐
│ Container (runtime secrets in /run/secrets/)    │
│  ├─ Secrets mounted as files (tmpfs)            │
│  ├─ Not visible in docker inspect               │
│  └─ Automatically deleted on container stop     │
└─────────────────────────────────────────────────┘
                    │
                    ▼
┌─────────────────────────────────────────────────┐
│ External Services (TLS encrypted)               │
│  ├─ Cloudflare API (HTTPS)                      │
│  ├─ Let's Encrypt (HTTPS)                       │
│  └─ Hetzner Storage Box (SFTP/HTTPS)            │
└─────────────────────────────────────────────────┘
```

## Disaster Recovery Scenarios

### Scenario 1: Lost Local Certificates

```
1. Deploy fresh certbot container
   │
   └─► Startup pulls certs from Storage Box
       │
       └─► Nginx continues serving with valid certs
```

### Scenario 2: Storage Box Unavailable

```
1. Certbot obtains/renews certificates normally
   │
   ├─► Saves locally to volume
   │   └─► Nginx uses local certs (still works)
   │
   └─► Sync to Storage Box fails (logged but not fatal)
       │
       └─► Will retry on next renewal cycle
```

### Scenario 3: Complete Cluster Rebuild

```
1. New cluster deployed with same secrets
   │
   ├─► Certbot pulls certs from Storage Box
   │   └─► All certificates restored
   │
   └─► Nginx starts with existing certs
       │
       └─► No downtime, no re-issuance needed
```

## Network Communication

```
Certbot Container                          External Services
    │                                           │
    ├─► Port 443 (outbound) ──────────────────► Let's Encrypt API
    │                                           │
    ├─► Port 443 (outbound) ──────────────────► Cloudflare API
    │                                           │
    └─► Port 23 (SFTP outbound) ──────────────► Hetzner Storage Box
        or Port 443 (WebDAV outbound)

No inbound ports needed!
(DNS-01 challenge = no port 80/443 exposure required)
```

## Benefits of This Architecture

1. ✅ **High Availability**: Nginx replicas share same certs
2. ✅ **Disaster Recovery**: Storage Box backup
3. ✅ **Zero Downtime**: Nginx reload, not restart
4. ✅ **Automatic**: No manual intervention
5. ✅ **Secure**: Docker secrets, encrypted transfer
6. ✅ **Scalable**: Add domains easily
7. ✅ **Observable**: Health checks and logs
8. ✅ **Wildcard Support**: Via DNS-01 challenge
9. ✅ **Multi-node**: Works across Docker Swarm nodes
10. ✅ **Version History**: Storage Box snapshots
