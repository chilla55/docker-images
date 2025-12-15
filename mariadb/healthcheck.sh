#!/bin/bash
# MariaDB healthcheck script

MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password 2>/dev/null || echo "${MYSQL_ROOT_PASSWORD}")

# Check if MariaDB is responding
if mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &>/dev/null; then
    exit 0
else
    exit 1
fi
