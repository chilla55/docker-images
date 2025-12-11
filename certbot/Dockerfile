# ============================================================================
# Certbot with Cloudflare DNS and Host-Mounted Storage Box
# ============================================================================
# This image extends the official certbot/dns-cloudflare image
# Certificates are stored on host-mounted Storage Box at /etc/letsencrypt
# ============================================================================

FROM certbot/dns-cloudflare:latest

LABEL maintainer="chilla55"
LABEL description="Certbot with Cloudflare DNS for host-mounted Hetzner Storage Box"

# Install bash and curl for scripting
RUN apk add --no-cache \
    bash \
    curl \
    && rm -rf /var/cache/apk/*

# Create necessary directories
RUN mkdir -p /scripts

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
