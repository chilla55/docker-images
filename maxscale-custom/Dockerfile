# Lightweight MaxScale image with procps-ng installed and non-root execution
FROM mariadb/maxscale:latest

# Install procps-ng for pgrep/ps tooling used by Monit scripts
RUN dnf -y install procps-ng \
    && dnf clean all

# Run MaxScale as the dedicated user and bypass base entrypoint (rsyslog)
USER maxscale
ENTRYPOINT ["/usr/bin/maxscale"]
CMD ["-d"]
