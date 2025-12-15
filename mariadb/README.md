# MariaDB Docker Swarm with Intelligent Backups

A production-ready MariaDB Docker container for Docker Swarm with intelligent automated backup system.

## Features

- **MariaDB 11.2** - Latest stable release
- **Intelligent Backup System**:
  - Weekly full backups (every Sunday at 3 AM)
  - Hourly incremental backups (only when changes detected)
  - Automatic change detection (skips backup if no changes)
  - 30-day retention policy with automatic cleanup
  - Compressed backups with checksums
- **Docker Swarm Ready** - Full orchestration support
- **Health Checks** - Automatic health monitoring
- **Resource Management** - CPU and memory limits
- **Storage Box Integration** - Persistent backup storage

## Backup Strategy

### Full Backups
- **Schedule**: Weekly (Sunday 3:00 AM)
- **Content**: Complete dump of all databases
- **Format**: Compressed SQL dump (bzip2)
- **Location**: `/backups/full/`

### Incremental Backups
- **Schedule**: Hourly (every hour)
- **Intelligence**: Only runs if database has changes
- **Detection**: Monitors database size, update times, and table counts
- **Content**: Changed databases only
- **Format**: Compressed SQL dump (bzip2)
- **Location**: `/backups/incremental/`

### Change Detection Logic
The incremental backup script:
1. Calculates current database state (size + update time + table count)
2. Compares with last backup state
3. Skips backup if no changes detected
4. Only backs up databases with recent modifications
5. Updates state tracking after successful backup

### Retention Policy
- Automatically deletes backups older than 30 days
- Keeps symbolic links to latest backups
- Maintains both full and incremental backup history

## Quick Start

### 1. Setup Prerequisites

```bash
# Create MariaDB network
docker network create -d overlay mariadb-net

# Create root password secret
echo "your_secure_password" | docker secret create mysql_root_password -

# Setup storage box mount (optional but recommended)
mkdir -p /mnt/storagebox/mariadb-backups

# Label the node for placement
docker node update --label-add mariadb.node=srv1 srv1
```

### 2. Build and Deploy

```bash
cd mariadb/

# Build image
make build VERSION=1.0.0

# Push to registry
make push VERSION=1.0.0

# Deploy to swarm
make deploy
```

### 3. Verify Deployment

```bash
# Check service status
make ps

# View logs
make logs

# Check health
docker service ps mariadb_mariadb
```

## Backup Management

### Manual Backups

```bash
# Trigger immediate full backup
make backup-full

# Trigger immediate incremental backup
make backup-incr

# List all available backups
make backup-list
```

### Restore from Backup

```bash
# Restore from latest full + incremental backup
make backup-restore

# Or manually inside container
docker exec -it <container_id> /usr/local/bin/backup-restore.sh restore-latest
```

### Custom Restore

```bash
# List backups
docker exec <container_id> /usr/local/bin/backup-restore.sh list

# Restore specific full backup
docker exec <container_id> /usr/local/bin/backup-restore.sh restore-full \
    /backups/full/20231215_030000_full.sql.bz2

# Restore specific incremental backup
docker exec <container_id> /usr/local/bin/backup-restore.sh restore-incremental \
    /backups/incremental/20231215_120000_incremental.sql.bz2
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `BACKUP_ENABLED` | `true` | Enable/disable backup system |
| `BACKUP_FULL_SCHEDULE` | `0 3 * * 0` | Cron schedule for full backups |
| `BACKUP_INCREMENTAL_SCHEDULE` | `0 * * * *` | Cron schedule for incremental backups |
| `BACKUP_RETENTION_DAYS` | `30` | Days to keep backups |
| `MYSQL_ROOT_PASSWORD_FILE` | - | Path to root password secret file |

### Custom Backup Schedule

Edit `docker-compose.swarm.yml`:

```yaml
environment:
  # Daily full backups at 2 AM
  BACKUP_FULL_SCHEDULE: "0 2 * * *"
  
  # Every 30 minutes incremental
  BACKUP_INCREMENTAL_SCHEDULE: "*/30 * * * *"
  
  # Keep for 90 days
  BACKUP_RETENTION_DAYS: "90"
