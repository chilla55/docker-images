# Resource Planning for MariaDB Galera Cluster

## Node 3 Specifications (Hetzner CX23)
- **CPU**: 2 vCPU (Intel/AMD)
- **RAM**: 4 GB
- **Storage**: 40 GB NVMe SSD
- **Traffic**: 20 TB included
- **Cost**: €4.15/month

## Resource Allocation

### Current Setup

| Component | Node 1 | Node 2 | Node 3 (CX23) | MaxScale |
|-----------|--------|--------|---------------|----------|
| CPU Limit | 2.0 | 2.0 | **1.5** | 1.0 |
| CPU Reserved | 1.0 | 1.0 | **0.5** | 0.5 |
| RAM Limit | 2G | 2G | **2.5G** | 512M |
| RAM Reserved | 1G | 1G | **1.5G** | 256M |

### Why These Limits for Node 3

**Available Resources**:
- 2 vCPU total
- 4 GB RAM total
- ~500MB needed for OS/Docker

**Allocation Strategy**:
```
MariaDB:   1.5 vCPU (75%), 2.5GB RAM (62.5%)
System:    0.5 vCPU (25%), 1.5GB RAM (37.5%)
```

This leaves headroom for:
- Operating system
- Docker overhead
- Network stack
- Temporary spikes

## MariaDB Configuration Tuning for Node 3

### Memory Distribution (2.5GB available)

```ini
innodb_buffer_pool_size  = 1.5G   (60% of available RAM)
query_cache_size         = 32M    (reduced)
max_connections          = 200    (reduced from 500)
tmp_table_size           = 16M    (reduced from 32M)
max_heap_table_size      = 16M    (reduced from 32M)
sort_buffer_size         = 2M     (reduced from 4M)
```

**Memory breakdown**:
- InnoDB Buffer Pool: 1.5 GB (main data cache)
- Query Cache: 32 MB
- Connection buffers: ~400 MB (200 × 2MB)
- OS/overhead: ~500 MB
- Total: ~2.5 GB

### Storage Optimization (40GB NVMe)

**Disk usage estimate**:
```
OS + Docker:           ~8 GB
MariaDB binaries:      ~500 MB
Data directory:        ~15-20 GB (depends on your data)
Binary logs:           ~2-3 GB (3 days retention)
InnoDB logs:           ~256 MB
Temporary files:       ~1 GB
Free space buffer:     ~10 GB (25% recommended)
```

**Monitoring disk space**:
```sql
-- Check database sizes
SELECT 
    table_schema AS 'Database',
    ROUND(SUM(data_length + index_length) / 1024 / 1024, 2) AS 'Size (MB)'
FROM information_schema.tables
GROUP BY table_schema;
```

### Binary Log Management

For 40GB storage, keep logs shorter:
```ini
expire_logs_days = 3        # Down from 7 days
max_binlog_size = 50M       # Down from 100M
```

**Cleanup if needed**:
```sql
PURGE BINARY LOGS BEFORE DATE_SUB(NOW(), INTERVAL 2 DAY);
```

## Traffic Considerations (20TB/month)

### Galera Replication Traffic

**Bandwidth usage estimation**:
- Write-heavy: ~2-5× data size per month
- Read-heavy: ~1-2× data size per month
- SST (full sync): Transfers entire dataset

**Example**:
```
10 GB database, 1000 writes/sec:
- Daily writes: ~10 GB
- Monthly replication: ~300 GB
- SST events: ~10 GB each

Total: ~400-500 GB/month << 20 TB limit ✅
```

**20TB is more than enough** unless you're doing:
- Constant full SST transfers
- Streaming large BLOBs
- Very high write volume (>10k writes/sec)

## Performance Expectations

### Node 3 as Quorum Member

**Best configuration**:
```
Node 1 (Powerful): Primary for writes + reads
Node 2 (Powerful): Secondary for reads
Node 3 (CX23):    Quorum member + light reads
```

**In MaxScale, set priority** (already configured):
```ini
[mariadb-node1]
priority=1    # Preferred for writes

[mariadb-node2]
priority=2    # Secondary

[mariadb-node3]
priority=3    # Last resort
```

This ensures Node 3 only handles traffic when nodes 1 & 2 are unavailable.

### Benchmark Expectations

**Node 3 (2 vCPU, 4GB RAM)**:
- Reads: ~500-1000 queries/sec
- Writes: ~100-300 queries/sec
- Concurrent connections: ~150-200

**Compared to larger nodes**:
- Will have higher latency under load
- Fine for quorum/failover
- Not ideal for primary production traffic

## Scaling Recommendations

### When to Upgrade Node 3

Upgrade if you see:
- CPU constantly >80%
- RAM usage >90%
- Disk space <20% free
- Replication lag >5 seconds
- Node frequently falls behind in Galera

**Next step**: Hetzner CX33
- 4 vCPU
- 8 GB RAM
- 80 GB NVMe
- €7.35/month

### Alternative: Dedicated Arbitrator

If Node 3 is only for quorum:
```yaml
# Replace Node 3 with lightweight Garbd
garbd:
  image: perconalab/garbd:latest
  environment:
    GALERA_CLUSTER: "gcomm://node1,node2"
    GALERA_GROUP: "mariadb_cluster"
  resources:
    limits:
      cpus: '0.5'
      memory: 256M
```

**Benefits**:
- Uses minimal resources
- Still provides quorum
- Cheaper (can run on smallest VPS)
- No data storage needed

## Monitoring Commands

### Check Resource Usage

```bash
# CPU usage
docker stats --no-stream

# Memory breakdown
docker exec <container> mysql -u root -p -e "
SHOW VARIABLES LIKE 'innodb_buffer_pool_size';
SHOW STATUS LIKE 'Innodb_buffer_pool_pages%';
"

# Disk usage
docker exec <container> df -h /var/lib/mysql
docker exec <container> du -sh /var/lib/mysql/*
```

### Set Up Alerts

Monitor these on Node 3:
- Disk usage >70%
- Memory >85%
- Replication lag >10s
- CPU >90% for >5min

## Cost Analysis

### Current Setup (3 nodes)

```
Node 1 (your specs):     €X/month
Node 2 (your specs):     €Y/month
Node 3 (CX23):          €4.15/month
Total:                  €X+Y+4.15/month
```

### Alternative: 2 Nodes + Garbd

```
Node 1:                 €X/month
Node 2:                 €Y/month
Garbd (CX12):          €2.49/month  (1 vCPU, 2GB)
Total:                 €X+Y+2.49/month (saves €1.66)
```

**Trade-off**: Garbd provides quorum only, no read capacity

## Recommendations

✅ **Node 3 as configured is fine for**:
- Quorum member (primary purpose)
- Failover/disaster recovery
- Light read queries (<100 qps)
- Development/staging environments

⚠️ **Consider upgrading if**:
- You need Node 3 to handle production reads
- Database grows >15GB
- High write volume requires more replication power

💡 **Cost optimization**:
- Use Garbd instead if you only need quorum
- Monitor actual usage first month
- Scale up only if needed

## Summary

Node 3 (CX23) specs are **adequate** for a Galera cluster quorum member:
- ✅ CPU sufficient for replication
- ✅ RAM enough for reasonable datasets (<15GB)
- ✅ Storage adequate with log management
- ✅ Traffic allowance more than enough
- ⚠️ Not ideal as primary production node
- ✅ Perfect for high availability at low cost
