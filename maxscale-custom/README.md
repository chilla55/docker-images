# Custom MaxScale Image

This image is a minimal wrapper around `mariadb/maxscale:latest` that installs `procps-ng` (needed by Monit scripts for `pgrep/ps`) and runs MaxScale as the non-root `maxscale` user.

## Build & Push (local)
```
docker build -t ghcr.io/${USER}/maxscale:latest ./maxscale-custom
docker push ghcr.io/${USER}/maxscale:latest
```

## Usage in Docker Swarm / Compose
Update your service to reference the new image:
```
image: ghcr.io/<owner>/maxscale:latest
```

Ports and configuration remain the same as upstream `mariadb/maxscale`.
