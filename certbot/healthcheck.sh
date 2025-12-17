#!/bin/bash
# ============================================================================
# Certbot Health Check Script
# ============================================================================
# Verifies that:
# 1. Storage Box is mounted (if not using local storage)
# 2. Certificates exist and are valid
# ============================================================================

set -e

# Check if /etc/letsencrypt is mounted from host (use mountinfo for reliability)
if grep -q " /etc/letsencrypt " /proc/self/mountinfo; then
    LINE=$(grep " /etc/letsencrypt " /proc/self/mountinfo | head -1)
    FSTYPE=$(echo "$LINE" | awk -F' - ' '{print $2}' | awk '{print $1}')
    SOURCE=$(echo "$LINE" | awk -F' - ' '{print $2}' | awk '{print $2}')
    PROP=$(echo "$LINE" | awk '{for(i=1;i<=NF;i++){if($i~/(shared|master):/){print $i;exit}}}')
    echo "✓ Mount present at /etc/letsencrypt (type=${FSTYPE:-unknown}, source=${SOURCE:-unknown}, prop=${PROP:-n/a})"
else
    echo "WARNING: No mount detected at /etc/letsencrypt (using local storage)"
fi

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
# Use openssl's -checkend instead of date parsing for better compatibility
EXPIRY_DATE=$(openssl x509 -in "/etc/letsencrypt/live/$FIRST_DOMAIN/fullchain.pem" -noout -enddate | cut -d= -f2)

# Calculate days until expiry using openssl -checkend and binary search approach
# First, check if already expired (checkend 0 means check if expired now)
if ! openssl x509 -in "/etc/letsencrypt/live/$FIRST_DOMAIN/fullchain.pem" -noout -checkend 0 > /dev/null 2>&1; then
    echo "Certificate has EXPIRED! (was valid until: $EXPIRY_DATE)"
    exit 1
fi

# Estimate days left by checking 30 days ahead
if ! openssl x509 -in "/etc/letsencrypt/live/$FIRST_DOMAIN/fullchain.pem" -noout -checkend 2592000 > /dev/null 2>&1; then
    # Less than 30 days
    # Find exact days by checking day by day
    DAYS_LEFT=0
    for days in {1..30}; do
        SECONDS=$((days * 86400))
        if openssl x509 -in "/etc/letsencrypt/live/$FIRST_DOMAIN/fullchain.pem" -noout -checkend $SECONDS > /dev/null 2>&1; then
            DAYS_LEFT=$days
        else
            break
        fi
    done
    echo "WARNING: Certificate expires in $DAYS_LEFT days (expires: $EXPIRY_DATE)"
else
    # More than 30 days - estimate by checking 60, 90 days
    if openssl x509 -in "/etc/letsencrypt/live/$FIRST_DOMAIN/fullchain.pem" -noout -checkend 7776000 > /dev/null 2>&1; then
        echo "✓ Certificate is valid (90+ days remaining, expires: $EXPIRY_DATE)"
    elif openssl x509 -in "/etc/letsencrypt/live/$FIRST_DOMAIN/fullchain.pem" -noout -checkend 5184000 > /dev/null 2>&1; then
        echo "✓ Certificate is valid (60+ days remaining, expires: $EXPIRY_DATE)"
    else
        echo "✓ Certificate is valid (30+ days remaining, expires: $EXPIRY_DATE)"
    fi
fi

exit 0
