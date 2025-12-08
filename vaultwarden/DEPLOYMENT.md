# Vaultwarden Deployment Checklist

Complete deployment guide for Vaultwarden in Docker Swarm with Storage Box integration.

## Pre-Deployment

### 1. Storage Box Setup

- [ ] SSH access to Storage Box configured
- [ ] Create directory structure:
```bash
ssh u123456@u123456.your-storagebox.de
mkdir -p /backup/vaultwarden/keys
chmod 700 /backup/vaultwarden/keys
exit
```

### 2. Docker Networks

- [ ] Create encrypted overlay networks:
```bash
docker network create --driver overlay --opt encrypted=true web-net
docker network create --driver overlay --opt encrypted=true mariadb-net
```

### 3. MariaDB Database

- [ ] MariaDB stack deployed and healthy
- [ ] MaxScale orchestrator running
- [ ] Create Vaultwarden database:
```sql
docker exec -it $(docker ps -q -f name=mariadb-primary) mysql -u root -p

CREATE DATABASE IF NOT EXISTS vaultwarden 
  CHARACTER SET utf8mb4 
  COLLATE utf8mb4_unicode_ci;

CREATE USER IF NOT EXISTS 'vaultwarden'@'%' 
  IDENTIFIED BY 'your-secure-db-password';

GRANT ALL PRIVILEGES ON vaultwarden.* TO 'vaultwarden'@'%';
FLUSH PRIVILEGES;
EXIT;
```

### 4. Generate Admin Token

- [ ] Generate argon2id hash:
```bash
docker run --rm -it vaultwarden/server /vaultwarden hash --preset owasp
# Enter your admin password when prompted
# Copy the hash (starts with $argon2id$...)
```

### 5. Create Docker Secrets

- [ ] Database password:
```bash
echo "your-secure-db-password" | docker secret create vaultwarden_db_password -
```

- [ ] Admin token:
```bash
echo '$argon2id$v=19$m=65540,t=3,p=4$...' | docker secret create vaultwarden_admin_token -
```

- [ ] SMTP password:
```bash
echo "your-smtp-password" | docker secret create vaultwarden_smtp_password -
```

- [ ] Storage Box password (if not already created):
```bash
echo "your-storagebox-password" | docker secret create storagebox_password -
```

- [ ] Verify secrets created:
```bash
docker secret ls | grep vaultwarden
docker secret ls | grep storagebox
```

### 6. Node Labels

- [ ] Label node for Vaultwarden placement:
```bash
# List nodes
docker node ls

# Label node1
docker node update --label-add vaultwarden.node=node1 <node1-name>

# Verify
docker node inspect <node1-name> | jq '.[0].Spec.Labels'
```

### 7. Configure Environment

