# ============================================================================
# Multi-stage build for optimized Pterodactyl Panel Docker image
# ============================================================================

# ──────────────────────────────────────────────────────────────────────────
# Stage 1: Builder - Install Composer dependencies
# ──────────────────────────────────────────────────────────────────────────
FROM alpine:3.20 AS builder

ARG PANEL_VERSION

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache \
    curl \
    ca-certificates \
    tar \
    php83 \
    php83-phar \
    php83-iconv \
    php83-openssl \
    php83-mbstring \
    php83-tokenizer \
    php83-dom \
    php83-xml \
    php83-xmlwriter \
    php83-simplexml \
    php83-curl \
    php83-zip \
    php83-fileinfo \
    php83-ctype

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
RUN composer install \
    --no-dev \
    --optimize-autoloader \
    --no-interaction \
    --no-progress \
    --prefer-dist && \
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
# Stage 2: Runtime - Final optimized image
# ──────────────────────────────────────────────────────────────────────────
FROM alpine:3.20

ARG PANEL_VERSION
ENV APP_DIR=/var/www/pterodactyl \
    PANEL_VERSION=${PANEL_VERSION}

LABEL maintainer="chilla55" \
      org.opencontainers.image.title="Pterodactyl Panel" \
      org.opencontainers.image.description="All-in-one Pterodactyl Panel container with Caddy, PHP-FPM, Queue Worker, and Scheduler" \
      org.opencontainers.image.version="${PANEL_VERSION}" \
      org.opencontainers.image.source="https://github.com/chilla55/docker-images"

# Install runtime dependencies only
RUN apk add --no-cache \
    bash \
    curl \
    ca-certificates \
    tzdata \
    # PHP 8.3 runtime
    php83 \
    php83-fpm \
    php83-opcache \
    php83-session \
    php83-pdo \
    php83-pdo_mysql \
    php83-mysqlnd \
    php83-curl \
    php83-mbstring \
    php83-bcmath \
    php83-gd \
    php83-zip \
    php83-xml \
    php83-dom \
    php83-xmlwriter \
    php83-simplexml \
    php83-tokenizer \
    php83-fileinfo \
    php83-openssl \
    php83-ctype \
    php83-json \
    php83-posix \
    php83-pcntl \
    # Web server
    caddy \
    # Process manager
    supervisor \
    # Database client for healthchecks
    mariadb-client \
    mariadb-connector-c && \
    # Create symlinks
    ln -sf /usr/bin/php83 /usr/bin/php && \
    ln -sf /usr/sbin/php-fpm83 /usr/sbin/php-fpm && \
    # Create www-data user (matching nginx UID for consistency)
    addgroup -g 82 -S www-data && \
    adduser -u 82 -D -S -G www-data www-data && \
    # Create necessary directories
    mkdir -p \
        ${APP_DIR} \
        /var/log/ptero \
        /run/php && \
    chown -R www-data:www-data /var/log/ptero /run/php

WORKDIR ${APP_DIR}

# Copy application from builder
COPY --from=builder --chown=www-data:www-data /build ${APP_DIR}

# Copy configuration files
COPY --chown=root:root Caddyfile /etc/caddy/Caddyfile
COPY --chown=root:root supervisord.conf /etc/supervisord.conf
COPY --chown=root:root entrypoint.sh /entrypoint.sh
COPY --chown=root:root healthcheck.sh /healthcheck.sh

# Make scripts executable
RUN chmod +x /entrypoint.sh /healthcheck.sh

# Configure PHP-FPM for production
RUN sed -i \
    -e 's/^;daemonize = yes/daemonize = no/' \
    -e 's/^;error_log = log\/php83\/error.log/error_log = \/proc\/self\/fd\/2/' \
    -e 's/^;log_level = notice/log_level = warning/' \
    /etc/php83/php-fpm.conf && \
    sed -i \
    -e 's/^user = nobody/user = www-data/' \
    -e 's/^group = nobody/group = www-data/' \
    -e 's/^listen = 127.0.0.1:9000/listen = 0.0.0.0:9000/' \
    -e 's/^;listen.owner = nobody/listen.owner = www-data/' \
    -e 's/^;listen.group = nobody/listen.group = www-data/' \
    -e 's/^;listen.mode = 0660/listen.mode = 0660/' \
    -e 's/^pm = dynamic/pm = ondemand/' \
    -e 's/^pm.max_children = 5/pm.max_children = 20/' \
    -e 's/^;pm.process_idle_timeout = 10s/pm.process_idle_timeout = 10s/' \
    -e 's/^;catch_workers_output = yes/catch_workers_output = yes/' \
    -e 's/^;decorate_workers_output = no/decorate_workers_output = no/' \
    /etc/php83/php-fpm.d/www.conf

# Configure PHP for production
RUN sed -i \
    -e 's/^expose_php = On/expose_php = Off/' \
    -e 's/^;date.timezone =/date.timezone = UTC/' \
    -e 's/^memory_limit = 128M/memory_limit = 256M/' \
    -e 's/^upload_max_filesize = 2M/upload_max_filesize = 100M/' \
    -e 's/^post_max_size = 8M/post_max_size = 100M/' \
    -e 's/^max_execution_time = 30/max_execution_time = 300/' \
    /etc/php83/php.ini

# Set proper permissions
RUN chown -R www-data:www-data \
    ${APP_DIR}/storage \
    ${APP_DIR}/bootstrap/cache && \
    chmod -R 755 ${APP_DIR}/storage ${APP_DIR}/bootstrap/cache

# Expose ports
EXPOSE 80 9000

# Healthcheck
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD /healthcheck.sh

# Security: Run as non-root for entrypoint (supervisord will handle user switching)
USER root

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf"]
