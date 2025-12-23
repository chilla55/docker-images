# Pterodactyl Go-Proxy TCP V2 Client Implementation

## Overview

Successfully implemented TCP v2 protocol integration for Pterodactyl Panel to dynamically register with the go-proxy service registry. This enables automatic route registration, health checking, and graceful service management.

## Changes Made

### 1. Registry Client (`registry_client_v2.go`)

Copied the complete TCP v2 registry client from the orbat service to pterodactyl. This provides:

- **Automatic IP Detection**: Detects container IP on web-net (10.2.2.0/24)
- **Route Management**: Dynamic route registration with domains, paths, and priorities
- **Health Checks**: Configurable health check intervals and timeouts
- **Event System**: Event-driven architecture with handlers for:
  - Connection events (connected, disconnected)
  - Route operations (added, removed, updated)
  - Health check configuration
  - Configuration application
  - Maintenance mode
- **Keepalive**: Automatic connection keepalive with reconnection logic
- **Graceful Shutdown**: Clean disconnection and resource cleanup

### 2. Main Entry Point (`main.go`)

Enhanced the main entry point with registry integration:

**Added Global Variables:**
```go
var (
    registryClientV2 *RegistryClientV2
    routeID          string
    done             = make(chan os.Signal, 1)
)
```

**New Functions:**
- `registerWithProxy()`: Initializes registry client and registers routes
  - Connects to proxy on port 81 (TCP)
  - Registers service with metadata
  - Configures event handlers for lifecycle events
  - Adds route with domain, path, and backend URL
  - Sets health check (path: `/`, interval: `30s`, timeout: `5s`)
  - Enables compression
  - Applies configuration atomically

- `keepAliveLoop()`: Maintains persistent connection
  - Pings every 30 seconds
  - Auto-reconnects on failure
  - Runs in background goroutine

- `cleanup()`: Graceful shutdown
  - Closes registry connection
  - Sends proper shutdown command to proxy
  - Cleans up resources

**Service Type Handling:**
- Registry registration only occurs for the `caddy` service type
- Other services (php-fpm, queue, cron) continue without registry integration
- This ensures only the HTTP-facing component registers routes

### 3. Docker Compose Configuration (`docker-compose.swarm.yml`)

Updated the Caddy service environment variables:

```yaml
environment:
  # go-proxy registry v2
  REGISTRY_HOST: proxy            # Registry hostname
  REGISTRY_PORT: "81"             # TCP registry port
  SERVICE_NAME: pterodactyl       # Service identifier
  DOMAINS: gpanel.chilla55.de     # Comma-separated domain list
  ROUTE_PATH: /                   # URL path prefix
  PORT: "80"                      # Backend port
```

**Key Changes:**
- Removed obsolete `BACKEND_HOST` variable (now auto-detected via IP)
- Added `SERVICE_NAME` for better service identification
- Simplified configuration for v2 protocol
- Environment variables moved above deploy section for better organization

### 4. Go Module (`go.mod`)

No changes required - the registry client uses only Go standard library packages:
- `bufio` - Buffered I/O for TCP communication
- `encoding/json` - JSON marshaling/unmarshaling
- `fmt` - Formatting and printing
- `net` - TCP networking
- `strings` - String manipulation
- `sync` - Synchronization primitives (mutex for event handlers)
- `time` - Time operations and timeouts

## How It Works

### Startup Flow

1. **Container Start**: Caddy service container starts
2. **IP Detection**: Registry client auto-detects container IP on web-net
3. **Registration**: Connects to `proxy:81` via TCP
4. **Session Creation**: Receives session ID from registry
5. **Route Configuration**: Stages route with domains and backend URL
6. **Health Check Setup**: Configures health monitoring
7. **Config Application**: Atomically applies all staged changes
8. **Service Start**: Caddy web server starts and serves traffic
9. **Keepalive**: Background goroutine maintains connection

### Runtime Behavior

- **Health Monitoring**: Proxy checks `/` endpoint every 30 seconds
- **Connection Maintenance**: Client pings registry every 30 seconds
- **Auto-Recovery**: Reconnects automatically on connection loss
- **Event Logging**: Logs all registry events for observability

