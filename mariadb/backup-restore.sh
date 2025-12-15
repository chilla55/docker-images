#!/bin/bash
set -e

BACKUP_DIR="/backups"
MYSQL_ROOT_PASSWORD=$(cat /run/secrets/mysql_root_password 2>/dev/null || echo "${MYSQL_ROOT_PASSWORD}")

echo "==================================="
echo "MariaDB Backup Restore Utility"
echo "==================================="

# Function to list databases in a backup file
list_databases_in_backup() {
    local BACKUP_FILE=$1
    echo ""
    echo "Databases in backup:"
    bunzip2 -c "${BACKUP_FILE}" | grep -E "^(CREATE DATABASE|USE) " | grep -oP '`\K[^`]+' | sort -u
}

# Function to restore specific database from backup
restore_database() {
    local BACKUP_FILE=$1
    local DATABASE_NAME=$2
    
    if [ ! -f "${BACKUP_FILE}" ]; then
        echo "ERROR: Backup file not found: ${BACKUP_FILE}"
        exit 1
    fi
    
    if [ -z "${DATABASE_NAME}" ]; then
        echo "ERROR: Database name required"
        exit 1
    fi
    
    echo ""
    echo "Restoring database '${DATABASE_NAME}' from: ${BACKUP_FILE}"
    echo "WARNING: This will overwrite the existing database!"
    echo ""
    
    # Verify checksum if available
    if [ -f "${BACKUP_FILE}.sha256" ]; then
        echo "Verifying backup integrity..."
        if sha256sum -c "${BACKUP_FILE}.sha256"; then
            echo "Checksum verified successfully"
        else
            echo "ERROR: Checksum verification failed!"
            exit 1
        fi
    fi
    
    # Extract and restore only the specified database
    echo "Extracting database '${DATABASE_NAME}'..."
    bunzip2 -c "${BACKUP_FILE}" | sed -n "/^-- Current Database: \`${DATABASE_NAME}\`/,/^-- Current Database:/p" | head -n -1 | mysql -uroot -p"${MYSQL_ROOT_PASSWORD}"
    
    # If sed pattern doesn't work, try alternative extraction
    if [ $? -ne 0 ]; then
        echo "Trying alternative extraction method..."
        bunzip2 -c "${BACKUP_FILE}" | mysql -uroot -p"${MYSQL_ROOT_PASSWORD}" --one-database "${DATABASE_NAME}"
    fi
    
    echo "Database '${DATABASE_NAME}' restored successfully!"
}