```

## Monitoring

### Check Backup Logs

```bash
# Full backup logs
docker exec <container_id> tail -f /var/log/mysql/backup-full.log

# Incremental backup logs
docker exec <container_id> tail -f /var/log/mysql/backup-incremental.log
```

### Backup Status

```bash
# View backup state
docker exec <container_id> cat /var/lib/mysql-backup-state/last_incremental_state

# Check last backup times
docker exec <container_id> ls -lah /backups/full/
docker exec <container_id> ls -lah /backups/incremental/
```

## Storage Requirements

### Backup Storage Planning
- **Full backup**: ~100MB - 10GB+ (depends on database size)
- **Incremental backup**: ~1MB - 1GB (only changed data)
- **Hourly incremental**: 24 backups/day × 30 days = 720 backups max
- **Weekly full**: 4-5 backups/month kept

**Example**: 1GB database, 50MB daily changes
- Full backups: 5 × 1GB = 5GB
- Incremental: 720 × 50MB = 36GB
- **Total**: ~41GB for 30-day retention

### Volume Configuration

```yaml
volumes:
  - mariadb-data:/var/lib/mysql              # Database data
  - mariadb-logs:/var/log/mysql              # Logs
  - /mnt/storagebox/mariadb-backups:/backups:rslave  # Backups
```

## Performance Tuning

The MariaDB configuration (`mariadb.cnf`) includes:
- 2GB InnoDB buffer pool
- Binary logging for point-in-time recovery
- Slow query logging
- Optimized for SSD storage

Adjust resources in `docker-compose.swarm.yml`:

```yaml
resources:
  limits:
    cpus: '4'
    memory: 4G
  reservations:
    cpus: '2'
    memory: 2G
```

## Security

- Root password stored in Docker secrets
- No password in environment variables
- Compressed backups with SHA256 checksums
- Isolated overlay network
- Health checks for automatic recovery

## Troubleshooting

### Backups Not Running

```bash
# Check cron is running
docker exec <container_id> service cron status

# Verify cron configuration
docker exec <container_id> cat /etc/cron.d/mariadb-backup

# Check backup logs
docker exec <container_id> tail -50 /var/log/mysql/backup-full.log
docker exec <container_id> tail -50 /var/log/mysql/backup-incremental.log
```

### No Incremental Backups Created

This is **normal** if:
- No changes were made to the database
- The intelligent detection skipped the backup

Check logs for "No changes detected" messages.

### Container Won't Start

```bash
# Check logs
docker service logs mariadb_mariadb

# Verify secret exists
docker secret ls | grep mysql_root_password

# Check node labels
docker node inspect srv1 | grep mariadb
```

## Makefile Commands

```bash
make help              # Show all commands
make build             # Build Docker image
make push              # Push to registry
make deploy            # Deploy to swarm
make update            # Update running service
make remove            # Remove stack
make logs              # View service logs
make ps                # Show service status
make backup-full       # Manual full backup
make backup-incr       # Manual incremental backup
make backup-list       # List backups
make backup-restore    # Restore from backup
make shell             # Open shell in container
make clean             # Remove local images
```

## Architecture

```
┌─────────────────────────────────────────┐
│         MariaDB Container               │
├─────────────────────────────────────────┤
│  ┌──────────────┐  ┌─────────────────┐ │
│  │   MariaDB    │  │  Cron Daemon    │ │
│  │   11.2       │  │  - Full Backup  │ │
│  │              │  │  - Incremental  │ │
│  └──────────────┘  └─────────────────┘ │
│         │                   │           │
│  ┌──────┴───────────────────┴────────┐ │
│  │      Backup Scripts               │ │
│  │  - Change Detection               │ │
│  │  - Compression & Checksum         │ │
│  │  - Retention Management           │ │
│  └───────────────────────────────────┘ │
└─────────────────────────────────────────┘
           │                  │
    ┌──────┴────┐      ┌─────┴──────┐
    │ Data Vol  │      │ Backup Vol │
    │  (Local)  │      │ (Storage)  │
    └───────────┘      └────────────┘
```

## License

Copyright (c) 2025 chilla55

## Support

For issues or questions, check the logs first:
```bash
make logs
```

Then review backup logs inside the container.
