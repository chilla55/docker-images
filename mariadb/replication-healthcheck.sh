#!/bin/bash

# Check if MariaDB is running
if ! mysqladmin ping -h localhost --silent 2>/dev/null; then
    echo "MariaDB is not responding"
    exit 1
fi

# Check replication status if slave
if [ "${REPLICATION_MODE}" = "slave" ]; then
    SLAVE_STATUS=$(mysql -u root -p"${MYSQL_ROOT_PASSWORD}" -e "SHOW SLAVE STATUS\G" 2>/dev/null | grep "Slave_.*_Running" || true)
    
    if [ -z "$SLAVE_STATUS" ]; then
        echo "Replication not configured yet"
        exit 0  # Don't fail during initial setup
    fi
    
    IO_RUNNING=$(echo "$SLAVE_STATUS" | grep "Slave_IO_Running" | awk '{print $2}')
    SQL_RUNNING=$(echo "$SLAVE_STATUS" | grep "Slave_SQL_Running" | awk '{print $2}')
    
    if [ "$IO_RUNNING" != "Yes" ] || [ "$SQL_RUNNING" != "Yes" ]; then
        echo "Replication not running: IO=$IO_RUNNING, SQL=$SQL_RUNNING"
        exit 1
    fi
fi

echo "MariaDB is healthy"
exit 0
