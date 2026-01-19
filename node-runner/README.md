# Node Runner Container

A minimal Node.js runner image with a Go entrypoint that:
- Executes a command passed via `ENTRY_COMMAND` (no app code baked into the image)
- Optionally runs `npm install`/`npm run build` on startup
- Registers itself with the go-proxy registry-client v2 (similar to `orbat`)
- Works with bind-mounted app code/data via volumes
- Can unpack a provided ZIP file from a host-mounted path before starting

## Build

```bash
docker build -t node-runner:local ./node-runner
```

## Run (standalone)

```bash
docker run --rm -it \
	-v /path/to/your/app:/workspace \
	-v /path/to/bundle.zip:/tmp/bundle.zip:ro \
	-e ENTRY_COMMAND="npm start" \
	-e ENTRY_INSTALL=1 \
	-e ZIP_PATH="/tmp/bundle.zip" \
	-e ZIP_STRIP_COMPONENTS=1 \
	-e ZIP_CLEAN=1 \
	-e DOMAINS="app.example.com" \
	-e ROUTE_PATH="/" \
	node-runner:local
```

## docker-compose example (behind go-proxy)

```yaml
services:
	nodeapp:
		image: node-runner:local
		environment:
			ENTRY_COMMAND: "node server.js"
			ENTRY_INSTALL: "1"   # npm install if package.json is present
			ENTRY_BUILD: "0"     # set to 1 to run npm run build when lockfile exists
			APP_DIR: /workspace   # override if you mount elsewhere
			APP_PORT: "30000"     # port your app listens on
			SERVICE_NAME: "nodeapp"
			DOMAINS: "nodeapp.example.com"
			ROUTE_PATH: "/"
			HEALTH_PATH: "/"
			REGISTRY_HOST: "proxy"
			REGISTRY_PORT: "81"
			ZIP_PATH: "/workspace/bundle.zip"   # host-mounted zip file
			ZIP_STRIP_COMPONENTS: "1"            # drop leading dirs inside the zip
			ZIP_CLEAN: "1"                       # wipe APP_DIR before extract
			ENABLE_WEBSOCKET: "true"             # enable WebSocket upgrades
			BACKEND_HTTP2: "false"               # force HTTP/1.1 upstream
			PRESERVE_HOST: "true"                # keep original Host header
		volumes:
			- /srv/nodeapp:/workspace
			- /srv/bundles/bundle.zip:/workspace/bundle.zip:ro
		networks:
			- web                 # join the same network go-proxy uses

networks:
	web:
		external: true
```

## Environment variables
- `ENTRY_COMMAND` (required): Command string executed via `sh -c`.
- `APP_DIR` (default `/workspace`): Where the app code is mounted.
- `ENTRY_INSTALL` (default `1`): When `1` and `package.json` exists, run `npm install` on start.
- `ENTRY_BUILD` (default `0`): When `1` and lockfile exists, run `npm run build` after install.
- `NODE_ENV` (default `production`): Passed through to Node processes.
- `APP_PORT` (default `30000`): Backend port exposed to go-proxy.
- `SERVICE_NAME` (default `nodeapp`): Name used when registering with go-proxy.
- `DOMAINS` (default `example.com`): Comma-separated list for routing.
- `ROUTE_PATH` (default `/`): Path prefix for the route.
- `HEALTH_PATH` (default `/`): Health endpoint for registry checks.
- `ENABLE_WEBSOCKET` (default `true`): Enable WebSocket upgrade handling.
- `BACKEND_HTTP2` (default `false`): Use HTTP/2 to backend when true; default off for WS.
- `PRESERVE_HOST` (default `true`): Keep original Host header to backend.
- `REGISTRY_HOST` / `REGISTRY_PORT` (defaults `proxy` / `81`): go-proxy registry endpoint.
- `ENABLE_REGISTRY` (default `true`): Set to `false` to skip registration.
- `WAIT_FOR_PORT` (default `true`): Wait for `APP_PORT` to accept TCP before registering.
- `PORT_WAIT_TIMEOUT` (default `30s`): Timeout for waiting on `APP_PORT`.
- `ZIP_PATH` (optional): Absolute path (inside container) to a host-mounted zip file to unpack before running.
- `ZIP_STRIP_COMPONENTS` (default `1`): How many leading path components to strip from entries when extracting.
- `ZIP_CLEAN` (default `1`): When true, empties `APP_DIR` before extracting the zip.

## Notes
- Mount your app (including `package.json`) via a volume; it is not baked into the image or the repo.
- For go-proxy, add a site pointing to `http://nodeapp:30000` (or your app port) once this container is on the shared `web` network.
- For Docker Swarm, set `REGISTRY_HOST` to the go-proxy service DNS (e.g., `tasks.go-proxy_proxy`) and attach to the same overlay network.
