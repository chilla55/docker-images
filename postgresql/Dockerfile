FROM postgres:16

LABEL maintainer="chilla55"
LABEL description="PostgreSQL with Master-Slave Replication support"

# Install necessary packages
RUN apt-get update && \
    apt-get install -y \
    curl \
    netcat-openbsd \
    && rm -rf /var/lib/apt/lists/*

# Copy configuration files
COPY scripts/ /usr/local/bin/
COPY scripts/init-replication-user.sh /docker-entrypoint-initdb.d/
COPY postgresql-primary.conf /etc/postgresql/
COPY postgresql-replica.conf /etc/postgresql/
COPY pg_hba.conf /etc/postgresql/

RUN chmod +x /usr/local/bin/*.sh /docker-entrypoint-initdb.d/*.sh

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD /usr/local/bin/healthcheck.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["postgres"]
