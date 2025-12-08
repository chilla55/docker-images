FROM redis:7-alpine

# Install healthcheck dependencies
RUN apk add --no-cache redis

# Copy custom configuration if needed
COPY redis.conf /usr/local/etc/redis/redis.conf
COPY entrypoint.sh /usr/local/bin/entrypoint.sh

# Create data directory
RUN mkdir -p /data && chown redis:redis /data && \
    chmod +x /usr/local/bin/entrypoint.sh

# Expose Redis port
# Healthcheck
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD redis-cli ping || exit 1

# Use custom entrypoint
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
# Use custom config
CMD ["redis-server", "/usr/local/etc/redis/redis.conf"]
