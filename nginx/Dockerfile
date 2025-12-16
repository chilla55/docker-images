# -----------------------------------------------
# Custom NGINX 1.29 with Brotli, HTTP/3 (Alpine based)
# Multistage Build for Minimal Image Size
# -----------------------------------------------

# ------------ Stage 1: Builder ------------
FROM alpine:3.20 AS builder

ARG NGINX_VERSION=1.29.0
ENV NGINX_VERSION=${NGINX_VERSION}

RUN apk add --no-cache \
    build-base \
    brotli-dev \
    openssl-dev \
    pcre-dev \
    zlib-dev \
    curl \
    git \
    cmake \
    unzip \
    linux-headers

WORKDIR /usr/src

RUN git clone --recursive https://github.com/google/ngx_brotli.git && \
    git clone https://github.com/openresty/headers-more-nginx-module.git && \
    git clone https://github.com/FRiCKLE/ngx_cache_purge.git && \
    cd ngx_brotli && git submodule update --init --recursive

RUN curl -sSL http://nginx.org/download/nginx-$NGINX_VERSION.tar.gz | tar zxv

WORKDIR /usr/src/nginx-$NGINX_VERSION

RUN ./configure \
    --prefix=/etc/nginx \
    --sbin-path=/usr/sbin/nginx \
    --conf-path=/etc/nginx/nginx.conf \
    --modules-path=/etc/nginx/modules \
    --with-http_ssl_module \
    --with-http_realip_module \
    --with-http_v2_module \
    --with-http_v3_module \
    --with-threads \
    --with-compat \
    --with-file-aio \
    --with-http_stub_status_module \
    --with-http_auth_request_module \
    --add-module=../ngx_brotli \
    --add-module=../headers-more-nginx-module \
    --add-module=../ngx_cache_purge \
 && make -j$(nproc) && make install

# Clean up unnecessary files
RUN rm -rf /usr/src/* /root/.cache /var/cache/apk/* && \
    strip /usr/sbin/nginx

# ------------ Stage 2: Runtime ------------
FROM alpine:3.20

# tools for updater/healthcheck
RUN apk add --no-cache brotli openssl pcre zlib curl jq bash su-exec

COPY --from=builder /usr/sbin/nginx /usr/sbin/nginx
COPY --from=builder /etc/nginx /etc/nginx

# our scripts
COPY update-cf-ips.sh /usr/local/bin/update-cf-ips.sh
COPY healthcheck.sh   /usr/local/bin/healthcheck.sh
COPY entrypoint.sh    /entrypoint.sh
COPY watch-cert-reload.sh   /usr/local/bin/watch-cert-reload.sh
COPY watch-sites-reload.sh  /usr/local/bin/watch-sites-reload.sh
COPY default-site.conf /etc/nginx/conf.d/00-default.conf
RUN chmod +x /usr/local/bin/update-cf-ips.sh /usr/local/bin/healthcheck.sh /usr/local/bin/watch-cert-reload.sh /usr/local/bin/watch-sites-reload.sh /entrypoint.sh

# usual dirs/users (keep yours as-is)
RUN mkdir -p \
    /etc/nginx/modules \
    /var/cache/nginx \
    /var/cache/nginx/client_body_temp \
    /var/cache/nginx/proxy_temp \
    /var/cache/nginx/fastcgi_temp \
    /var/cache/nginx/uwsgi_temp \
    /var/cache/nginx/scgi_temp \
    /etc/nginx/conf.d \
    /etc/nginx/sites-enabled \
    /etc/nginx/logs \
    /etc/nginx/certs \
    /var/www && \
    adduser -D -g 'nginx' nginx && \
    chown -R nginx:nginx /etc/nginx /var/cache/nginx /var/www && \
    chmod -R 755 /etc/nginx/logs
# healthcheck
HEALTHCHECK --interval=5m --timeout=10s --retries=3 \
  CMD /usr/local/bin/healthcheck.sh || exit 1

# Need to run as root for CIFS mount, then drop to nginx user
USER root
ENTRYPOINT ["/entrypoint.sh"]