# Function to list available backups
list_backups() {
    echo ""
    echo "Available FULL backups:"
    if [ -d "${BACKUP_DIR}/full" ]; then
        ls -lh "${BACKUP_DIR}/full"/*_full.sql.bz2 2>/dev/null || echo "  No full backups found"
    else
        echo "  Backup directory not found"
    fi
    
    echo ""
    echo "Available INCREMENTAL backups:"
    if [ -d "${BACKUP_DIR}/incremental" ]; then
        ls -lh "${BACKUP_DIR}/incremental"/*_incremental.sql.bz2 2>/dev/null || echo "  No incremental backups found"
    else
        echo "  Backup directory not found"
    fi
}

# Function to restore from full backup
restore_full() {
    local BACKUP_FILE=$1
    
    if [ ! -f "${BACKUP_FILE}" ]; then
        echo "ERROR: Backup file not found: ${BACKUP_FILE}"
        exit 1
    fi
    
    echo ""
    echo "Restoring from full backup: ${BACKUP_FILE}"
    echo "WARNING: This will overwrite existing databases!"
    echo ""
    
    # Verify checksum if available
    if [ -f "${BACKUP_FILE}.sha256" ]; then
        echo "Verifying backup integrity..."
        if sha256sum -c "${BACKUP_FILE}.sha256"; then
            echo "Checksum verified successfully"
        else
            echo "ERROR: Checksum verification failed!"
            exit 1
        fi
    fi
    
    # Decompress and restore
    echo "Decompressing and restoring backup..."
    bunzip2 -c "${BACKUP_FILE}" | mysql -uroot -p"${MYSQL_ROOT_PASSWORD}"
    
    echo "Full backup restored successfully!"
}

# Function to restore incremental backup
restore_incremental() {
    local BACKUP_FILE=$1
    
    if [ ! -f "${BACKUP_FILE}" ]; then
        echo "ERROR: Backup file not found: ${BACKUP_FILE}"
        exit 1
    fi
    
    echo ""
    echo "Restoring from incremental backup: ${BACKUP_FILE}"
    
    # Verify checksum if available
    if [ -f "${BACKUP_FILE}.sha256" ]; then
        echo "Verifying backup integrity..."
        if sha256sum -c "${BACKUP_FILE}.sha256"; then
            echo "Checksum verified successfully"
        else
            echo "ERROR: Checksum verification failed!"
            exit 1
        fi
    fi
    
    # Decompress and restore
    echo "Decompressing and restoring backup..."
    bunzip2 -c "${BACKUP_FILE}" | mysql -uroot -p"${MYSQL_ROOT_PASSWORD}"
    
    echo "Incremental backup restored successfully!"
}

# Main script
case "${1}" in
    list)
        list_backups
        ;;
    restore-full)
        if [ -z "${2}" ]; then
            echo "Usage: $0 restore-full <backup_file>"
            echo "Example: $0 restore-full /backups/full/20231215_030000_full.sql.bz2"
            list_backups
            exit 1
        fi
        restore_full "${2}"
        ;;
    restore-incremental)
        if [ -z "${2}" ]; then
            echo "Usage: $0 restore-incremental <backup_file>"
            echo "Example: $0 restore-incremental /backups/incremental/20231215_120000_incremental.sql.bz2"
            list_backups
            exit 1
        fi
        restore_incremental "${2}"
        ;;
    restore-latest)
        echo "Restoring from latest full backup..."
        LATEST_FULL="${BACKUP_DIR}/full/latest_full.sql.bz2"
        if [ -L "${LATEST_FULL}" ]; then
            restore_full "${LATEST_FULL}"
        else
            echo "ERROR: No latest full backup found"
            exit 1
        fi
        
        echo ""
        echo "Applying latest incremental backup if available..."
        LATEST_INCREMENTAL="${BACKUP_DIR}/incremental/latest_incremental.sql.bz2"
        if [ -L "${LATEST_INCREMENTAL}" ]; then
            restore_incremental "${LATEST_INCREMENTAL}"
        else
            echo "No incremental backup found (this is OK if no changes were made)"
        fi
        
        echo ""
        echo "Restore complete!"
        ;;
    restore-database)
        if [ -z "${2}" ] || [ -z "${3}" ]; then
            echo "Usage: $0 restore-database <backup_file> <database_name>"
            echo "Example: $0 restore-database /backups/full/20231215_030000_full.sql.bz2 mydb"
            echo ""
            echo "To see databases in a backup:"
            echo "  $0 list-databases <backup_file>"
            exit 1
        fi
        restore_database "${2}" "${3}"
        ;;
    list-databases)
        if [ -z "${2}" ]; then
            echo "Usage: $0 list-databases <backup_file>"
            echo "Example: $0 list-databases /backups/full/20231215_030000_full.sql.bz2"
            exit 1
        fi
        list_databases_in_backup "${2}"
        ;;
    *)
        echo "Usage: $0 {list|restore-full|restore-incremental|restore-latest|restore-database|list-databases}"
        echo ""
        echo "Commands:"
        echo "  list                                    - List all available backups"
        echo "  restore-full <file>                     - Restore from a specific full backup"
        echo "  restore-incremental <file>              - Restore from a specific incremental backup"
        echo "  restore-latest                          - Restore from latest full + incremental backups"
        echo "  restore-database <file> <database>      - Restore a single database from backup"
        echo "  list-databases <file>                   - List databases in a backup file"
        echo ""
        echo "Examples:"
        echo "  $0 list"
        echo "  $0 restore-latest"
        echo "  $0 restore-full /backups/full/20231215_030000_full.sql.bz2"
        echo "  $0 list-databases /backups/full/20231215_030000_full.sql.bz2"
        echo "  $0 restore-database /backups/full/20231215_030000_full.sql.bz2 mydb"
        exit 1
        ;;
esac
