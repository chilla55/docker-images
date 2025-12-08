FROM vaultwarden/server:latest

# Install healthcheck dependencies and CIFS utilities
RUN apt-get update && apt-get install -y wget cifs-utils && rm -rf /var/lib/apt/lists/*

# Add scripts
COPY healthcheck.sh /usr/local/bin/healthcheck.sh
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY post-start.sh /usr/local/bin/post-start.sh
RUN chmod +x /usr/local/bin/healthcheck.sh /usr/local/bin/entrypoint.sh /usr/local/bin/post-start.sh

# Healthcheck
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/healthcheck.sh"]

# Expose port
EXPOSE 80

# Use custom entrypoint
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
