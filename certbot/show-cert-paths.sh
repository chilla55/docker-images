#!/bin/bash
# ============================================================================
# Helper Script - Show Certificate Paths for Nginx Configuration
# ============================================================================
# Usage: ./show-cert-paths.sh [domain]
# ============================================================================

set -e

DOMAIN="${1:-}"

if [ -z "$DOMAIN" ]; then
    echo "Usage: $0 <domain>"
    echo "Example: $0 example.com"
    exit 1
fi

# Remove wildcard asterisk if present and use base domain
BASE_DOMAIN=$(echo "$DOMAIN" | sed 's/^\*\.//')

cat << EOF
================================================================================
Nginx SSL Certificate Configuration for: $DOMAIN
================================================================================

In your nginx configuration file, use these paths:

server {
    listen 443 ssl http2;
    server_name $DOMAIN;
    
    # SSL Certificate Configuration
    ssl_certificate /etc/nginx/certs/live/$BASE_DOMAIN/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/live/$BASE_DOMAIN/privkey.pem;
    
    # Optional: Include SSL configuration
    # include /etc/nginx/conf.d/ssl-params.conf;
    
    # Your server configuration here
    location / {
        proxy_pass http://your-backend;
        # ... proxy settings
    }
}

================================================================================
Available Certificate Files:
================================================================================

/etc/nginx/certs/live/$BASE_DOMAIN/
  ├─ fullchain.pem    ← Use this for ssl_certificate
  ├─ privkey.pem      ← Use this for ssl_certificate_key
  ├─ cert.pem         (certificate only, no chain)
  └─ chain.pem        (intermediate certificates only)

================================================================================
Docker Compose Volume Configuration:
================================================================================

In your nginx docker-compose.swarm.yml:

services:
  nginx:
    volumes:
      - certbot_certs:/etc/nginx/certs:ro  # Read-only mount
      
volumes:
  certbot_certs:
    external: true
    name: certbot_certbot_data  # Reference certbot's volume

================================================================================
Recommended SSL Configuration:
================================================================================

# Create /etc/nginx/conf.d/ssl-params.conf:

ssl_protocols TLSv1.2 TLSv1.3;
ssl_ciphers HIGH:!aNULL:!MD5;
ssl_prefer_server_ciphers on;
ssl_session_cache shared:SSL:10m;
ssl_session_timeout 10m;
ssl_stapling on;
ssl_stapling_verify on;
resolver 8.8.8.8 8.8.4.4 valid=300s;
resolver_timeout 5s;

# Security headers
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
add_header X-Frame-Options "SAMEORIGIN" always;
add_header X-Content-Type-Options "nosniff" always;
add_header X-XSS-Protection "1; mode=block" always;

================================================================================
Testing Your Configuration:
================================================================================

# 1. Test nginx configuration
docker exec <nginx-container> nginx -t

# 2. Reload nginx
docker exec <nginx-container> nginx -s reload

# 3. Test SSL certificate
curl -vI https://$DOMAIN 2>&1 | grep -i 'subject:\|issuer:\|expire'

# Or use SSL Labs:
# https://www.ssllabs.com/ssltest/analyze.html?d=$DOMAIN

================================================================================
EOF
