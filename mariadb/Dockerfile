FROM mariadb:11.2

LABEL maintainer="chilla55"
LABEL description="MariaDB with Master-Slave Replication support"

# Install necessary packages
RUN apt-get update && \
    apt-get install -y \
    curl \
    netcat-openbsd \
    && rm -rf /var/lib/apt/lists/*

# Copy configuration files
COPY replication-entrypoint.sh /usr/local/bin/
COPY replication-healthcheck.sh /usr/local/bin/
COPY scripts/check-connectivity.sh /usr/local/bin/
COPY mariadb-master.cnf /etc/mysql/conf.d/
COPY mariadb-slave.cnf /etc/mysql/conf.d/

RUN chmod +x /usr/local/bin/replication-entrypoint.sh && \
    chmod +x /usr/local/bin/replication-healthcheck.sh && \
    chmod +x /usr/local/bin/check-connectivity.sh

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD /usr/local/bin/replication-healthcheck.sh

ENTRYPOINT ["/usr/local/bin/replication-entrypoint.sh"]
CMD ["mariadbd"]
