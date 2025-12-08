# Master-Slave Replication with MaxScale Orchestrator

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Applications                            │
│                    (Connect to MaxScale)                        │
└────────────────────────────┬────────────────────────────────────┘
                             │
                    Port 4006 (Read-Write)
                             │
                    ┌────────▼─────────┐
                    │    MaxScale      │  ◄── Node 3 (Orchestrator)
                    │  (Auto Failover) │      CX23: 2 vCPU, 4GB
                    └────────┬─────────┘
                             │
              ┌──────────────┴──────────────┐
              │                             │
         Replication                   Replication
         Monitor                       Monitor
              │                             │
    ┌─────────▼──────────┐       ┌─────────▼──────────┐
    │  PRIMARY (Node 1)  │       │ SECONDARY (Node 2) │
    │   Read + Write     │──────►│    Read Only       │
    │                    │ Async │                    │
    └────────────────────┘ Repl  └────────────────────┘
```

## How It Works

### Normal Operation

**Primary (Node 1)**:
- Handles ALL write queries
- Can handle read queries
- Sends binary log to Secondary

**Secondary (Node 2)**:
- Receives updates from Primary (async replication)
- Handles read queries only (read_only=1)
- Standby for automatic promotion

**MaxScale (Node 3)**:
- Monitors both nodes every 2 seconds
- Routes writes → Primary
- Routes reads → Secondary (or Primary if needed)
- Detects failures and triggers automatic failover

### Failover Scenarios

#### Scenario 1: Primary Goes Down

```
1. MaxScale detects Primary is unreachable (6 seconds / 3 failed pings)
2. MaxScale verifies:
   - Can it reach Secondary? ✅
   - Is Node 3 (MaxScale) still reachable? ✅
3. MaxScale promotes Secondary to Primary:
   - Disables read_only on Secondary
   - Secondary becomes new Primary
4. Applications continue via MaxScale (transparent)
5. Old Primary rejoins as Secondary when back online
```

**Downtime**: ~10-15 seconds

#### Scenario 2: Secondary Goes Down

```
1. MaxScale detects Secondary is unreachable
2. Routes all queries to Primary
3. No failover needed
4. Secondary rejoins and syncs when back online
```

**Downtime**: 0 seconds (no impact)

#### Scenario 3: Primary Can't Reach Secondary + MaxScale

```
1. Primary tries to reach Secondary → FAIL (3 attempts)
2. Primary tries to reach MaxScale → FAIL (3 attempts)
3. Primary: "I'm isolated, entering read_only mode" ✅
4. MaxScale can still reach Secondary
5. MaxScale promotes Secondary to Primary
6. Prevents split-brain ✅
```

**Connectivity Monitor Logic**:
```bash
# On Primary:
- Check Secondary every 5s
- Check MaxScale every 5s
#### Scenario 4: Network Partition (Split-Brain Prevention)

**Case A: Primary isolated**
```
Primary ←X→ Secondary (no connection)
Primary ←X→ MaxScale (no connection)
Secondary ←→ MaxScale (connected)

1. Primary connectivity monitor detects isolation (15s)
2. Primary: SET read_only=ON, super_read_only=ON
3. MaxScale detects Primary down
4. MaxScale promotes Secondary → new Primary
5. Split-brain prevented ✅
```

**Case B: Secondary isolated**
```
Primary ←→ MaxScale (connected)
Primary ←X→ Secondary (no connection)
Secondary ←X→ MaxScale (no connection)

1. Secondary connectivity monitor detects isolation
2. Secondary stays read_only
3. Primary continues serving (can reach MaxScale)
4. No failover needed
5. Secondary rejoins when network recovers
```

**Case C: MaxScale isolated**
```
Primary ←→ Secondary (connected)
Primary ←X→ MaxScale (no connection)
Secondary ←X→ MaxScale (no connection)

1. Primary can reach Secondary → stays active
2. Secondary can reach Primary → replication continues
3. MaxScale isolated → cannot trigger failover
4. System continues normally without orchestrator
5. Manual intervention may be needed if Primary fails
```

#### Scenario 4: Network Partition (Split-Brain Prevention)

```
If Primary can't reach MaxScale (Node 3):
- Primary: "I can't confirm quorum" → read_only
- MaxScale can reach Secondary → promotes Secondary
- Only one active writer at a time ✅
```

## Resource Usage

### Node 3 (Manager + MaxScale Only)

```
Service          CPU    RAM    Disk
──────────────────────────────────────
Swarm Manager   0.5    512M   -
Certbot         0.2    256M   500M
PostgreSQL      1.0    1G     10-15G
MaxScale        0.5    512M   1G
OS/Docker       -      500M   8G
──────────────────────────────────────
TOTAL           2.2    2.8G   ~25G ✅
```

**Much lighter** than running full MariaDB node!

### Benefits vs Galera

| Feature | Galera (3 nodes) | Master-Slave + MaxScale |
|---------|------------------|-------------------------|
| Node 3 CPU | 1.5 cores | 0.5 cores |
| Node 3 RAM | 2.5GB | 512MB |
| Node 3 Disk | 10-15GB | 1GB |
| Write Performance | Slower (sync all) | Faster (async) |
| Network Sensitivity | Very high | Medium |
| Internet Latency Impact | High | Low |
| Split-Brain Risk | Higher (2 nodes) | Lower (arbitrator) |
| Complexity | High | Medium |

