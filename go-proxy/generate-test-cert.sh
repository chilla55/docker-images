#!/bin/bash
# Generate self-signed wildcard certificates for testing

set -e

DOMAIN=${1:-test.local}
CERT_DIR="certs/${DOMAIN}"

echo "Generating self-signed wildcard certificate for *.${DOMAIN}"
echo "Certificate will be valid for 365 days"
echo ""

# Create directory
mkdir -p "${CERT_DIR}"

# Generate certificate
openssl req -x509 -newkey rsa:4096 -nodes \
    -keyout "${CERT_DIR}/privkey.pem" \
    -out "${CERT_DIR}/fullchain.pem" \
    -days 365 \
    -subj "/CN=*.${DOMAIN}" \
    -addext "subjectAltName=DNS:*.${DOMAIN},DNS:${DOMAIN}"

# Set permissions
chmod 644 "${CERT_DIR}/fullchain.pem"
chmod 600 "${CERT_DIR}/privkey.pem"

echo ""
echo "✓ Certificate generated successfully!"
echo ""
echo "Certificate: ${CERT_DIR}/fullchain.pem"
echo "Private Key: ${CERT_DIR}/privkey.pem"
echo ""
echo "Add to global.yaml:"
echo ""
echo "tls:"
echo "  certificates:"
echo "    - domains:"
echo "        - \"*.${DOMAIN}\""
echo "        - \"${DOMAIN}\""
echo "      cert_file: /etc/proxy/certs/${DOMAIN}/fullchain.pem"
echo "      key_file: /etc/proxy/certs/${DOMAIN}/privkey.pem"
echo ""
echo "Add to /etc/hosts for testing:"
echo "127.0.0.1 test.${DOMAIN}"
echo "127.0.0.1 demo.${DOMAIN}"
echo ""
