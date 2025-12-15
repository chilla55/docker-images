FROM mariadb:11.2

LABEL maintainer="chilla55"
LABEL description="MariaDB single-node with intelligent automated backups"

# Install backup tools
RUN apt-get update && \
    apt-get install -y \
    cron \
    gzip \
    bzip2 \
    rsync \
    mysql-client \
    && rm -rf /var/lib/apt/lists/*

# Copy backup scripts
COPY backup-entrypoint.sh /usr/local/bin/
COPY backup-full.sh /usr/local/bin/
COPY backup-incremental.sh /usr/local/bin/
COPY backup-restore.sh /usr/local/bin/
COPY healthcheck.sh /usr/local/bin/

RUN chmod +x /usr/local/bin/*.sh

# Copy custom MariaDB configuration
COPY mariadb.cnf /etc/mysql/mariadb.conf.d/99-custom.cnf

# Create backup state directory
RUN mkdir -p /var/lib/mysql-backup-state

ENTRYPOINT ["/usr/local/bin/backup-entrypoint.sh"]
CMD ["mariadbd"]

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
    CMD /usr/local/bin/healthcheck.sh
