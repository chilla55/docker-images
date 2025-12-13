# PostgreSQL High Availability Cluster

PostgreSQL 16 with streaming replication, SSL/TLS encryption, and pgpool-II for automatic failover.

## Architecture

- **Primary** (srv1): Read-write operations with streaming replication
- **Secondary** (mail): Hot standby replica
- **pgpool** (srv2): Connection pooling, load balancing, automatic failover/failback
- **Network**: 10.2.2.0/24 overlay network (postgres-net)
- **Security**: Root CA-based SSL certificate verification, Docker Swarm secrets

## Features

- ✅ Streaming replication with SSL encryption
- ✅ Automatic failover and failback via pgpool
- ✅ Connectivity monitoring with read-only protection
- ✅ Load balancing for read queries
- ✅ Health checks on all components
- ✅ Certificate auto-generation and signing

## Connection

**Via pgpool (recommended):**
```bash
psql -h pgpool -p 5432 -U postgres -d app_db
```

**Direct connections:**
- Primary: `postgresql-primary:5432` (read-write)
- Secondary: `postgresql-secondary:5432` (read-only)

## SSL Configuration

All connections use SSL with verify-ca mode:
- Root CA: `/mnt/storagebox/rootca/ca-cert.pem`
- Server certificates auto-generated and signed on first start
- Mounted read-only with rslave propagation

## Secrets

Required Docker Swarm secrets:
- `postgres_password` - PostgreSQL superuser password
- `postgres_replication_password` - Replication user password
- `pgpool_admin_password` - pgpool admin password

## Deployment

```bash
# Build and push images
docker build -t ghcr.io/chilla55/postgresql-replication:latest .
docker push ghcr.io/chilla55/postgresql-replication:latest

# Deploy stack
docker stack deploy -c docker-compose.swarm.yml postgresql
```

## Monitoring

```bash
# Check service status
docker stack ps postgresql

# View replication status on primary
docker exec $(docker ps -q -f name=postgresql_postgresql-primary) \
  psql -U postgres -c "SELECT * FROM pg_stat_replication;"

# Check pgpool node status
docker service logs postgresql_pgpool | grep "node status"
```

## Configuration Files

- `docker-compose.swarm.yml` - Swarm orchestration
- `pg_hba.conf` - Client authentication (hostssl required)
- `postgresql-primary.conf` - Primary server config with SSL
- `postgresql-replica.conf` - Replica server config with SSL
- `scripts/entrypoint.sh` - Startup and SSL cert generation
- `scripts/check-connectivity.sh` - Cluster health monitoring
- `scripts/healthcheck.sh` - Container health validation
- `scripts/init-replication-user.sh` - Replication user creation
