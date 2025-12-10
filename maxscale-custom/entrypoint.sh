#!/bin/bash
set -e

# Substitute secrets into maxscale.cnf
if [ -f /run/secrets/mysql_root_password ]; then
    MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password)
    sed -i "s|\${MYSQL_ROOT_PASSWORD}|${MYSQL_ROOT_PASSWORD}|g" /etc/maxscale.cnf
fi

# Start MaxScale with passed arguments
exec /usr/bin/maxscale "$@"
