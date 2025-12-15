FROM mariadb:11.7

# Install backup tools and dependencies
RUN apt-get update && apt-get install -y \
    mariadb-backup \
    cron \
    rsync \
    openssh-client \
    sshfs \
    && rm -rf /var/lib/apt/lists/*

# Copy backup scripts
COPY backup-entrypoint.sh /usr/local/bin/
COPY backup-full.sh /usr/local/bin/
COPY backup-incremental.sh /usr/local/bin/
COPY backup-restore.sh /usr/local/bin/
COPY healthcheck.sh /usr/local/bin/

RUN chmod +x /usr/local/bin/backup-*.sh /usr/local/bin/healthcheck.sh

# MariaDB configuration optimized for single-node with backups
COPY mariadb-single.cnf /etc/mysql/conf.d/

ENTRYPOINT ["/usr/local/bin/backup-entrypoint.sh"]

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
    CMD /usr/local/bin/healthcheck.sh
