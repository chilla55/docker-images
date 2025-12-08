# Node 3 Resource Planning - Multi-Service Host

## Server Specs (Hetzner CX23)
- **CPU**: 2 vCPU
- **RAM**: 4 GB
- **Storage**: 40 GB NVMe SSD

## Services Running on Node 3

1. **Docker Swarm Manager** (control plane)
2. **Certbot** (SSL certificate management)
3. **PostgreSQL** (database)
4. **MariaDB Node 3** (Galera cluster member)

## Resource Allocation Analysis

### Current Allocations

| Service | CPU Reserved | CPU Limit | RAM Reserved | RAM Limit |
|---------|--------------|-----------|--------------|-----------|
| **Swarm Manager** | 0.3 | 0.5 | 256M | 512M |
| **Certbot** | 0.1 | 0.2 | 128M | 256M |
| **PostgreSQL** | 0.5 | 1.0 | 512M | 1G |
| **MariaDB** | 0.5 | 1.5 | 1.5G | 2.5G |
| **OS/Docker** | - | - | ~500M | ~750M |
| **TOTAL** | 1.4 | 3.2 | 2.9G | 5G |

### ⚠️ PROBLEM: Overcommitted!

**RAM**: 5G needed but only 4G available
**CPU**: 3.2 cores needed but only 2 available

## Solution Options

### Option 1: Reduce MariaDB Resources (Recommended)

Make Node 3 a **lightweight quorum member only**:

```yaml
mariadb-node3:
  resources:
    limits:
      cpus: '0.8'      # Down from 1.5
      memory: 1.5G     # Down from 2.5G
    reservations:
      cpus: '0.3'      # Down from 0.5
      memory: 1G       # Down from 1.5G
```

**New total**:
```
CPU: 0.5 + 0.2 + 1.0 + 0.8 = 2.5 limit (still over but acceptable)
RAM: 512M + 256M + 1G + 1.5G + 500M = 3.77G ✅
```

**Node 3 MariaDB config** (`mariadb-node3.cnf`):
```ini
innodb_buffer_pool_size  = 768M     # Down from 1.5G
max_connections          = 100      # Down from 200
query_cache_size         = 16M      # Down from 32M
```

### Option 2: Move PostgreSQL Off Node 3

Deploy PostgreSQL on Node 1 or Node 2:

```yaml
postgresql:
  deploy:
    placement:
      constraints:
        - node.labels.mariadb.node == node1  # Move to Node 1
```

**Node 3 becomes**:
```
CPU: 0.5 + 0.2 + 1.5 = 2.2 limit ✅
RAM: 512M + 256M + 2.5G + 500M = 3.77G ✅
```

### Option 3: Use Garbd Instead of Full MariaDB

Replace MariaDB Node 3 with lightweight Garbd (arbitrator):

```yaml
garbd:
  image: perconalab/garbd:latest
  environment:
    GALERA_CLUSTER: "gcomm://node1-ip,node2-ip"
    GALERA_GROUP: "mariadb_cluster"
  resources:
    limits:
      cpus: '0.3'
      memory: 256M
    reservations:
      cpus: '0.1'
      memory: 128M
```

**Node 3 becomes**:
```
CPU: 0.5 + 0.2 + 1.0 + 0.3 = 2.0 limit ✅
RAM: 512M + 256M + 1G + 256M + 500M = 2.52G ✅
```

**Benefits**:
- Much lighter resource usage
- Still provides quorum for Galera
- No data storage needed
- Better for multi-service hosts

### Option 4: Upgrade Server (Most Reliable)

**Hetzner CX33** (€7.35/month):
- 4 vCPU (+2)
- 8 GB RAM (+4GB)
- 80 GB NVMe (+40GB)
- Cost increase: €3.20/month

All services fit comfortably with room for growth.

## Recommended Configuration

### If Keeping CX23: Use Garbd

This is the **best balance** for a multi-service manager node:

**Update docker-compose**:
```yaml
# Remove mariadb-node3, add:
garbd:
  image: perconalab/garbd:latest
  hostname: garbd
  environment:
    GALERA_CLUSTER: "gcomm://${NODE1_IP}:4567,${NODE2_IP}:4567"
    GALERA_GROUP: "${GALERA_CLUSTER_NAME:-mariadb_cluster}"
    GALERA_OPTIONS: "gmcast.listen_addr=tcp://0.0.0.0:4567"
  networks:
    - mariadb-cluster
  deploy:
    mode: replicated
    replicas: 1
    placement:
      constraints:
        - node.labels.mariadb.node == node3
        - node.role == manager
    restart_policy:
      condition: on-failure
      delay: 5s
      max_attempts: 3
    resources:
      limits:
        cpus: '0.3'
        memory: 256M
      reservations:
        cpus: '0.1'
        memory: 128M
```

