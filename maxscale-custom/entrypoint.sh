#!/bin/bash
set -e

# Copy config from temp location to /etc/maxscale.cnf
cp /tmp/maxscale.cnf /etc/maxscale.cnf

# Substitute secrets into it
if [ -f /run/secrets/mysql_root_password ]; then
    MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password)
    sed -i "s|\${MYSQL_ROOT_PASSWORD}|${MYSQL_ROOT_PASSWORD}|g" /etc/maxscale.cnf
fi

# Start MaxScale with the edited config
exec su -s /bin/bash maxscale -c "/usr/bin/maxscale $*"
