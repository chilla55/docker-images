# Redis for Docker Swarm

Redis 7 Alpine-based image optimized for Docker Swarm deployments.

## Features

- **Redis 7** on Alpine Linux (minimal footprint)
- **Persistence**: RDB snapshots + AOF (Append Only File)
- **Password authentication** via environment variable
- **Health checks** built-in
- **Memory management**: LRU eviction with 256MB limit
- **Optimized** for Pterodactyl Panel and general caching

## Quick Start

### Build Image

```bash
make build
```

### Test Image

```bash
make test
```

### Deploy to Swarm

1. **Label your node:**
```bash
docker node update --label-add redis.node=node1 <node-name>
```

2. **Create network:**
```bash
docker network create --driver overlay --attachable net_redis_pterodactyl
```

3. **Set environment variables:**
```bash
export REDIS_PASSWORD="your-secure-password"
export VERSION="1.0.0"
```

4. **Deploy stack:**
```bash
docker stack deploy -c docker-compose.swarm.yml redis
```

## Configuration

### Environment Variables

- `REDIS_PASSWORD`: Password for Redis authentication (required)
- `VERSION`: Image version tag (default: latest)
- `DOCKER_REGISTRY`: Container registry URL

### Persistence Settings

- **RDB Snapshots**:
  - Every 900s if 1+ keys changed
  - Every 300s if 10+ keys changed
  - Every 60s if 10000+ keys changed

- **AOF**: Enabled with `everysec` fsync policy

### Memory

- **Max memory**: 256MB
- **Eviction policy**: allkeys-lru (least recently used)

## Connecting

```bash
# From application
redis-cli -h redis -p 6379 -a ${REDIS_PASSWORD}

# Test connection
redis-cli -a ${REDIS_PASSWORD} ping
```

## Resource Usage

- **CPU**: 0.5-1 core
- **Memory**: 256-512MB
- **Storage**: Depends on dataset size

## Health Check

Built-in healthcheck pings Redis every 30 seconds.

## Backup

Data persists in the `redis-data` volume at `/data`:

```bash
# Backup
docker run --rm -v redis_redis-data:/data -v $(pwd):/backup alpine tar czf /backup/redis-backup.tar.gz -C /data .

# Restore
docker run --rm -v redis_redis-data:/data -v $(pwd):/backup alpine tar xzf /backup/redis-backup.tar.gz -C /data
```
