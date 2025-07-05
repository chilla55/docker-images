FROM debian:bookworm-slim

WORKDIR /var/www/pterodactyl

# Install dependencies
RUN apt-get update && apt-get install -y \
    curl gnupg2 lsb-release ca-certificates apt-transport-https software-properties-common unzip git sudo tar \
    && curl -fsSL https://packages.sury.org/php/apt.gpg | gpg --dearmor -o /etc/apt/trusted.gpg.d/php.gpg \
    && echo "deb https://packages.sury.org/php/ $(lsb_release -sc) main" > /etc/apt/sources.list.d/php.list \
    && apt-get update && apt-get install -y \
    php8.3 php8.3-fpm php8.3-cli php8.3-mysql php8.3-mbstring php8.3-bcmath php8.3-gd \
    php8.3-zip php8.3-curl php8.3-xml php8.3-tokenizer \
    mariadb-client redis \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Install Composer
RUN curl -sS https://getcomposer.org/installer | php && \
    mv composer.phar /usr/local/bin/composer

# Add healthcheck script
COPY healthcheck.sh /usr/local/bin/healthcheck.sh
RUN chmod +x /usr/local/bin/healthcheck.sh

# Define healthcheck
HEALTHCHECK --interval=10s --timeout=5s --retries=5 \
  CMD /usr/local/bin/healthcheck.sh

# Copy entrypoint script
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["php-fpm8.3", "-F"]
