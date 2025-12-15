# MariaDB Migration Summary

## What Changed

### Removed Components
- ❌ Master-Slave replication setup
- ❌ MaxScale orchestrator
- ❌ PgPool-II connection pooler  
- ❌ Multi-node complexity
- ❌ Replication monitoring scripts

### New Single-Node Setup
- ✅ Single MariaDB 11.7 instance on srv1
- ✅ Automated backup system using mariabackup
- ✅ Weekly full + daily incremental backups
- ✅ 30-day retention policy
- ✅ Backups stored on storage box: `/mnt/storagebox/mariadb-backups`

## Backup Strategy

### Schedule
- **Full Backup**: Every Sunday at 2:00 AM
- **Incremental Backup**: Monday-Saturday at 2:00 AM

### How It Works
1. **Week 1**: Sunday full backup creates baseline
2. **Week 1**: Mon-Sat incrementals only backup changes
3. **Week 2**: Sunday new full backup (old weekly cycle archived)
4. **Retention**: Backups older than 30 days deleted automatically

### Storage Requirements
- Full backup: ~1-5 GB (depends on data)
- Incremental backup: ~50-500 MB per day
- Weekly storage: ~1.5-8 GB
- Monthly storage (with rotation): ~6-30 GB

### Restore Process
To restore, you need:
1. The full backup from the start of the week
2. All incremental backups up to desired recovery point
3. Run: `make restore` and follow prompts

## Migration Steps

### 1. Create Backup Directory on Storage Box
```bash
ssh srv0 "sudo mkdir -p /mnt/storagebox/mariadb-backups"
```

### 2. Export Data from Current Setup
```bash
# Dump from secondary (currently running on mail node)
ssh mail "sudo docker exec -i \$(sudo docker ps -q -f name=mariadb-secondary) \
  mysqldump -u root -p'2vQ9jTXzaT9nkJF4eRacjl0g6+tytR6OlbrE8Zve26U=' \
  --all-databases --single-transaction --routines --triggers --events \
  > /tmp/mariadb-migration.sql"

# Copy to srv0
scp mail:/tmp/mariadb-migration.sql /tmp/
```

### 3. Stop Old Services
```bash
ssh srv0 "sudo docker stack rm Mariadb"
```

### 4. Build and Deploy New MariaDB
```bash
cd mariadb
make build push deploy
```

### 5. Import Data
```bash
# Wait for service to be ready
sleep 30

# Get container ID
CONTAINER=$(ssh srv1 "sudo docker ps -q -f name=mariadb-single")

# Copy dump
ssh srv1 "sudo docker cp /tmp/mariadb-migration.sql ${CONTAINER}:/tmp/"

# Import
ssh srv1 "sudo docker exec -i ${CONTAINER} mysql -u root -p\$(cat /run/secrets/mysql_root_password) < /tmp/mariadb-migration.sql"
```

### 6. Update Vaultwarden Connection
The connection in vaultwarden already points to `mariadb-secondary:3306`. 
Update to `mariadb:3306` or `mariadb.srv1.chilla55.de:3306`.

Edit `vaultwarden/entrypoint.sh`:
```bash
export DATABASE_URL="mysql://vaultwarden:${DB_PASSWORD}@mariadb:3306/vaultwarden"
```

Rebuild and deploy:
```bash
cd vaultwarden
git add entrypoint.sh
git commit -m "Update MariaDB connection to single-node setup"
git push
# Wait for GitHub Actions build
make deploy
```

### 7. Test First Backup
```bash
# Trigger immediate full backup
cd mariadb
make backup-full

# Verify backup created
ssh srv1 "ls -lh /mnt/storagebox/mariadb-backups/full/"
```

## Benefits

### Simplified Operations
- One node to manage instead of three
- No replication lag issues
- No split-brain scenarios
- Easier troubleshooting

### Reliable Backups
- Industry-standard mariabackup tool
- Compressed storage (~80% space savings)
- Point-in-time recovery with binary logs
- Automated retention management

### Cost Efficiency
- Reduced resource usage (2 fewer nodes)
- Lower storage costs (compression)
- Minimal operational overhead

## Monitoring

### Check Backup Status
```bash
# View last full backup
ssh srv1 "ls -lht /mnt/storagebox/mariadb-backups/full/ | head -3"

# View recent incrementals
ssh srv1 "ls -lht /mnt/storagebox/mariadb-backups/incremental/ | head -5"

# Check backup logs
make logs
```

### Test Restore (Dry Run)
```bash
# List available backups
make restore
```

## Rollback Plan

If issues occur, you can temporarily revert:

1. Keep old stack definition files
2. Export data from new setup
3. Redeploy old Mariadb stack
4. Import data back

However, the single-node setup is simpler and more reliable for this use case.

## Questions & Answers

**Q: What if srv1 goes down?**  
A: Restore from latest backup to any node. Recovery time: ~10-30 minutes depending on data size.

**Q: Can I change backup times?**  
A: Yes, edit `docker-compose.yml` and update cron schedules.

**Q: How do I restore to a specific point in time?**  
A: Use full backup + incrementals + binary logs. See restore script documentation.

**Q: Do I still need the secondary on mail node?**  
A: No, it can be decommissioned once migration is confirmed working.

**Q: What about high availability?**  
A: For most use cases, good backups are better than complex HA setups. If needed, can add replication later.
