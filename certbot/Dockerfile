# ============================================================================
# Certbot with Cloudflare DNS and Hetzner Storage Box Support
# ============================================================================
# This image extends the official certbot/dns-cloudflare image with CIFS/Samba
# support for mounting Hetzner Storage Box directly
# ============================================================================

FROM certbot/dns-cloudflare:latest

LABEL maintainer="chilla55"
LABEL description="Certbot with Cloudflare DNS and Hetzner Storage Box (CIFS/Samba) support"

# Install cifs-utils for mounting Storage Box via CIFS
RUN apk add --no-cache \
    cifs-utils \
    curl \
    bash \
    && rm -rf /var/cache/apk/*

# Create necessary directories
RUN mkdir -p /scripts \
    && mkdir -p /mnt/storagebox

# Copy entrypoint and helper scripts
COPY entrypoint.sh /scripts/entrypoint.sh
COPY healthcheck.sh /scripts/healthcheck.sh

# Make scripts executable
RUN chmod +x /scripts/*.sh

# Health check
HEALTHCHECK --interval=6h --timeout=30s --start-period=10s --retries=3 \
    CMD /scripts/healthcheck.sh

# Use custom entrypoint
ENTRYPOINT ["/scripts/entrypoint.sh"]
