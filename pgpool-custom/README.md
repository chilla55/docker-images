# pgpool-custom (Alpine)

Custom Pgpool-II image based on Alpine, configured via environment variables to be compatible with existing Bitnami-style compose settings.

## Environment variables
- `PGPOOL_BACKEND_NODES`: comma-separated `IDX:HOST:PORT` entries, e.g. `0:postgresql-primary:5432,1:postgresql-secondary:5432`.
- `PGPOOL_SR_CHECK_USER`, `PGPOOL_SR_CHECK_PASSWORD`: user/password for streaming replication checks.
- `PGPOOL_POSTGRES_USERNAME`, `PGPOOL_POSTGRES_PASSWORD`: database user/password for client authentication; renders `pool_passwd`.
- `PGPOOL_ENABLE_LOAD_BALANCING`: `yes`/`no`.
- `PGPOOL_AUTO_FAILBACK`: `yes`/`no`.
- `PGPOOL_FAILOVER_ON_BACKEND_ERROR`: `yes`/`no`.
- `PGPOOL_NUM_INIT_CHILDREN`: default 32.
- `PGPOOL_MAX_POOL`: default 4.

## Notes
- Runs in foreground (`pgpool -n`) suitable for Docker/Swarm.
- No host port exposure required; uses overlay networks.
- Configuration files generated at container start from templates.

## Build & Push
```bash
# Build locally
docker build -t ghcr.io/chilla55/pgpool-custom:latest -f pgpool-custom/Dockerfile pgpool-custom

# Login to GHCR (if not already)
echo "$GHCR_TOKEN" | docker login ghcr.io -u chilla55 --password-stdin

# Push
docker push ghcr.io/chilla55/pgpool-custom:latest
```