- [ ] Update `docker-compose.swarm.yml` settings:
  - `VAULTWARDEN_DOMAIN`: Your domain (e.g., https://vault.example.com)
  - `STORAGE_BOX_HOST`: Your Storage Box hostname
  - `STORAGE_BOX_USER`: Your Storage Box username
  - `SMTP_HOST`: Your SMTP server
  - `SMTP_FROM`: From email address
  - `SMTP_USERNAME`: SMTP username
  - `SIGNUPS_ALLOWED`: false (recommended for production)

### 8. Build Image

- [ ] Build locally:
```bash
cd vaultwarden
make build
```

OR

- [ ] Push to GitHub for multi-arch build:
```bash
git add .
git commit -m "Add Vaultwarden v1.0.0"
git tag vaultwarden-v1.0.0
git push origin main --tags
```

- [ ] Wait for GitHub Actions to complete
- [ ] Verify image pushed to GHCR:
```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u $GITHUB_USERNAME --password-stdin
docker pull ghcr.io/yourusername/vaultwarden:1.0.0
```

## Deployment

### 9. Deploy Stack

- [ ] Deploy to Swarm:
```bash
cd vaultwarden
docker stack deploy -c docker-compose.swarm.yml vaultwarden
```

- [ ] Verify service created:
```bash
docker service ls | grep vaultwarden
```

### 10. Monitor Startup

- [ ] Watch logs:
```bash
docker service logs -f vaultwarden_vaultwarden
```

- [ ] Look for:
  - ✅ "Mounting Storage Box for RSA keys..."
  - ✅ "Storage Box mounted at /data/keys"
  - ✅ "Rocket has launched from http://0.0.0.0:80"

- [ ] Check service status:
```bash
docker service ps vaultwarden_vaultwarden
```

Expected: State = Running

## Post-Deployment Verification

### 11. Storage Box Mount

- [ ] Verify CIFS mount:
```bash
docker exec $(docker ps -q -f name=vaultwarden) mount | grep vaultwarden
```

Expected: `//u123456.your-storagebox.de/backup/vaultwarden/keys on /data/keys type cifs`

- [ ] Check RSA keys on Storage Box:
```bash
ssh u123456@u123456.your-storagebox.de ls -la /backup/vaultwarden/keys/
```

Expected:
```
-rw------- 1 u123456 u123456 1675 date rsa_key.pem
-rw-r--r-- 1 u123456 u123456  451 date rsa_key.pub.pem
```

- [ ] Verify symlinks in container:
```bash
docker exec $(docker ps -q -f name=vaultwarden) ls -la /data/ | grep rsa
```

Expected:
```
lrwxrwxrwx 1 root root   24 date rsa_key.pem -> /data/keys/rsa_key.pem
lrwxrwxrwx 1 root root   28 date rsa_key.pub.pem -> /data/keys/rsa_key.pub.pem
```

### 12. Database Connectivity

- [ ] Test MaxScale connection:
```bash
docker exec $(docker ps -q -f name=vaultwarden) nc -zv maxscale 4006
```

Expected: `maxscale (10.x.x.x:4006) open`

- [ ] Check database tables created:
```bash
docker exec -it $(docker ps -q -f name=mariadb-primary) mysql -u vaultwarden -p vaultwarden -e "SHOW TABLES;"
```

Expected: Multiple Vaultwarden tables (users, ciphers, etc.)

### 13. Health Check

- [ ] Service health:
```bash
docker service ps vaultwarden_vaultwarden
```

Expected: Current State = Running X minutes ago

- [ ] Manual health check:
```bash
docker exec $(docker ps -q -f name=vaultwarden) /usr/local/bin/healthcheck.sh
```

Expected: Exit code 0 (success)

### 14. Network Connectivity

- [ ] From nginx container:
```bash
docker exec $(docker ps -q -f name=nginx) nc -zv vaultwarden 80
```

Expected: `vaultwarden (10.x.x.x:80) open`

- [ ] From external (via nginx):
```bash
curl -k https://vault.example.com
```

Expected: Vaultwarden web vault HTML

## Nginx Configuration

### 15. Add Reverse Proxy

- [ ] Create nginx config:
```bash
cat > /path/to/nginx/conf.d/vaultwarden.conf <<'EOF'
server {
    listen 443 ssl http2;
    server_name vault.example.com;

    ssl_certificate /etc/nginx/certs/vault.example.com/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/vault.example.com/privkey.pem;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options SAMEORIGIN always;
    add_header X-Content-Type-Options nosniff always;
    add_header X-XSS-Protection "1; mode=block" always;

    location / {
        proxy_pass http://vaultwarden:80;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebSocket support for notifications
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        
        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # Larger uploads for attachments
    client_max_body_size 525M;
}
EOF
```

- [ ] Reload nginx:
```bash
docker exec $(docker ps -q -f name=nginx) nginx -t
docker exec $(docker ps -q -f name=nginx) nginx -s reload
```

### 16. SSL Certificate

- [ ] Request certificate via certbot:
```bash
docker exec $(docker ps -q -f name=certbot) certbot certonly \
  --dns-cloudflare \
  --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini \
  -d vault.example.com
```

- [ ] Reload nginx after cert issued

## Testing

### 17. Web Vault Access

- [ ] Open browser: `https://vault.example.com`
- [ ] Verify Vaultwarden web vault loads
- [ ] Create account (if signups enabled) or login

### 18. Admin Panel

- [ ] Access admin panel: `https://vault.example.com/admin`
- [ ] Login with admin token password (not the hash)
- [ ] Verify all settings visible
- [ ] Check "Diagnostics" tab for any warnings

### 19. Email Functionality

- [ ] In admin panel: Send test email
- [ ] Verify email received
- [ ] Create new account and verify email verification works

### 20. Client Apps

- [ ] Download Bitwarden browser extension
- [ ] Configure server URL: `https://vault.example.com`
- [ ] Login with test account
- [ ] Create test vault item
- [ ] Verify sync works

## Backup & Recovery Testing

### 21. Backup Test

- [ ] Stop Vaultwarden:
```bash
docker service scale vaultwarden_vaultwarden=0
```

- [ ] Download RSA keys from Storage Box:
```bash
scp u123456@u123456.your-storagebox.de:/backup/vaultwarden/keys/* ./backup/
```

- [ ] Verify keys downloaded

- [ ] Restart Vaultwarden:
```bash
docker service scale vaultwarden_vaultwarden=1
```

### 22. Recovery Test

- [ ] Delete keys from Storage Box:
```bash
ssh u123456@u123456.your-storagebox.de rm /backup/vaultwarden/keys/*
```

- [ ] Upload backup keys:
```bash
scp ./backup/* u123456@u123456.your-storagebox.de:/backup/vaultwarden/keys/
```

- [ ] Restart Vaultwarden:
```bash
docker service update --force vaultwarden_vaultwarden
```

- [ ] Verify keys loaded from Storage Box
- [ ] Login to web vault - should work with restored keys

## Monitoring

### 23. Setup Monitoring

- [ ] Add healthcheck monitoring (if using monitoring stack)
- [ ] Configure log aggregation for Vaultwarden logs
- [ ] Set up alerts for:
  - Service down
  - Health check failures
  - High memory usage
  - Storage Box mount failures

### 24. Log Rotation

- [ ] Configure Docker log rotation:
```bash
# In /etc/docker/daemon.json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}
```

## Production Hardening

### 25. Security Review

- [ ] Disable signups: `SIGNUPS_ALLOWED=false`
- [ ] Enable invitation-only: `INVITATIONS_ALLOWED=true`
- [ ] Verify admin token is strong argon2id hash
- [ ] Confirm all secrets using Docker secrets (not env vars)
- [ ] Check network encryption enabled on overlay networks
- [ ] Verify no unnecessary ports exposed

### 26. Performance Tuning

- [ ] Adjust resource limits if needed:
```yaml
resources:
  limits:
    cpus: '1'
    memory: 512M
  reservations:
    cpus: '0.5'
    memory: 256M
```

- [ ] Monitor resource usage:
```bash
docker stats $(docker ps -q -f name=vaultwarden)
```

## Documentation

### 27. Update Documentation

- [ ] Document admin token location (secure password manager)
- [ ] Document database credentials
- [ ] Document Storage Box credentials
- [ ] Create runbook for common operations:
  - Restart service
  - Backup RSA keys
  - Restore from backup
  - Scale to different node

### 28. Team Training

- [ ] Share admin panel credentials with team
- [ ] Document how to create new users (invitations)
- [ ] Provide client setup instructions
- [ ] Share troubleshooting guide

## Post-Deployment Cleanup

### 29. Remove Test Data

- [ ] Delete test accounts
- [ ] Remove test vault items
- [ ] Clear test emails

### 30. Final Checks

- [ ] All checklist items completed
- [ ] Service healthy and stable
- [ ] Backups working
- [ ] Monitoring configured
- [ ] Documentation updated
- [ ] Team trained

---

## Rollback Procedure

If deployment fails:

1. Stop service:
```bash
docker service rm vaultwarden_vaultwarden
```

2. Remove secrets:
```bash
docker secret rm vaultwarden_admin_token vaultwarden_db_password vaultwarden_smtp_password
```

3. Drop database:
```bash
docker exec -it $(docker ps -q -f name=mariadb-primary) mysql -u root -p -e "DROP DATABASE vaultwarden; DROP USER 'vaultwarden'@'%';"
```

4. Remove Storage Box keys:
```bash
ssh u123456@u123456.your-storagebox.de rm -rf /backup/vaultwarden
```

---

## Success Criteria

✅ Service running without restarts
✅ Health checks passing
✅ Web vault accessible via HTTPS
✅ Admin panel accessible
✅ Email notifications working
✅ RSA keys on Storage Box
✅ Database connection stable
✅ Client apps can login
✅ Backups tested and working
