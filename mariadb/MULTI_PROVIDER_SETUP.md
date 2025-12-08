# Multi-Provider MariaDB Galera Cluster Setup

## Architecture for Nodes Across Different Hosting Providers

When running MariaDB Galera nodes across **different hosting providers** using the public internet for synchronization, special considerations apply.

## Critical Requirements

### 1. **Minimum 3 Nodes for High Availability**

With nodes communicating over the internet:
- **2 nodes = NO HIGH AVAILABILITY** - any network issue causes total outage
- **3 nodes = Minimum for quorum** - can survive 1 node/network failure
- **5 nodes = Recommended for production** - can survive 2 failures

**Quorum calculation**:
```
Nodes | Quorum Needed | Can Survive
------|---------------|-------------
  2   |      2        |  0 failures ❌
  3   |      2        |  1 failure  ✅
  5   |      3        |  2 failures ✅
  7   |      4        |  3 failures ✅
```

### 2. **Network Requirements**

**Ports to Open Between Providers**:
```
3306  - MySQL client connections
4444  - State Snapshot Transfer (SST)
4567  - Galera cluster replication
4568  - Incremental State Transfer (IST)
```

**Firewall Rules** (on each provider):
```bash
# Allow from other node IPs
ufw allow from <node1-public-ip> to any port 3306,4444,4567,4568
ufw allow from <node2-public-ip> to any port 3306,4444,4567,4568
ufw allow from <node3-public-ip> to any port 3306,4444,4567,4568
```

### 3. **Network Overlay Configuration**

Docker Swarm overlay networks **don't work** across providers. You need:

**Option A: WireGuard VPN Mesh** (Recommended)
```bash
# Creates encrypted tunnel between providers
# Galera traffic goes through VPN
# Lower latency than public internet routing
```

**Option B: Public IP Configuration**
```yaml
# In docker-compose, use public IPs
environment:
  GALERA_CLUSTER_ADDRESS: "gcomm://1.2.3.4,5.6.7.8,9.10.11.12"
  GALERA_NODE_ADDRESS: "1.2.3.4"  # This node's public IP
```

### 4. **Latency Optimization**

**Problem**: Galera is synchronous - every write waits for all nodes
```
Write time = Local processing + (Network latency × 2)
```

**Solutions**:
```ini
# In mariadb.cnf - optimize for WAN
[mysqld]
# Increase timeout for slow networks
wsrep_provider_options="evs.keepalive_period=PT3S;evs.inactive_timeout=PT30S"

# Use mariabackup for SST (faster over WAN)
wsrep_sst_method=mariabackup

# Async replication for read replicas
# Keep sync replication for critical nodes only
```

**Choose geographically close providers**:
```
Provider A (US-East) <-> Provider B (US-West) = ~60ms ✅
Provider A (US) <-> Provider B (Europe) = ~150ms ⚠️
Provider A (US) <-> Provider B (Asia) = ~300ms ❌
```

### 5. **Split-Brain Prevention**

**With 3 Nodes Across Providers**:
```
Provider A (Node 1) --- Provider B (Node 2) --- Provider C (Node 3)
```

**Scenario: Provider A loses internet**:
- Node 1: Can't reach nodes 2 & 3 (cluster_size = 1, no quorum) → READ-ONLY
- Nodes 2 & 3: Still connected (cluster_size = 2, has quorum) → CONTINUE ✅

**Scenario: Any 2 providers lose connection**:
- All nodes see cluster_size < 2 → All go READ-ONLY → Manual recovery needed

## Setup Instructions

### Step 1: Deploy Across Providers

**Provider A (US-East)**:
```bash
# Label the node
docker node update --label-add mariadb.node=node1 <node-name>
docker node update --label-add mariadb.provider=provider-a <node-name>
```

**Provider B (US-West)**:
```bash
docker node update --label-add mariadb.node=node2 <node-name>
docker node update --label-add mariadb.provider=provider-b <node-name>
```

**Provider C (US-Central)**:
```bash
docker node update --label-add mariadb.node=node3 <node-name>
docker node update --label-add mariadb.provider=provider-c <node-name>
```

