FROM alpine:3.19

LABEL maintainer="chilla55"
LABEL description="MariaDB single-node with intelligent automated backups"

# Install MariaDB and backup tools
RUN apk add --no-cache \
    mariadb \
    mariadb-client \
    bash \
    dcron \
    bzip2 \
    tzdata \
    && mkdir -p /run/mysqld \
    && chown -R mysql:mysql /run/mysqld \
    && mkdir -p /var/lib/mysql \
    && chown -R mysql:mysql /var/lib/mysql

# Copy MariaDB configuration
COPY mariadb.cnf /etc/my.cnf.d/custom.cnf

# Copy backup scripts
COPY backup-entrypoint.sh /usr/local/bin/
COPY backup-full.sh /usr/local/bin/
COPY backup-incremental.sh /usr/local/bin/
COPY backup-restore.sh /usr/local/bin/
COPY healthcheck.sh /usr/local/bin/

RUN chmod +x /usr/local/bin/*.sh

# Create backup state directory
RUN mkdir -p /var/lib/mysql-backup-state

# Expose MariaDB port
EXPOSE 3306

VOLUME ["/var/lib/mysql"]

ENTRYPOINT ["/usr/local/bin/backup-entrypoint.sh"]
CMD ["mariadbd", "--user=mysql", "--console"]

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
    CMD /usr/local/bin/healthcheck.sh
