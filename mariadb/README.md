# MariaDB Single-Node with Automated Backups

Production-ready single-node MariaDB setup with automated backup capabilities using mariabackup.

## Features

- **Single-Node Design**: Simplified deployment on srv1.chilla55.de
- **Automated Backups**: 
  - Weekly full backups (Sunday 2 AM)
  - Daily incremental backups (Monday-Saturday 2 AM)
  - 30-day retention by default
- **Backup Strategy**: mariabackup with incremental support
- **Storage**: Backups saved to `/mnt/storagebox/mariadb-backups`
- **Binary Logging**: 7-day retention for point-in-time recovery

## Quick Start

### Build and Deploy

```bash
cd mariadb
make build push deploy
```

### Manual Backups

```bash
make backup-full        # Full backup
make backup-incremental # Incremental backup
```

### Restore

```bash
make restore  # Lists available backups and restore instructions
```

## Backup Strategy

- **Weekly Full**: Complete database snapshot (Sunday 2 AM)
- **Daily Incremental**: Only changes since last backup (Mon-Sat 2 AM)
- **Retention**: 30 days automatic cleanup
- **Storage**: `/mnt/storagebox/mariadb-backups`

## Configuration

Edit `docker-compose.yml`:
```yaml
BACKUP_FULL_SCHEDULE: "0 2 * * 0"      # Weekly full
BACKUP_INCREMENTAL_SCHEDULE: "0 2 * * 1-6"  # Daily incremental
BACKUP_RETENTION_DAYS: "30"
```

## Commands

```bash
make build              # Build image
make push               # Push to registry
make deploy             # Deploy stack
make logs               # View logs
make backup-full        # Trigger full backup
make backup-incremental # Trigger incremental backup
make restore            # Restore from backup
```
