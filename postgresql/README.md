# PostgreSQL Docker Swarm Container

Complete PostgreSQL 16 setup for Docker Swarm with automated 3-tier backup strategy.

## Features

- **PostgreSQL 16** on Alpine Linux 3.19 (minimal footprint)
- **3-Tier Backup Strategy**:
  - Full backup: Weekly (Sunday 3AM)
  - Differential backup: Mid-week (Wednesday 3AM)  
  - Incremental backup: Hourly (with change detection)
- **Intelligent Change Detection**: Skips backups when no changes detected
- **Tiered Retention Policy**:
  - First 14 days: Full granularity (all backups)
  - 15-42 days: Differential + Full only
  - 42+ days: Full backups only (minimum 2 kept)
- **Auto-restore**: Automatically restores latest backup when database is empty
- **Single Database Restore**: Restore individual databases from full backups
- **Health Checks**: Built-in PostgreSQL health monitoring
- **Docker Swarm Native**: Secrets, configs, overlay networks

## Quick Start

### 1. Prerequisites

```bash
# Create backend network (if not exists)
docker network create --driver overlay --attachable backend-net

# Create backup directory on all nodes
mkdir -p /mnt/storagebox/backups/postgresql

# Label database nodes
docker node update --label-add database=true <node-name>
```

### 2. Create Secrets

```bash
# PostgreSQL password
echo "your_secure_password" | docker secret create postgres_password -
```

### 3. Create PostgreSQL Configuration

Create `postgresql.conf` content and add as Docker config:

```bash
docker config create postgresql-conf postgresql.conf
```

Example minimal `postgresql.conf`:
```
listen_addresses = '*'
max_connections = 100
shared_buffers = 256MB
effective_cache_size = 1GB
maintenance_work_mem = 64MB
checkpoint_completion_target = 0.9
wal_buffers = 16MB
default_statistics_target = 100
random_page_cost = 1.1
effective_io_concurrency = 200
work_mem = 2621kB
min_wal_size = 1GB
max_wal_size = 4GB
max_worker_processes = 2
max_parallel_workers_per_gather = 1
max_parallel_workers = 2
max_parallel_maintenance_workers = 1
```

### 4. Build & Deploy

```bash
# Build image
make build

# Push to registry
make push

# Deploy to Swarm
make deploy
```

## Backup Management

### Trigger Manual Backups

```bash
# Full backup
make backup-full

# Differential backup
make backup-diff

# Incremental backup
make backup-incr

# List available backups
make backup-list
```

### Restore from Backup

```bash
# Restore latest (full + incremental)
make backup-restore

# Or use restore script directly
docker exec <container> /usr/local/bin/backup-restore.sh list
docker exec <container> /usr/local/bin/backup-restore.sh restore-latest
docker exec <container> /usr/local/bin/backup-restore.sh restore-full /backups/full/20231215_030000_full.sql.bz2
```

### Single Database Restore

```bash
# List databases in a backup
docker exec <container> /usr/local/bin/backup-restore.sh list-databases /backups/full/20231215_030000_full.sql.bz2

# Restore single database
docker exec <container> /usr/local/bin/backup-restore.sh restore-database /backups/full/20231215_030000_full.sql.bz2 mydb
```

## Backup Strategy Details

### 3-Tier Backup System

1. **Full Backup** (Sunday 3AM)
   - Complete dump of all databases using `pg_dumpall`
   - Includes all users, roles, permissions
   - Creates baseline for differential/incremental backups
   - Retention: Minimum 2 full backups always kept

2. **Differential Backup** (Wednesday 3AM)
   - Dumps all databases if changes detected since last full backup
   - Smaller than full, faster than incremental
   - Change detection via database state checksum
   - Retention: 42 days

3. **Incremental Backup** (Hourly)
   - Per-database dumps using `pg_dump`
   - Only backs up databases with detected changes
   - Skips entirely if no changes across all databases
   - Auto-triggers full backup if full backup is missing
   - Retention: 14 days

### Change Detection

Backups are skipped intelligently when no changes are detected:

```sql
-- State tracked per database:
SELECT 
  datname,
  pg_database_size(datname),
  stats_reset,
  numbackends
FROM pg_stat_database
WHERE datname NOT IN ('template0', 'template1')
```

State checksum is compared before each backup. If unchanged, backup is skipped.

### Tiered Retention

The cleanup strategy preserves data efficiently:

- **0-14 days**: Keep everything (full granularity recovery)
- **15-42 days**: Keep differential + full only (removes incremental)
- **42+ days**: Keep full backups only (minimum 2 always preserved)

This provides:
- Recent data: Hourly recovery points
- Medium-term: Daily recovery points
- Long-term: Weekly recovery points