### Shutdown Flow

1. **Signal Received**: SIGTERM or SIGINT caught
2. **Cleanup Triggered**: `cleanup()` function called via defer
3. **Shutdown Command**: Sends `CLIENT_SHUTDOWN` to registry
4. **Connection Close**: Closes TCP connection
5. **Service Exit**: Container stops gracefully

## Benefits

### Dynamic Service Discovery
- No static configuration files to manage
- Routes automatically registered on startup
- Instant updates when services start/stop

### Zero-Downtime Operations
- Graceful maintenance mode support (for future use)
- Health-based traffic routing
- Circuit breaker integration (configured on proxy)

### Production-Ready
- Automatic reconnection on failures
- Event-driven architecture for flexibility
- Comprehensive error handling
- Detailed logging for debugging

### Operational Excellence
- Session-based tracking for observability
- Metadata support for service information
- Stats and monitoring integration
- Graceful shutdown prevents connection leaks

## Testing

To test the implementation:

```bash
# Build and deploy the updated stack
cd /mnt/hdd8tb/__________Docker/docker-images/petrodactyl
make build
docker stack deploy -c docker-compose.swarm.yml pterodactyl

# Check Caddy logs for registry registration
docker service logs pterodactyl_caddy -f

# Look for these log messages:
# [client] Detected container IP: 10.2.2.X
# [client] Registered with session ID: <session_id>
# ✓ Connected to registry - Session: <session_id>, IP: 10.2.2.X
# ✓ Route registered - ID: <route_id>
# ✓ Health check configured for route <route_id>
# ✓ Configuration applied and active on proxy

# Check proxy logs to see the registration
docker service logs proxy_proxy -f

# Access Pterodactyl via the proxy
curl -H "Host: gpanel.chilla55.de" http://proxy/
```

## Configuration Options

All configuration is done via environment variables in docker-compose.swarm.yml:

| Variable | Default | Description |
|----------|---------|-------------|
| `REGISTRY_HOST` | `proxy` | Hostname of go-proxy registry |
| `REGISTRY_PORT` | `81` | TCP port for registry communication |
| `SERVICE_NAME` | `pterodactyl` | Service identifier in registry |
| `DOMAINS` | `gpanel.chilla55.de` | Comma-separated list of domains |
| `ROUTE_PATH` | `/` | URL path prefix for routing |
| `PORT` | `80` | Backend HTTP port |

## Future Enhancements

The implementation supports additional features that can be enabled:

1. **Maintenance Mode**: 
   - Call `registryClientV2.MaintenanceEnter("ALL")` to enter maintenance
   - Call `registryClientV2.MaintenanceExit("ALL")` to exit

2. **Circuit Breaker**: Already configured on proxy side

3. **Rate Limiting**: Can be configured per-route if needed

4. **Custom Headers**: Can inject response headers dynamically

5. **Drain Mode**: Graceful traffic draining for updates

## Troubleshooting

### Registry Connection Fails
- Check if proxy service is running: `docker service ls | grep proxy`
- Verify network connectivity: Container must be on `web-net`
- Check proxy logs for errors: `docker service logs proxy_proxy`

### Routes Not Working
- Verify domain in DOMAINS matches request Host header
- Check route priority if multiple overlapping routes exist
- Review proxy routing logs for request handling

### Health Checks Failing
- Ensure Caddy is responding on port 80
- Check if `/` endpoint is accessible
- Verify PHP-FPM is running and healthy

## Compatibility

- **Go Version**: 1.21+
- **Protocol**: TCP Registry V2
- **Networks**: Requires `web-net` overlay network
- **Dependencies**: Go standard library only (no external packages)

## Summary

Pterodactyl Panel is now fully integrated with the go-proxy TCP v2 service registry, providing:

✅ Automatic service registration on startup
✅ Dynamic route management
✅ Health monitoring and circuit breaking
✅ Graceful shutdown and cleanup
✅ Event-driven architecture for extensibility
✅ Production-ready error handling and reconnection logic

The implementation follows the same proven pattern as the orbat service, ensuring consistency and reliability across the infrastructure.
