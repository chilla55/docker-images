#!/bin/bash

# Check if MariaDB is responding
if mysqladmin ping -h localhost -u root -p"$(cat /run/secrets/mysql_root_password)" --silent 2>/dev/null; then
    exit 0
else
    exit 1
fi