**Final Node 3 allocation**:
```
Service          CPU   RAM
─────────────────────────────
Swarm Manager   0.5   512M
Certbot         0.2   256M
PostgreSQL      1.0    1G
Garbd           0.3   256M
OS/Docker       -     500M
─────────────────────────────
TOTAL           2.0   2.5G  ✅ Fits!
```

## Storage Planning

### 40GB NVMe Breakdown

```
Service              Space
───────────────────────────────
OS + Docker          8 GB
PostgreSQL data      10-15 GB
Garbd                ~100 MB
Certbot certs        ~500 MB
Docker volumes       2 GB
Logs                 1 GB
Free buffer          10-15 GB
───────────────────────────────
TOTAL                ~35 GB ✅
```

**Monitor disk usage**:
```bash
# Check overall
df -h

# Check by service
docker system df
du -sh /var/lib/docker/volumes/*
```

## Network Considerations

### Swarm Manager Traffic

**Gossip protocol**: ~1-5 MB/hour
**Service coordination**: ~10-50 MB/day
**Not significant** compared to 20TB allowance

### Combined Traffic Estimate

```
Service          Monthly Traffic
────────────────────────────────
Galera (Garbd)   ~5-10 GB
PostgreSQL       ~50-100 GB (depends on usage)
Certbot          ~100 MB
Swarm gossip     ~2 GB
────────────────────────────────
TOTAL            ~60-115 GB << 20 TB ✅
```

## Swarm Manager Implications

### Why Manager Node Matters

**Should run stable services**:
- ✅ Certbot (lightweight, infrequent runs)
- ✅ Garbd (lightweight, critical for quorum)
- ⚠️ PostgreSQL (medium load, acceptable)
- ❌ Heavy MariaDB node (competes with manager duties)

**Manager node responsibilities**:
- Cluster state management
- Task scheduling
- Service orchestration
- Health monitoring

**Under load**: Heavy database + manager duties = degraded cluster performance

### Best Practice

```
Manager nodes: Run control plane + lightweight services
Worker nodes:  Run heavy workloads (databases, apps)
```

**With 3 nodes**:
```
Node 1 (Worker):  MariaDB Node 1, Heavy apps
Node 2 (Worker):  MariaDB Node 2, PostgreSQL
Node 3 (Manager): Garbd, Certbot, Lightweight apps
```

## Migration Steps

### If Using Garbd Instead of MariaDB Node 3

1. **Update docker-compose.swarm.yml** (remove node3, add garbd)
2. **Update MaxScale config** (remove node3 from servers)
3. **Deploy changes**:
   ```bash
   docker stack deploy -c docker-compose.swarm.yml mariadb-cluster
   ```

4. **Verify cluster**:
   ```bash
   # On Node 1 or 2
   docker exec <mariadb-container> mysql -u root -p -e "SHOW STATUS LIKE 'wsrep_cluster_size';"
   # Should show: 3 (2 nodes + 1 garbd)
   ```

## Performance Expectations

### Node 3 as Manager + Multi-Service

**Acceptable**:
- Swarm cluster management: ✅
- Certbot renewals: ✅
- PostgreSQL (light-medium load): ✅
- Garbd quorum: ✅

**Not recommended**:
- Full MariaDB node with production load: ❌
- Heavy PostgreSQL (>500 qps): ⚠️
- Resource-intensive apps: ❌

### When to Scale Up

Upgrade to CX33 (€7.35) if you see:
- Manager response time >200ms
- PostgreSQL CPU >80%
- RAM usage >85%
- Swarm task scheduling delays
- OOM (out of memory) kills

## Monitoring Setup

### Critical Alerts for Node 3

```bash
# Memory pressure
docker stats --no-stream

# Swarm health
docker node inspect node3 --format '{{.Status.State}}'

# PostgreSQL performance
docker exec <postgres> psql -c "SELECT * FROM pg_stat_database;"

# Disk space
df -h | grep -E "/$|/var/lib/docker"
```

**Alert thresholds**:
- RAM >90%: Critical
- Disk >85%: Warning
- CPU sustained >90%: Warning
- Swarm state != ready: Critical

## Final Recommendation

**For CX23 hosting Manager + Certbot + PostgreSQL + MariaDB quorum**:

1. ✅ Use **Garbd** instead of full MariaDB node
2. ✅ Keep PostgreSQL on Node 3 if light-medium load
3. ✅ Move PostgreSQL to Node 1/2 if heavy load expected
4. ⚠️ Monitor first month and scale if needed
5. 💡 Consider CX33 if running production workloads

**Cost-benefit**:
- CX23 + Garbd: €4.15/month - Tight but workable
- CX33 + Full MariaDB: €7.35/month - Comfortable headroom
- Difference: €3.20/month for peace of mind
