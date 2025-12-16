#!/bin/bash
# MariaDB healthcheck script

MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password 2>/dev/null || echo "${MYSQL_ROOT_PASSWORD}")

# Try using mariadb-admin ping (fastest and most reliable)
if command -v mariadb-admin &>/dev/null; then
    mariadb-admin -uroot -p"${MYSQL_ROOT_PASSWORD}" ping &>/dev/null
    exit $?
elif command -v mysqladmin &>/dev/null; then
    mysqladmin -uroot -p"${MYSQL_ROOT_PASSWORD}" ping &>/dev/null
    exit $?
fi

# Fallback to SELECT query
if command -v mariadb &>/dev/null; then
    mariadb -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &>/dev/null
elif command -v mysql &>/dev/null; then
    mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &>/dev/null
else
    exit 1
fi

exit $?
