# ============================================================================
# Multi-stage build for optimized Pterodactyl Panel Docker image
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
# Stage 2: PHP Builder - Install Composer dependencies
# ──────────────────────────────────────────────────────────────────────────
FROM alpine:3.20 AS php-builder

ARG PANEL_VERSION

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache \
    curl \
    ca-certificates \
    tar \
    unzip \
    git \
    php83 \
    php83-phar \
    php83-iconv \
    php83-openssl \
    php83-mbstring \
    php83-tokenizer \
    php83-dom \
    php83-xml \
    php83-xmlwriter \
    php83-xmlreader \
    php83-simplexml \
    php83-curl \
    php83-zip \
    php83-fileinfo \
    php83-ctype \
    php83-pdo \
    php83-pdo_mysql \
    php83-session \
    php83-posix \
    php83-sodium \
    php83-bcmath \
    php83-intl

# Create php symlink
RUN ln -sf /usr/bin/php83 /usr/bin/php

# Install Composer
RUN curl -sS https://getcomposer.org/installer | php -- --install-dir=/usr/local/bin --filename=composer

# Download and extract Pterodactyl Panel
RUN echo "[BUILD] Downloading Pterodactyl ${PANEL_VERSION}..." && \
    curl -fsSL "https://github.com/pterodactyl/panel/releases/download/${PANEL_VERSION}/panel.tar.gz" \
        -o /tmp/panel.tar.gz && \
    tar -xzf /tmp/panel.tar.gz -C /build && \
    rm /tmp/panel.tar.gz

# Install production dependencies
RUN COMPOSER_ALLOW_SUPERUSER=1 composer install \
    --no-dev \
    --optimize-autoloader \
    --no-interaction \
    --prefer-dist \
    --verbose && \
    composer clear-cache

# Remove unnecessary files
RUN rm -rf \
    .git \
    .github \
    .env.example \
    tests \
    storage/logs/*.log \
    bootstrap/cache/*.php

# ──────────────────────────────────────────────────────────────────────────
# Stage 3: Runtime - Final optimized image
# ──────────────────────────────────────────────────────────────────────────
FROM alpine:3.20

ARG PANEL_VERSION
ENV APP_DIR=/var/www/pterodactyl \
    PANEL_VERSION=${PANEL_VERSION}

LABEL maintainer="chilla55" \
      org.opencontainers.image.title="Pterodactyl Panel" \
      org.opencontainers.image.description="Pterodactyl Panel for 4-service architecture (php-fpm, caddy, queue, cron)" \
      org.opencontainers.image.version="${PANEL_VERSION}" \
      org.opencontainers.image.source="https://github.com/chilla55/docker-images"

# Enable community repository for Caddy
RUN echo "https://dl-cdn.alpinelinux.org/alpine/v3.20/community" >> /etc/apk/repositories

# Install runtime dependencies only
RUN apk add --no-cache \
    bash \
    curl \
    ca-certificates \
    tzdata \
    netcat-openbsd \
    php83 \
    php83-fpm \
    php83-opcache \
    php83-session \
    php83-pdo \
    php83-pdo_mysql \
    php83-curl \
    php83-mbstring \
    php83-bcmath \
    php83-gd \
    php83-zip \
    php83-xml \
    php83-dom \
    php83-xmlwriter \
    php83-xmlreader \
    php83-simplexml \
    php83-tokenizer \
    php83-fileinfo \
    php83-openssl \
    php83-ctype \
    php83-posix \
    php83-sodium \
    php83-intl \
    caddy \
    mariadb-client && \
    ln -sf /usr/bin/php83 /usr/bin/php && \
    ln -sf /usr/sbin/php-fpm83 /usr/sbin/php-fpm && \
    rm -f /etc/php83/php-fpm.d/www.conf

# Create nginx user and directories (use different GID since 101 is taken)
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

# Copy application from php-builder
COPY --from=php-builder --chown=nginx:nginx /build ${APP_DIR}

# Copy healthcheck script
COPY --chown=root:root healthcheck.sh /healthcheck.sh
RUN chmod +x /healthcheck.sh

# Set proper permissions
RUN chown -R nginx:nginx \
    ${APP_DIR}/storage \
    ${APP_DIR}/bootstrap/cache && \
    chmod -R 755 ${APP_DIR}/storage ${APP_DIR}/bootstrap/cache

# Expose ports (php-fpm: 9000, caddy: 80)
EXPOSE 80 9000

# Healthcheck
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD /healthcheck.sh

# Run as root (services will drop privileges as needed)
USER root

ENTRYPOINT ["/entrypoint"]
# Default command can be overridden by docker-compose
CMD ["php-fpm"]
