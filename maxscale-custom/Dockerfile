# Lightweight MaxScale image with procps-ng installed and non-root execution
FROM mariadb/maxscale:latest

# Install procps-ng for pgrep/ps tooling used by Monit scripts
RUN dnf -y install procps-ng \
    && dnf clean all

# Create entrypoint script to substitute secrets before starting MaxScale
RUN mkdir -p /usr/local/bin
COPY entrypoint.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/entrypoint.sh

# Run entrypoint as root (it will drop to maxscale user)
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["-d"]
