FROM golang:1.21-alpine AS go-builder

WORKDIR /build
COPY main.go registry_client_v2.go go.mod ./
RUN go build -o entrypoint .

FROM vaultwarden/server:latest

# Install healthcheck dependencies
RUN apt-get update && apt-get install -y wget && rm -rf /var/lib/apt/lists/*

# Add scripts
COPY healthcheck.sh /usr/local/bin/healthcheck.sh
COPY post-start.sh /usr/local/bin/post-start.sh
RUN chmod +x /usr/local/bin/healthcheck.sh /usr/local/bin/post-start.sh

# Copy Go entrypoint from builder
COPY --from=go-builder /build/entrypoint /usr/local/bin/entrypoint
RUN chmod +x /usr/local/bin/entrypoint

# Healthcheck
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/healthcheck.sh"]

# Expose port
EXPOSE 80

# Use Go entrypoint
ENTRYPOINT ["/usr/local/bin/entrypoint"]

