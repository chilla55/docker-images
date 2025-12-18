# -----------------------------------------------
# Pure Go Reverse Proxy with HTTP/3 Support
# Replaces nginx entirely with Go-based proxy
# Includes SQLite database for metrics and logging
# -----------------------------------------------

FROM golang:1.21-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata gcc musl-dev

# Copy go module files first for better caching
COPY proxy-manager/go.mod proxy-manager/go.sum ./
RUN go mod download

# Copy source code
COPY proxy-manager/ ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o proxy-manager .

# ------------ Runtime Stage ------------
FROM alpine:3.20

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    curl \
    tzdata \
    sqlite \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 proxy && \
    adduser -D -u 1000 -G proxy proxy

# Create directories
RUN mkdir -p \
    /etc/proxy/sites-available \
    /etc/proxy/certs \
    /var/log/proxy \
    /data \
    && chown -R proxy:proxy /data /var/log/proxy

# Copy binary from builder
COPY --from=builder /build/proxy-manager /usr/local/bin/proxy-manager

# Copy default configs
COPY global.yaml /etc/proxy/global.yaml.example

USER proxy

# Expose ports
EXPOSE 80 443 443/udp 81 8080

# Environment variables with defaults
ENV SITES_PATH=/etc/proxy/sites-available \
    GLOBAL_CONFIG=/etc/proxy/global.yaml \
    HTTP_ADDR=:80 \
    HTTPS_ADDR=:443 \
    REGISTRY_PORT=81 \
    HEALTH_PORT=8080 \
    DB_PATH=/data/proxy.db \
    LOG_LEVEL=info \
    LOG_FORMAT=json \
    SHUTDOWN_TIMEOUT=30s \
    TZ=UTC

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/usr/local/bin/proxy-manager"]