### Step 2: Configure Public IPs

Update `.env`:
```bash
# Public IPs of each provider
NODE1_IP=1.2.3.4
NODE2_IP=5.6.7.8
NODE3_IP=9.10.11.12

GALERA_CLUSTER_ADDRESS=gcomm://${NODE1_IP}:4567,${NODE2_IP}:4567,${NODE3_IP}:4567
```

### Step 3: WireGuard Setup (Recommended)

Install on each node:
```bash
# Create mesh network
# Node 1: 10.0.0.1
# Node 2: 10.0.0.2
# Node 3: 10.0.0.3

# Use WireGuard IPs in cluster address
GALERA_CLUSTER_ADDRESS=gcomm://10.0.0.1,10.0.0.2,10.0.0.3
```

### Step 4: Deploy with Constraints

Modified placement for multi-provider:
```yaml
deploy:
  placement:
    constraints:
      - node.labels.mariadb.provider == provider-a
```

## Monitoring Cross-Provider Setup

### Check Replication Lag
```sql
SHOW STATUS LIKE 'wsrep_local_recv_queue_avg';
-- Should be < 1.0 for healthy replication
-- > 10 = serious lag, investigate network
```

### Check Network Latency
```bash
# From each node to others
docker exec <container> ping -c 10 <other-node-ip>
# Should be consistent, < 100ms ideal
```

### Monitor Cluster State
```sql
SHOW STATUS LIKE 'wsrep_cluster_status';
-- Must show: Primary

SHOW STATUS LIKE 'wsrep_cluster_size';
-- Must show: 3 (or your total node count)

SHOW STATUS LIKE 'wsrep_connected';
-- Must show: ON
```

## Disaster Recovery

### Lost Quorum (Network Partition)
```bash
# Find the most up-to-date node
docker exec <node> mysql -u root -p -e "SHOW STATUS LIKE 'wsrep_last_committed';"
# Node with highest value is most current

# Bootstrap from that node
docker service update --env-add GALERA_CLUSTER_ADDRESS="gcomm://" mariadb-cluster_mariadb-node1

# After it starts, rejoin others
docker service update --env-add GALERA_CLUSTER_ADDRESS="gcomm://node1,node2,node3" mariadb-cluster_mariadb-node2
docker service update --env-add GALERA_CLUSTER_ADDRESS="gcomm://node1,node2,node3" mariadb-cluster_mariadb-node3
```

### Complete Network Failure
```bash
# Promote single node to standalone
SET GLOBAL wsrep_provider_options='pc.bootstrap=1';
# WARNING: Only do this on most current node!
```

## Performance Tuning for WAN

```ini
[mysqld]
# Increase buffers for WAN latency
wsrep_provider_options="
    gcache.size=1G;
    evs.keepalive_period=PT3S;
    evs.inactive_timeout=PT30S;
    evs.suspect_timeout=PT10S;
    evs.install_timeout=PT30S;
    evs.send_window=512;
    evs.user_send_window=256
"

# Use parallel SST
wsrep_sst_method=mariabackup
[sst]
compress=1
parallel=4
```

## Cost Optimization

- Use **compression** for replication to reduce bandwidth
- Consider **async replicas** for read-heavy workloads (don't need sync)
- Monitor **egress costs** - can be expensive across providers
- Use **dedicated interconnects** if providers support it (AWS Direct Connect, etc.)

## When NOT to Use Multi-Provider Galera

❌ High write volume (>1000 writes/sec)
❌ Latency > 200ms between nodes  
❌ Unreliable internet connections
❌ Budget constraints (egress costs add up)

✅ **Better alternatives**:
- Single-provider HA with local replicas
- Async replication across providers
- Application-level sharding
- Managed database services (RDS Multi-AZ, etc.)

## Summary

**For 2 nodes across providers**: 
- ❌ Not recommended - any network issue = total outage
- Add 3rd node minimum

**For 3+ nodes across providers**:
- ✅ Can survive 1 network/node failure
- ⚠️ Requires careful network configuration
- ⚠️ Performance impact from latency
- ✅ Monitor replication lag closely
