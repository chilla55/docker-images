# Lightweight MaxScale image with procps-ng installed and non-root execution
FROM mariadb/maxscale:latest

# Install procps-ng for pgrep/ps tooling used by Monit scripts
RUN dnf -y install procps-ng \
    && dnf clean all

# Run MaxScale as the dedicated user
USER maxscale

# Start MaxScale in daemon mode
CMD ["/usr/bin/maxscale", "-d"]
