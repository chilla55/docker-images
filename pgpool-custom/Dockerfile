FROM alpine:3.20

LABEL maintainer="chilla55"
LABEL description="Pgpool-II Alpine image with env-driven config"

RUN apk add --no-cache pgpool openssl bash su-exec coreutils

# Directories
RUN mkdir -p /etc/pgpool2 /var/log/pgpool /var/run/pgpool /var/lib/pgpool && \
    chown -R root:root /etc/pgpool2 && \
    chown -R postgres:postgres /var/log/pgpool /var/run/pgpool /var/lib/pgpool

# Copy entrypoint and templates
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
COPY templates/pgpool.conf.tpl /etc/pgpool2/pgpool.conf.tpl
COPY templates/pool_hba.conf.tpl /etc/pgpool2/pool_hba.conf.tpl

RUN chmod +x /usr/local/bin/entrypoint.sh

EXPOSE 5432

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD pgpool -n -f /etc/pgpool2/pgpool.conf -m online -s -o -? >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["pgpool"]
