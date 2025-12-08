# PostgreSQL Master-Slave Replication with PgPool for Docker Swarm

High-availability PostgreSQL setup with automatic failover using master-slave replication and PgPool orchestrator.

## Architecture

- **Primary Node (Node 1)**: Handles all writes and reads
- **Secondary Node (Node 2)**: Standby replica for reads and automatic failover
- **PgPool (Node 3)**: Connection pooling and automatic failover orchestrator
- **Overlay Network**: Secure communication between components

## Features

- ✅ Automatic failover with PgPool
- ✅ Streaming replication (async)
- ✅ Split-brain prevention with connectivity monitoring
- ✅ Connection pooling and load balancing
- ✅ Automatic recovery when network restores
- ✅ Lightweight orchestrator (512MB RAM on Node 3)

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
```

### 4. Deploy Stack

```bash
make deploy
```

### 5. Setup Replication User

```bash
make setup-replication
```

## Ports

- **5432**: PgPool (applications connect here)
- **5432** (node1): Primary (direct access)
- **5433** (node2): Secondary (direct access)
- **9999**: PgPool admin interface

## Connecting to the Database

### Via PgPool (Recommended)

```bash
psql -h <pgpool-host> -p 5432 -U postgres -d app_db
```

### Direct to Nodes

```bash
# Primary
psql -h <primary-host> -p 5432 -U postgres -d app_db

# Secondary (read-only)
psql -h <secondary-host> -p 5433 -U postgres -d app_db
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
make logs-pgpool
```

## Failover

PgPool automatically handles failover when Primary becomes unavailable. The Secondary is promoted and applications continue with minimal disruption.

## Split-Brain Prevention

Both nodes run connectivity monitors that check every 5 seconds:

**Primary**: Enters read-only mode if isolated from Secondary for 15s
**Secondary**: Waits for PgPool promotion if Primary is down

## Security

- Change all default passwords in `.env`
- Use Docker secrets for production
- Restrict network access
- Enable SSL/TLS for client connections

## License

This configuration is provided as-is for use in your infrastructure.
