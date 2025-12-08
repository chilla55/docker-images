# MariaDB Master-Slave Replication with MaxScale for Docker Swarm

High-availability MariaDB setup with automatic failover using master-slave replication and MaxScale orchestrator.

## Architecture

- **Primary Node (Node 1)**: Handles all writes and reads
- **Secondary Node (Node 2)**: Standby replica for reads and automatic failover
- **MaxScale (Node 3)**: Orchestrator for automatic failover and query routing
- **Overlay Network**: Secure communication between components

## Features

- ✅ Automatic failover (10-15s downtime)
- ✅ Async replication (better for WAN/multi-provider)
- ✅ Split-brain prevention with cooperative monitoring
- ✅ Read-write splitting
- ✅ Automatic recovery when network restores
- ✅ Lightweight orchestrator (512MB RAM on Node 3)
- ✅ REST API for cluster management

## Prerequisites

- Docker Swarm cluster (minimum 2 worker nodes + 1 manager)
- Node labels configured for placement
- Environment variables configured

## Quick Start

### 1. Configure Environment

```bash
cp .env.example .env
# Edit .env with your passwords
```

### 2. Build Image

```bash
make build
```

### 3. Label Swarm Nodes

```bash
make label-nodes
# You'll be prompted to enter node names for Primary, Secondary, and MaxScale
```

### 4. Deploy Stack

```bash
make deploy
```

### 5. Setup Replication

```bash
make setup-replication
# Follow the instructions to configure replication
```

### 6. Create MaxScale User

```bash
make create-maxscale-user
```

## Ports

- **3306**: Primary (direct access)
- **3307**: Secondary (direct access)
- **4006**: MaxScale Read-Write Router (recommended for applications)
- **4008**: MaxScale Read-Only Router
- **8989**: MaxScale REST API

## Connecting to the Database

### Via MaxScale (Recommended)

```bash
mysql -h <maxscale-host> -P 4006 -u app_user -p
```

### Direct to Nodes

```bash
# Primary
mysql -h <primary-host> -P 3306 -u root -p

# Secondary
mysql -h <secondary-host> -P 3307 -u root -p
```

## Monitoring

### Check Status

```bash
make status
```

### View Logs

```bash
make logs-primary
make logs-secondary
make logs-maxscale
```

### MaxScale REST API

```bash
curl http://<maxscale-host>:8989/v1/servers
```

## Failover

### Automatic Failover

MaxScale automatically promotes Secondary when:
- Primary is unreachable for 15 seconds
- Secondary can reach MaxScale
- Verified from multiple perspectives

### Manual Failover

```bash
# Immediate failover
make failover

# Graceful switchover
make switchover
```

## Split-Brain Prevention

Both nodes run connectivity monitors that check every 5 seconds:

**Primary checks**:
- Can reach Secondary?
- Can reach MaxScale?
- If neither reachable for 15s → enters read-only mode

**Secondary checks**:
- Can reach Primary?
- Can reach MaxScale?
- If Primary down + MaxScale up → ready for promotion

This ensures only one active writer at any time.

## Troubleshooting

### Replication Not Running

```bash
# On Secondary
docker exec <secondary> mysql -u root -p -e "SHOW SLAVE STATUS\G"

# Restart replication
docker exec <secondary> mysql -u root -p -e "STOP SLAVE; START SLAVE;"
```

### MaxScale Can't Connect

```bash
# Re-create MaxScale user
make create-maxscale-user

# Check connectivity
docker exec <maxscale> maxctrl list servers
```

### Check Connectivity Monitor

```bash
# View logs for connectivity checks
make logs-primary | grep "ISOLATED\|Connectivity"
make logs-secondary | grep "ISOLATED\|Connectivity"
```

## Architecture Documentation

See `MASTER_SLAVE_ARCHITECTURE.md` for detailed architecture, failover scenarios, and resource planning.

## Performance Tuning

Key configuration files:
- `mariadb-master.cnf`: Primary configuration
- `mariadb-slave.cnf`: Secondary configuration
- `maxscale.cnf`: MaxScale routing and failover settings

## Security

- Change all default passwords in `.env`
- Use Docker secrets for production
- Restrict network access
- Enable SSL/TLS for client connections
- Regular security updates

## License

This configuration is provided as-is for use in your infrastructure.
