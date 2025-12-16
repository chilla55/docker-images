FROM alpine:3.19

LABEL maintainer="chilla55"
LABEL description="PostgreSQL single-node with intelligent automated backups"

# Install PostgreSQL and backup tools
RUN apk add --no-cache \
    postgresql16 \
    postgresql16-client \
    bash \
    dcron \
    bzip2 \
    tzdata \
    su-exec \
    coreutils \
    && mkdir -p /run/postgresql \
    && chown -R postgres:postgres /run/postgresql \
    && mkdir -p /var/lib/postgresql/data \
    && chown -R postgres:postgres /var/lib/postgresql

# Copy backup scripts
COPY backup-entrypoint.sh /usr/local/bin/
COPY backup-full.sh /usr/local/bin/
COPY backup-differential.sh /usr/local/bin/
COPY backup-incremental.sh /usr/local/bin/
COPY backup-restore.sh /usr/local/bin/

RUN chmod +x /usr/local/bin/*.sh

# Create backup state directory
RUN mkdir -p /var/lib/postgresql-backup-state

# Expose PostgreSQL port
EXPOSE 5432

VOLUME ["/var/lib/postgresql/data"]

ENTRYPOINT ["/usr/local/bin/backup-entrypoint.sh"]
CMD ["postgres"]

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
    CMD pg_isready -h localhost -U postgres -d postgres
