# PostgreSQL Single-Node with Automated Backups

Production-ready single-node PostgreSQL setup with automated backup capabilities using pg_basebackup.

## Features

- **Single-Node Design**: Simplified deployment on srv1.chilla55.de
- **Automated Backups**: 
  - Weekly full backups (Sunday 3 AM)
  - Daily WAL archiving (continuous)
  - 30-day retention by default
- **Backup Strategy**: pg_basebackup with WAL archiving
- **Storage**: Backups saved to `/mnt/storagebox/postgresql-backups`

## Quick Start

### Build and Deploy

```bash
cd postgresql
make build push deploy
```

### Manual Backups

```bash
make backup-full        # Full backup
make backup-incremental # Archive WAL files
```

### Restore

```bash
make restore  # Lists available backups and restore instructions
```

## Backup Strategy

- **Weekly Full**: Complete database snapshot using pg_basebackup (Sunday 3 AM)
- **Continuous WAL**: Write-Ahead Log files archived continuously
- **Retention**: 30 days automatic cleanup
- **Storage**: `/mnt/storagebox/postgresql-backups`

## Configuration

Edit `docker-compose.yml`:
```yaml
BACKUP_FULL_SCHEDULE: "0 3 * * 0"      # Weekly full
BACKUP_INCREMENTAL_SCHEDULE: "0 3 * * 1-6"  # Daily WAL archive
BACKUP_RETENTION_DAYS: "30"
```

## Commands

```bash
make build              # Build image
make push               # Push to registry
make deploy             # Deploy stack
make logs               # View logs
make backup-full        # Trigger full backup
make backup-incremental # Archive WAL files
make restore            # Restore from backup
```

## Network

Service available on `postgres-net` as:
- `postgresql` (service name)
- `postgresql.srv1.chilla55.de` (hostname)
- Port: `5432`
