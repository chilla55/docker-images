# pgpool-custom (Alpine)

Custom pgpool-II image based on Alpine Linux with SSL support and DNS resolution waiting.

## Features

- Environment-driven configuration
- SSL/TLS for backend connections
- Root CA certificate support
- Automatic DNS resolution waiting
- Health checks via pg_isready
- Auto-generated SSL certificates

## Environment Variables

**Backend Configuration:**
- `PGPOOL_BACKEND_NODES` - Comma-separated backends: `IDX:HOST:PORT` (e.g., `0:postgresql-primary:5432,1:postgresql-secondary:5432`)

**Authentication:**
- `PGPOOL_SR_CHECK_USER` - Streaming replication check user
- `PGPOOL_SR_CHECK_PASSWORD_FILE` - Path to replication password secret
- `PGPOOL_POSTGRES_USERNAME` - Database username for pool_passwd
- `PGPOOL_POSTGRES_PASSWORD_FILE` - Path to postgres password secret
- `PGPOOL_ADMIN_PASSWORD_FILE` - Path to admin password secret

**Behavior:**
- `PGPOOL_ENABLE_LOAD_BALANCING` - Enable load balancing (yes/no, default: yes)
- `PGPOOL_AUTO_FAILBACK` - Enable automatic failback (yes/no, default: yes)
- `PGPOOL_FAILOVER_ON_BACKEND_ERROR` - Failover on backend errors (yes/no, default: yes)
- `PGPOOL_NUM_INIT_CHILDREN` - Number of connection pools (default: 32)
- `PGPOOL_MAX_POOL` - Max connections per pool (default: 4)

## SSL Configuration

Automatically generates and signs certificates using root CA:
- Root CA: `/var/lib/postgresql/rootca/ca-cert.pem` (mounted read-only)
- Server cert: `/var/lib/postgresql/server.crt` (auto-generated)
- Server key: `/var/lib/postgresql/server.key` (auto-generated)

## Health Check

Uses `pg_isready -h localhost -p 5432` with 90-second start period.

## Build & Deploy

```bash
# Build
docker build -t ghcr.io/chilla55/pgpool-custom:latest .

# Push
docker push ghcr.io/chilla55/pgpool-custom:latest

# Deploy with PostgreSQL stack
docker stack deploy -c docker-compose.swarm.yml postgresql
```

## Notes

- Runs in foreground mode suitable for Docker Swarm
- Waits for backend DNS resolution before starting
- Configuration generated from templates at runtime
- Ports: 5432 (PostgreSQL), 9898 (PCP admin)

