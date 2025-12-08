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
COPY postgresql-primary.conf /etc/postgresql/
COPY postgresql-replica.conf /etc/postgresql/
COPY pg_hba.conf /etc/postgresql/

RUN chmod +x /usr/local/bin/*.sh

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD /usr/local/bin/healthcheck.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["postgres"]
