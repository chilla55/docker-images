#!/bin/bash
# MariaDB healthcheck script

MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password 2>/dev/null || echo "${MYSQL_ROOT_PASSWORD}")

# Try mariadb client first (newer), fall back to mysql
# Check common socket locations
if [ -S /run/mysqld/mysqld.sock ]; then
    SOCKET="/run/mysqld/mysqld.sock"
elif [ -S /var/run/mysqld/mysqld.sock ]; then
    SOCKET="/var/run/mysqld/mysqld.sock"
elif [ -S /tmp/mysql.sock ]; then
    SOCKET="/tmp/mysql.sock"
else
    SOCKET=""
fi

if command -v mariadb &>/dev/null; then
    if [ -n "$SOCKET" ]; then
        mariadb --socket="$SOCKET" -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &>/dev/null
    else
        mariadb -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &>/dev/null
    fi
elif command -v mysql &>/dev/null; then
    if [ -n "$SOCKET" ]; then
        mysql --socket="$SOCKET" -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &>/dev/null
    else
        mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" -e "SELECT 1" &>/dev/null
    fi
else
    exit 1
fi

exit $?
