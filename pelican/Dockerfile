# ============================================================================
# Multi-stage build for optimized Pelican Panel Docker image
# ============================================================================
# Designed for 4-service architecture:
# - php-fpm: PHP FastCGI Process Manager
# - caddy: HTTP server
# - queue: Laravel queue workers
# - cron: Laravel scheduler
# ============================================================================

# ──────────────────────────────────────────────────────────────────────────
# Stage 1: Go Builder - Build entrypoint binary
# ──────────────────────────────────────────────────────────────────────────
FROM golang:1.21-alpine AS go-builder

WORKDIR /build

# Copy go module files first for better caching
COPY go.mod go.sum* ./

# Download dependencies (including registry-client-v2 from GitHub)
RUN go mod download

# Copy source files
COPY main.go ./

# Build the entrypoint
RUN go build -o entrypoint .

# ──────────────────────────────────────────────────────────────────────────
# Stage 2: Runtime - Panel downloaded at first container start into volume
# ──────────────────────────────────────────────────────────────────────────
FROM alpine:edge

ARG PANEL_VERSION
ENV APP_DIR=/var/www/pelican \
    PANEL_VERSION=${PANEL_VERSION}

LABEL maintainer="chilla55" \
      org.opencontainers.image.title="Pelican Panel" \
      org.opencontainers.image.description="Pelican Panel for 4-service architecture (php-fpm, caddy, queue, cron)" \
      org.opencontainers.image.version="${PANEL_VERSION}" \
      org.opencontainers.image.source="https://github.com/chilla55/docker-images"

# Ensure edge repositories are available (main/community/testing)
RUN printf '%s\n' \
    "https://dl-cdn.alpinelinux.org/alpine/edge/main" \
    "https://dl-cdn.alpinelinux.org/alpine/edge/community" \
    "https://dl-cdn.alpinelinux.org/alpine/edge/testing" \
    > /etc/apk/repositories

# Install runtime dependencies + Composer + download tools
RUN apk add --no-cache \
    bash \
    curl \
    ca-certificates \
    tzdata \
    netcat-openbsd \
    tar \
    unzip \
    git \
    php85 \
    php85-fpm \
    php85-phar \
    php85-iconv \
    php85-session \
    php85-pdo \
    php85-pdo_mysql \
    php85-curl \
    php85-mbstring \
    php85-bcmath \
    php85-gd \
    php85-zip \
    php85-xml \
    php85-dom \
    php85-xmlwriter \
    php85-xmlreader \
    php85-simplexml \
    php85-tokenizer \
    php85-fileinfo \
    php85-openssl \
    php85-ctype \
    php85-posix \
    php85-sodium \
    php85-intl \
    php85-sqlite3 \
    php85-pdo_sqlite \
    caddy \
    mariadb-client && \
    ln -sf /usr/bin/php85 /usr/bin/php && \
    ln -sf /usr/sbin/php-fpm85 /usr/sbin/php-fpm && \
    rm -f /etc/php85/php-fpm.d/www.conf

# Install Composer globally
RUN curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/local/bin --filename=composer

# Create nginx user and directories
RUN addgroup -g 1001 -S nginx && \
    adduser -u 1001 -D -S -G nginx nginx && \
    mkdir -p \
        ${APP_DIR} \
        /var/log/php-fpm \
        /run/php-fpm && \
    chown -R nginx:nginx /var/log/php-fpm /run/php-fpm

WORKDIR ${APP_DIR}

# Copy Go entrypoint binary from go-builder
COPY --from=go-builder /build/entrypoint /entrypoint
RUN chmod +x /entrypoint

# Panel files are downloaded at first container start into the mounted volume
# Permissions on storage/bootstrap/cache are set by the entrypoint after install
COPY --chown=root:root healthcheck.sh /healthcheck.sh
RUN chmod +x /healthcheck.sh

# Expose ports (php-fpm: 9000, caddy: 80)
EXPOSE 80 9000

# Healthcheck — start-period is long to allow first-time panel download + composer install
HEALTHCHECK --interval=30s --timeout=5s --start-period=300s --retries=3 \
    CMD /healthcheck.sh

# Run as root (services will drop privileges as needed)
USER root

ENTRYPOINT ["/entrypoint"]
# Default command can be overridden by docker-compose
CMD ["php-fpm"]
