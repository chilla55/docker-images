#!/bin/bash
# ============================================================================
# Certbot Health Check Script
# ============================================================================
# Verifies that certificates exist and are valid
# ============================================================================

set -e

# Get the first domain from CERT_DOMAINS
FIRST_DOMAIN=$(echo "${CERT_DOMAINS:-example.com}" | cut -d',' -f1)

# Check if certificate directory exists
if [ ! -d "/etc/letsencrypt/live/$FIRST_DOMAIN" ]; then
    echo "Certificate directory not found: /etc/letsencrypt/live/$FIRST_DOMAIN"
    exit 1
fi

# Check if certificate file exists
if [ ! -f "/etc/letsencrypt/live/$FIRST_DOMAIN/fullchain.pem" ]; then
    echo "Certificate file not found: /etc/letsencrypt/live/$FIRST_DOMAIN/fullchain.pem"
    exit 1
fi

# Check certificate expiry (warn if less than 30 days)
EXPIRY_DATE=$(openssl x509 -in "/etc/letsencrypt/live/$FIRST_DOMAIN/fullchain.pem" -noout -enddate | cut -d= -f2)
EXPIRY_EPOCH=$(date -d "$EXPIRY_DATE" +%s)
NOW_EPOCH=$(date +%s)
DAYS_LEFT=$(( ($EXPIRY_EPOCH - $NOW_EPOCH) / 86400 ))

if [ $DAYS_LEFT -lt 0 ]; then
    echo "Certificate has EXPIRED!"
    exit 1
elif [ $DAYS_LEFT -lt 30 ]; then
    echo "WARNING: Certificate expires in $DAYS_LEFT days"
else
    echo "Certificate is valid for $DAYS_LEFT more days"
fi

exit 0