## Setup Instructions

### 1. Build Replication Image

```bash
cd /path/to/mariadb
docker build -f Dockerfile.replication -t mariadb-replication:latest .
```

### 2. Configure Environment

Update `.env`:
```bash
# Add replication password
REPLICATION_PASSWORD=your_strong_replication_password
```

### 3. Deploy Stack

```bash
docker stack deploy -c docker-compose.replication.yml mariadb-cluster
```

### 4. Setup Replication

**On Primary (Node 1)**:
```bash
docker exec -it <primary-container> mysql -u root -p

# Create replication user
CREATE USER 'replicator'@'%' IDENTIFIED BY 'your_strong_replication_password';
GRANT REPLICATION SLAVE ON *.* TO 'replicator'@'%';
FLUSH PRIVILEGES;

# Note the position
SHOW MASTER STATUS;
# Remember: File and Position
```

**On Secondary (Node 2)**:
```bash
docker exec -it <secondary-container> mysql -u root -p

# Configure replication
CHANGE MASTER TO
  MASTER_HOST='mariadb-primary',
  MASTER_USER='replicator',
  MASTER_PASSWORD='your_strong_replication_password',
  MASTER_PORT=3306,
  MASTER_LOG_FILE='mariadb-bin.000001',  # From SHOW MASTER STATUS
  MASTER_LOG_POS=123456;                 # From SHOW MASTER STATUS

# Start replication
START SLAVE;

# Verify
SHOW SLAVE STATUS\G
# Look for:
# Slave_IO_Running: Yes
# Slave_SQL_Running: Yes
```

### 5. Create MaxScale User

```bash
docker exec -it <primary-container> mysql -u root -p

# Create MaxScale monitoring user
CREATE USER 'maxscale'@'%' IDENTIFIED BY 'your_maxscale_password';
GRANT SELECT ON mysql.user TO 'maxscale'@'%';
GRANT SELECT ON mysql.db TO 'maxscale'@'%';
GRANT SELECT ON mysql.tables_priv TO 'maxscale'@'%';
GRANT SELECT ON mysql.columns_priv TO 'maxscale'@'%';
GRANT SHOW DATABASES ON *.* TO 'maxscale'@'%';
GRANT REPLICATION CLIENT ON *.* TO 'maxscale'@'%';
GRANT REPLICATION SLAVE ON *.* TO 'maxscale'@'%';

# Grant failover permissions
GRANT SUPER ON *.* TO 'maxscale'@'%';
GRANT RELOAD ON *.* TO 'maxscale'@'%';
GRANT PROCESS ON *.* TO 'maxscale'@'%';
GRANT CREATE ON *.* TO 'maxscale'@'%';
GRANT EVENT ON *.* TO 'maxscale'@'%';

FLUSH PRIVILEGES;
```

## Monitoring

### Check Replication Status

```bash
# On Secondary
docker exec <secondary> mysql -u root -p -e "SHOW SLAVE STATUS\G"

# Important fields:
# Slave_IO_Running: Yes
# Slave_SQL_Running: Yes
# Seconds_Behind_Master: 0
```

### Check MaxScale Status

```bash
# Via REST API
curl http://maxscale:8989/v1/servers

# Via MaxCtrl (inside container)
docker exec <maxscale> maxctrl list servers
docker exec <maxscale> maxctrl show monitor MariaDB-Monitor
```

### Test Failover

```bash
# Manual failover
docker exec <maxscale> maxctrl call command mariadbmon failover MariaDB-Monitor

# Manual switchover (graceful)
docker exec <maxscale> maxctrl call command mariadbmon switchover MariaDB-Monitor
```

## Application Connection

**Connect to MaxScale** (not directly to nodes):
```bash
mysql -h <maxscale-host> -P 4006 -u app_user -p
```

MaxScale automatically:
- Routes writes to current Primary
- Routes reads to Secondary
- Handles failover transparently

## Advantages for Your Setup

✅ **Node 3 Resources**: Freed up ~2GB RAM, 1 CPU core, 10GB disk
✅ **Performance**: Faster writes (no sync replication over internet)
✅ **Network Resilient**: Async replication handles latency better
✅ **Simple Failover**: Clear Primary/Secondary roles
✅ **Cost**: Node 3 can stay CX23, no upgrade needed
✅ **Multi-Service**: Node 3 has room for Swarm Manager + Certbot + PostgreSQL

## Disadvantages vs Galera

⚠️ **Async Replication**: Secondary may be slightly behind (usually <1 second)
⚠️ **Single Writer**: Can't write to Secondary (Galera allowed multi-master)
⚠️ **Failover Gap**: ~10-15 seconds downtime during failover (vs Galera's instant)

## When to Use Each

**Use Master-Slave + MaxScale**:
- Nodes across different providers (internet between them)
- One node is resource-constrained (Node 3)
- Write performance is critical
- Acceptable brief downtime during failover

**Use Galera Cluster**:
- All nodes in same datacenter (low latency <5ms)
- Need zero-downtime failover
- Need multi-master writes
- All nodes have equal resources

## Recommended for Your Setup

**Master-Slave is BETTER** because:
1. Node 3 is resource-constrained (CX23)
2. Nodes are across providers (internet latency)
3. Simpler to manage and troubleshoot
4. Better performance over WAN
5. Node 3 can focus on orchestration

This is the **recommended architecture** for your multi-provider, multi-service setup!