## Directory Structure

```
/backups/
├── full/               # Weekly full backups
│   ├── YYYYMMDD_HHMMSS_full.sql.bz2
│   ├── YYYYMMDD_HHMMSS_full.sql.bz2.sha256
│   └── latest_full.sql.bz2 -> (symlink)
├── differential/       # Mid-week differential backups
│   ├── YYYYMMDD_HHMMSS_differential.sql.bz2
│   ├── YYYYMMDD_HHMMSS_differential.sql.bz2.sha256
│   └── latest_differential.sql.bz2 -> (symlink)
└── incremental/        # Hourly incremental backups
    ├── YYYYMMDD_HHMMSS_incremental.sql.bz2
    ├── YYYYMMDD_HHMMSS_incremental.sql.bz2.sha256
    └── latest_incremental.sql.bz2 -> (symlink)
```

## Configuration

### Environment Variables

- `TZ`: Timezone (default: `Europe/Berlin`)
- `POSTGRES_PASSWORD_FILE`: Path to password secret (`/run/secrets/postgres_password`)

### Resource Limits

Default limits in `docker-compose.swarm.yml`:
- CPU: 0.5-2 cores
- Memory: 512MB-2GB

Adjust based on your workload.

### Volumes

- `/var/lib/postgresql/data`: PostgreSQL data directory (Docker volume)
- `/mnt/storagebox/backups/postgresql`: Backup storage (bind mount with `rslave`)

The `rslave` mount propagation ensures backups are visible on the host.

## Monitoring

### Service Status

```bash
# Check service status
make ps

# View logs
make logs

# Access container shell
make shell
```

### Health Checks

Health check runs every 30s:
```bash
psql -U postgres -c "SELECT 1"
```

Unhealthy after 3 failed attempts (90s total).

## Maintenance

### Update Service

```bash
# Build new version
make build VERSION=v1.1.0

# Push to registry
make push VERSION=v1.1.0

# Update running service
make update VERSION=v1.1.0
```

### Remove Service

```bash
make remove
```

### Clean Local Images

```bash
make clean
```

## Security Notes

- PostgreSQL password stored in Docker secrets (encrypted at rest)
- Configuration file stored in Docker config
- Backups compressed with bzip2 (saves ~70% space)
- SHA256 checksums for backup integrity verification
- PostgreSQL runs as `postgres` user (non-root)

## Troubleshooting

### Container won't start

Check logs:
```bash
make logs
```

Common issues:
- Secret `postgres_password` not created
- Config `postgresql-conf` not created
- Network `backend-net` doesn't exist
- No node labeled with `database=true`

### Backup not working

Check backup logs:
```bash
docker exec <container> cat /var/log/postgresql/backup.log
```

Verify backup directory permissions:
```bash
docker exec <container> ls -la /backups
```

### Restore fails

Verify backup integrity:
```bash
docker exec <container> sha256sum -c /backups/full/YYYYMMDD_HHMMSS_full.sql.bz2.sha256
```

Check available space:
```bash
docker exec <container> df -h
```

## Architecture

```
┌─────────────────────────────────────────┐
│ Docker Swarm Node (database=true label) │
│                                         │
│  ┌──────────────────────────────────┐  │
│  │ PostgreSQL Container             │  │
│  │                                  │  │
│  │  ┌────────────────────────────┐ │  │
│  │  │ PostgreSQL 16              │ │  │
│  │  │ - Data: /var/lib/postgresql│ │  │
│  │  │ - Config: Docker config    │ │  │
│  │  │ - Password: Docker secret  │ │  │
│  │  └────────────────────────────┘ │  │
│  │                                  │  │
│  │  ┌────────────────────────────┐ │  │
│  │  │ Cron (dcron)               │ │  │
│  │  │ - Sunday 3AM: Full         │ │  │
│  │  │ - Wednesday 3AM: Diff      │ │  │
│  │  │ - Hourly: Incremental      │ │  │
│  │  └────────────────────────────┘ │  │
│  │                                  │  │
│  │  ┌────────────────────────────┐ │  │
│  │  │ Backup Scripts             │ │  │
│  │  │ - Change detection         │ │  │
│  │  │ - Compression (bzip2)      │ │  │
│  │  │ - Tiered retention         │ │  │
│  │  │ - Auto-restore             │ │  │
│  │  └────────────────────────────┘ │  │
│  └──────────────────────────────────┘  │
│                                         │
│  /mnt/storagebox/backups/postgresql    │
│  (rslave mount)                         │
└─────────────────────────────────────────┘
```

## Version

See `VERSION` file for current version.

## License

MIT
