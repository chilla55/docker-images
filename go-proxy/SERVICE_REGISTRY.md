# Service Registry Guide

Complete guide for integrating applications with the go-proxy service registry for dynamic route management.

## Overview

The service registry allows backend applications to dynamically register and deregister routes without editing configuration files or restarting the proxy. This is ideal for:

- **Microservices** - Services register themselves on startup
- **Auto-scaling** - New instances automatically get routed
- **Blue-Green Deployments** - Switch traffic programmatically
- **Canary Releases** - Gradually roll out new versions
- **Maintenance Mode** - Temporarily disable routes

**Registry Port:** `81` (TCP protocol)

---

## TCP Protocol

The service registry uses a **TCP-based protocol** on port 81 with persistent connections and session management.

**Why TCP?**
- Persistent connections enable automatic cleanup when services die
- Session-based connection tracking for reconnection support
- Real-time maintenance mode coordination
- Lower overhead than HTTP for high-frequency updates
- Connection monitoring detects crashed services automatically

**Port:** `81`

### Protocol Format

Commands are newline-terminated strings with pipe-separated fields:

```
COMMAND|field1|field2|...\n
```

All responses are also newline-terminated.

---

## Protocol v2

This protocol provides comprehensive service registration, route management, and operational control with staged configuration, observability, and production-grade resilience features.

### General Conventions
- All commands include `session_id` after registration.
- All commands receive an immediate acknowledgement response.
- `route_id` identifies a specific route returned from `ROUTE_ADD`.
- `target` is either a specific `route_id` or `ALL` for global settings.
- Backend identifiers are full connection strings with scheme (e.g., `http://orbat:3000`, `https://api:9443`, `ws://chat:8080`).
- The proxy responds with `OK`, `ACK`, specific `*_OK` codes, or `ERROR|message`.
- **Configuration is staged**: All `ROUTE_*`, `HEADERS_SET`, `OPTIONS_SET`, `HEALTH_SET`, and `RATELIMIT_SET` commands stage changes without applying them immediately.
- Use `CONFIG_VALIDATE` to check for errors, then `CONFIG_APPLY` to atomically apply all staged changes.
- `CONFIG_APPLY` returns detailed error messages if validation fails.
- Route priority: routes are matched by longest prefix first; use `priority` field to override (higher = matched first).

### REGISTER
Obtain a `session_id` and establish a persistent connection.

Format:
```
REGISTER|service_name|instance_name|maintenance_port|metadata
```

Parameters:
- `service_name`: logical service name (e.g., `orbat`).
- `instance_name`: internal identifier (e.g., container name).
- `maintenance_port`: port for maintenance/health page.
- `metadata`: optional JSON object with version, build, tags (e.g., `{"version":"1.2.3","build":"abc123"}`).

Response:
```
ACK|session_id
```

Notes:
- The server enables TCP keepalive (30s period) on this connection to detect crashes.
- Metadata is used for observability and logging only.

### ROUTE_ADD
Stage a backend route for addition; returns a `route_id` for future updates.

Format:
```
ROUTE_ADD|session_id|domains|path|backend_url|priority
```

Parameters:
- `domains`: comma-separated list (e.g., `orbat.chilla55.de,www.orbat.chilla55.de`).
- `path`: URL path prefix (e.g., `/`, `/api`).
- `backend_url`: full connection string with scheme (e.g., `http://orbat:3000`, `https://api:9443`, `ws://chat:8080`).
- `priority`: integer priority (higher = matched first); use `0` for default (longest prefix match).

Response:
```
ROUTE_OK|route_id
```

Notes:
- Route is staged; call `CONFIG_APPLY` to activate.
- Default priority is 0; routes with same priority use longest prefix matching.

### ROUTE_ADD_BULK
Stage multiple backend routes in a single command.

Format:
```
ROUTE_ADD_BULK|session_id|json_array
```

Parameters:
- `json_array`: JSON array of route objects with fields: `domains` (array), `path`, `backend_url`, `priority`.

Example:
```
ROUTE_ADD_BULK|sess123|[{"domains":["api.example.com"],"path":"/v1","backend_url":"http://api:9000","priority":0},{"domains":["api.example.com"],"path":"/v2","backend_url":"http://api-v2:9001","priority":10}]
```

Response:
```
ROUTE_BULK_OK|json_array
```

Example response:
```
ROUTE_BULK_OK|[{"route_id":"r1","status":"ok"},{"route_id":"r2","status":"ok"}]
```

Notes:
- All routes are staged; call `CONFIG_APPLY` to activate.
- If any route fails validation, entire command fails and no routes are staged.
- More efficient than multiple `ROUTE_ADD` commands for services with many routes.

### ROUTE_UPDATE
Stage an update to an existing route without removing and re-adding.

Format:
```
ROUTE_UPDATE|session_id|route_id|field|value
```

Parameters:
- `route_id`: target route to update.
- `field`: one of `backend_url`, `priority`, `domains`, `path`.
- `value`: new value (for `domains`, use comma-separated list).

Response:
```
ROUTE_OK
```
or
```
ERROR|reason
```

Notes:
- Changes are staged; call `CONFIG_APPLY` to activate.
- More efficient than `ROUTE_REMOVE` + `ROUTE_ADD` for single field updates.

### ROUTE_REMOVE
Stage a route for removal.

Format:
```
ROUTE_REMOVE|session_id|route_id
```

Response:
```
ROUTE_OK
```

Notes:
- Removal is staged; call `CONFIG_APPLY` to activate.
- Attempting to remove a non-existent `route_id` returns `ERROR|route not found`.

### ROUTE_LIST
List all routes for this session (active and staged).

Format:
```
ROUTE_LIST|session_id
```

Response:
```
ROUTE_LIST_OK|json_array
```

Example response:
```
ROUTE_LIST_OK|[{"route_id":"r1","domains":["orbat.chilla55.de"],"path":"/","backend":"http://orbat:3000","priority":0,"status":"active"},{"route_id":"r2","domains":["api.chilla55.de"],"path":"/v2","backend":"http://api:9000","priority":10,"status":"staged"}]
```

Notes:
- `status` is either `active` (live), `staged` (pending apply), or `pending_removal`.

### HEADERS_SET
Stage header changes globally or for a specific route.

Format:
```
HEADERS_SET|session_id|target|header_name|header_value
```

Parameters:
- `target`: `ALL` for all routes or a specific `route_id`.
- `header_name`: HTTP header name (e.g., `X-Service-Version`).
- `header_value`: Header value.

Response:
```
HEADERS_OK
```
or
```
ERROR|reason
```

Notes:
- Changes are staged; call `CONFIG_APPLY` to activate.

### HEADERS_REMOVE
Stage removal of a header globally or for a specific route.

Format:
```
HEADERS_REMOVE|session_id|target|header_name
```

Parameters:
- `target`: `ALL` for all routes or a specific `route_id`.
- `header_name`: HTTP header name to remove.

Response:
```
HEADERS_OK
```
or
```
ERROR|reason
```

Notes:
- Changes are staged; call `CONFIG_APPLY` to activate.

### OPTIONS_SET
Stage option changes globally or per-route.

Format:
```
OPTIONS_SET|session_id|target|key|value
```

Parameters:
- `target`: `ALL` for all routes or a specific `route_id`.
- `key`: e.g., `timeout`, `health_check_interval`, `compression`, `websocket`, `http2`, `http3`.
- `value`: string; server parses type per key.

Response:
```
OPTIONS_OK
```
or
```
ERROR|reason
```

Notes:
- Changes are staged; call `CONFIG_APPLY` to activate.

### OPTIONS_REMOVE
Stage removal of an option, resetting it to default.

Format:
```
OPTIONS_REMOVE|session_id|target|key
```

Parameters:
- `target`: `ALL` for all routes or a specific `route_id`.
- `key`: option key to reset to default.

Response:
```
OPTIONS_OK
```
or
```
ERROR|reason
```

Notes:
- Changes are staged; call `CONFIG_APPLY` to activate.

### HEALTH_SET
Stage custom health check configuration for a route.

Format:
```
HEALTH_SET|session_id|route_id|path|interval|timeout
```

Parameters:
- `route_id`: target route.
- `path`: health check path (e.g., `/health`, `/api/status`).
- `interval`: check interval (e.g., `30s`, `1m`).
- `timeout`: check timeout (e.g., `5s`, `10s`).

Response:
```
HEALTH_OK
```
or
```
ERROR|reason
```

Notes:
- Changes are staged; call `CONFIG_APPLY` to activate.
- Default health check uses backend root path with 30s interval and 5s timeout.

### RATELIMIT_SET
Stage rate limiting for a route.

Format:
```
RATELIMIT_SET|session_id|route_id|requests|window
```

Parameters:
- `route_id`: target route (use `ALL` for global rate limit).
- `requests`: maximum requests allowed.
- `window`: time window (e.g., `1s`, `1m`, `1h`).

Response:
```
RATELIMIT_OK
```
or
```
ERROR|reason
```

Notes:
- Changes are staged; call `CONFIG_APPLY` to activate.
- Rate limits are per source IP address.

### CONFIG_VALIDATE
Validate all staged configuration changes without applying.

Format:
```
CONFIG_VALIDATE|session_id
```

Response:
```
OK
```
or
```
ERROR|detailed_error_message
```

Notes:
- Validates routes, headers, options, health checks, and rate limits.
- Returns specific errors (e.g., `ERROR|route r2: invalid backend URL format`).

### CONFIG_APPLY
Atomically apply all staged configuration changes.

Format:
```
CONFIG_APPLY|session_id
```

Response:
```
OK
```
or
```
ERROR|detailed_error_message
```

Notes:
- Automatically validates before applying; if validation fails, returns detailed error and nothing is applied.
- On success, all staged changes become active and staging area is cleared.
- This is an atomic operation: either all changes apply or none do.

### CONFIG_ROLLBACK
Discard all staged configuration changes without applying.

Format:
```
CONFIG_ROLLBACK|session_id
```

Response:
```
OK
```

Notes:
- Clears all staged routes, headers, options, health checks, and rate limits.
- Active configuration remains unchanged.
- Useful for error recovery or abandoning incomplete configurations.

### CONFIG_DIFF
Get a summary of staged changes compared to active configuration.

Format:
```
CONFIG_DIFF|session_id
```

Response:
```
DIFF_OK|json_object
```

Example response:
```
DIFF_OK|{"routes":{"added":["r3"],"removed":["r1"],"modified":["r2"]},"headers":{"added":2,"removed":1},"options":{"modified":3}}
```

Notes:
- Shows what will change when `CONFIG_APPLY` is called.
- Useful for reviewing changes before applying.

### CONFIG_APPLY_PARTIAL
Apply only specific types of staged changes.

Format:
```
CONFIG_APPLY_PARTIAL|session_id|scope
```

Parameters:
- `scope`: comma-separated list of types: `routes`, `headers`, `options`, `health`, `ratelimit`.

Example:
```
CONFIG_APPLY_PARTIAL|sess123|routes,headers
```

Response:
```
OK
```
or
```
ERROR|detailed_error_message
```

Notes:
- Only applies changes of specified types; other staged changes remain.
- Useful for applying route changes without affecting other configuration.

### STATS_GET
Query request statistics and metrics for routes.

Format:
```
STATS_GET|session_id|route_id
```

Parameters:
- `route_id`: specific route or `ALL` for all routes in this session.

Response:
```
STATS_OK|json_object
```

Example response:
```
STATS_OK|{"route_id":"r1","requests":12453,"errors":23,"avg_latency_ms":45,"p95_latency_ms":120,"p99_latency_ms":250,"bytes_sent":5242880,"bytes_received":1048576,"status_codes":{"200":12400,"404":30,"500":23}}
```

Notes:
- Statistics are real-time and include request counts, error rates, latency percentiles, bandwidth, and status code distribution.
- Useful for observability and auto-scaling decisions.

### PING
Test connection liveness at application level.

Format:
```
PING|session_id
```

Response:
```
PONG
```

Notes:
- TCP keepalive handles OS-level detection; `PING` detects application-level hangs.
- Optional; use if you need explicit heartbeat verification.

### SESSION_INFO
Get current session information and statistics.

Format:
```
SESSION_INFO|session_id
```

Response:
```
SESSION_OK|json_object
```

Example response:
```
SESSION_OK|{"session_id":"orbat-3000-1734532800","service_name":"orbat","instance_name":"orbat.1.abc123","connected_at":"2024-12-20T10:30:00Z","uptime_seconds":3600,"routes_active":3,"routes_staged":1,"last_apply":"2024-12-20T11:15:00Z","metadata":{"version":"1.2.3"}}
```

Notes:
- Useful for debugging and monitoring.
- Shows active and staged route counts, last apply time, and session metadata.

### DRAIN_START
Gracefully reduce traffic to this service over a specified duration.

Format:
```
DRAIN_START|session_id|duration
```

Parameters:
- `duration`: drain period (e.g., `30s`, `2m`, `5m`).

Response:
```
DRAIN_OK|completion_time
```

Example response:
```
DRAIN_OK|2024-12-20T11:35:00Z
```

Notes:
- Traffic is gradually reduced using weighted load balancing.
- After drain completes, service can enter maintenance or shutdown.
- Use `DRAIN_STATUS` to monitor progress.
- Better than immediate maintenance for zero-downtime deployments.

### DRAIN_STATUS
Check drain operation progress.

Format:
```
DRAIN_STATUS|session_id
```

Response:
```
DRAIN_STATUS_OK|json_object
```

Example response:
```
DRAIN_STATUS_OK|{"active":true,"started_at":"2024-12-20T11:30:00Z","duration_seconds":120,"elapsed_seconds":45,"remaining_seconds":75,"traffic_percent":62}
```

Notes:
- `traffic_percent` shows current traffic percentage (100 = full, 0 = drained).
- Returns `ERROR|no drain in progress` if not draining.

### DRAIN_CANCEL
Cancel an active drain operation.

Format:
```
DRAIN_CANCEL|session_id
```

Response:
```
DRAIN_OK
```

Notes:
- Immediately restores service to full traffic.
- Returns `ERROR|no drain in progress` if not draining.

### MAINT_ENTER
Enter maintenance mode for all routes or specific routes; proxy serves the maintenance page from the supplied backend URL.

Format:
```
MAINT_ENTER|session_id|target|backend_url
```

Parameters:
- `target`: `ALL` for all routes, or comma-separated `route_id` list (e.g., `r1,r3,r5`).
- `backend_url`: full connection string to maintenance server (e.g., `http://orbat:3001`).

Response (immediate acknowledgement):
```
ACK
```

Event (when maintenance is active):
```
MAINT_OK|target
```

Example:
```
MAINT_ENTER|sess123|r2,r3|http://orbat:3001
→ ACK
→ MAINT_OK|r2,r3
```

Client should acknowledge the event:
```
ACK
```

Notes:
- Route-specific maintenance allows partial updates without affecting other routes.
- Useful for feature-specific deployments or multi-path services.

### MAINT_EXIT
Exit maintenance mode for all routes or specific routes.

Format:
```
MAINT_EXIT|session_id|target
```

Parameters:
- `target`: `ALL` for all routes in maintenance, or comma-separated `route_id` list.

Response (immediate acknowledgement):
```
ACK
```

Event (when maintenance is fully exited):
```
MAINT_OK|target
```

Example:
```
MAINT_EXIT|sess123|r2
→ ACK
→ MAINT_OK|r2
```

Notes:
- Can exit maintenance for specific routes while others remain in maintenance.
- Use `MAINT_STATUS` to check which routes are in maintenance.

### MAINT_STATUS
Check maintenance status for routes in this session.

Format:
```
MAINT_STATUS|session_id
```

Response:
```
MAINT_STATUS_OK|json_object
```

Example response:
```
MAINT_STATUS_OK|{"in_maintenance":["r2","r3"],"maintenance_backend":"http://orbat:3001","entered_at":"2024-12-20T11:30:00Z"}
```

Notes:
- Shows which routes are currently in maintenance mode.
- Returns empty array if no routes are in maintenance.

### SUBSCRIBE
Subscribe to proxy events for this session.

Format:
```
SUBSCRIBE|session_id|event_type
```

Parameters:
- `event_type`: one of `cert_renewed`, `global_config_changed`, `backend_health_changed`, `all`.

Response:
```
SUBSCRIBE_OK
```

Event examples:
```
EVENT|cert_renewed|{"domain":"chilla55.de","expiry":"2025-03-20T00:00:00Z"}
EVENT|global_config_changed|{"reason":"admin_update"}
EVENT|backend_health_changed|{"route_id":"r1","healthy":false,"reason":"connection_timeout"}
```

Notes:
- Events are sent asynchronously on the same TCP connection.
- Client should handle events in addition to command responses.

### UNSUBSCRIBE
Unsubscribe from proxy events.

Format:
```
UNSUBSCRIBE|session_id|event_type
```

Parameters:
- `event_type`: same as `SUBSCRIBE`, or `all` to unsubscribe from everything.

Response:
```
UNSUBSCRIBE_OK
```

### BACKEND_TEST
Test backend connectivity and health before staging a route.

Format:
```
BACKEND_TEST|session_id|backend_url
```

Parameters:
- `backend_url`: full connection string to test.

Response:
```
BACKEND_OK|json_object
```

Example response:
```
BACKEND_OK|{"reachable":true,"response_time_ms":45,"status_code":200,"tls_valid":true}
```

or
```
BACKEND_FAIL|{"reachable":false,"error":"connection refused"}
```

Notes:
- Performs a test request to the backend (GET to root path or health check path).
- Useful for validating backends before adding routes.
- Does not affect staged or active configuration.

### CIRCUIT_BREAKER_SET
Configure circuit breaker for a route to handle backend failures gracefully.

Format:
```
CIRCUIT_BREAKER_SET|session_id|route_id|threshold|timeout|half_open_requests
```

Parameters:
- `route_id`: target route or `ALL` for global.
- `threshold`: number of consecutive failures before opening circuit.
- `timeout`: duration to wait before attempting recovery (e.g., `30s`, `1m`).
- `half_open_requests`: number of test requests to send during recovery.

Response:
```
CIRCUIT_OK
```

Notes:
- Circuit breaker prevents cascading failures by failing fast when backend is unhealthy.
- States: `closed` (normal), `open` (failing fast), `half_open` (testing recovery).
- Changes are staged; call `CONFIG_APPLY` to activate.

### CIRCUIT_BREAKER_STATUS
Get current circuit breaker state for a route.

Format:
```
CIRCUIT_BREAKER_STATUS|session_id|route_id
```

Response:
```
CIRCUIT_STATUS_OK|json_object
```

Example response:
```
CIRCUIT_STATUS_OK|{"state":"open","failures":15,"last_failure":"2024-12-20T11:20:00Z","next_attempt":"2024-12-20T11:20:30Z"}
```

### CIRCUIT_BREAKER_RESET
Manually reset a circuit breaker to closed state.

Format:
```
CIRCUIT_BREAKER_RESET|session_id|route_id
```

Response:
```
CIRCUIT_OK
```

Notes:
- Useful for forcing recovery after manual backend fixes.

### CLIENT_SHUTDOWN
Client declares a graceful shutdown; removes routes immediately.

Format:
```
CLIENT_SHUTDOWN|session_id
```

Response:
```
SHUTDOWN_OK
```

### SERVER_SHUTDOWN (Server → Client)
Server notifies all connected services of proxy shutdown.

Event:
```
SHUTDOWN
```

### RECONNECT
Reconnect to an existing session after disconnection.

Format:
```
RECONNECT|session_id
```

Response:
```
OK
```
or
```
REREGISTER
```

### Connection Monitoring
- Server enables TCP keepalive (default 30s period).
- If the connection drops, routes are retained for a grace period (e.g., 5 minutes) and then cleaned up.
- Clients should also enable TCP keepalive and implement reconnect logic.
- Use `PING` for application-level keepalive and `SESSION_INFO` to monitor connection health.

### Staged Configuration Management
- All configuration changes are staged and require `CONFIG_APPLY` to take effect.
- Staged changes timeout after 30 minutes if not applied (configurable).
- Use `CONFIG_DIFF` to review changes before applying.
- Use `CONFIG_ROLLBACK` to discard staged changes.
- Use `CONFIG_VALIDATE` to check for errors before `CONFIG_APPLY`.
- Use `CONFIG_APPLY_PARTIAL` to apply only specific types of changes.

### Transaction IDs (Optional)
- All commands support an optional transaction ID for request tracing.
- Format: `COMMAND|session_id|txn_id|...` (insert `txn_id` after `session_id`).
- Response includes the same `txn_id`: `RESPONSE|txn_id|...`.
- Useful for correlating commands and responses in distributed tracing systems.
- If omitted, responses do not include `txn_id`.

### Response Compression (Optional)
- Large responses (`ROUTE_LIST`, `STATS_GET`) support optional compression.
- Add `compressed` parameter: `ROUTE_LIST|session_id|compressed`.
- Response format: `ROUTE_LIST_OK|base64_gzipped_json`.
- Reduces bandwidth for services with many routes or high-frequency stats queries.

---

## Troubleshooting

### Connection Refused

**Problem:** Cannot connect to proxy on port 81  
**Solution:** Check network connectivity and firewall

```bash
# From service container
nc -zv proxy 81

# Check if port is exposed
docker service inspect proxy_proxy | grep 81
```

### Session Expired (REREGISTER)

**Problem:** Reconnection returns `REREGISTER`  
**Solution:** Session timeout (>60s), must register again

```python
response = client.send_command(f"RECONNECT|{session_id}")
if response == "REREGISTER":
    client.register('api-service', 'api-v2', 9000, 9001)
```

### Routes Not Active After Registration

**Problem:** Routes registered but traffic not flowing  
**Solution:** Check backend connectivity from proxy

```bash
# From proxy container
docker exec -it proxy curl http://api-v2:9000/health
```

### ERROR|Invalid session

**Problem:** Using wrong or expired session ID  
**Solution:** Re-register to get new session

```python
# Store session ID after registration
response = client.send_command("REGISTER|api|api-v2|9000|9001")
session_id = response.split('|')[1]

# Use this session ID for all subsequent commands
```

---

## Security Considerations

### Network Isolation

**Best Practice:** Only allow registry access from internal networks

```bash
# Firewall rule - Docker Swarm overlay network only
# Port 81 is NOT exposed to public internet by default

# If you need external access, restrict by IP:
ufw allow from 10.0.0.0/8 to any port 81
```

### No Authentication

**Current State:** Registry has NO authentication

**Security Measures:**
- Run on internal overlay network only
- Use firewall rules to restrict access
- Monitor registration events
- Consider implementing mutual TLS in future

**Production Recommendations:**
- DO NOT expose port 81 to public internet
- Use Docker Swarm overlay networks
- Implement application-level authentication if needed

---

## Monitoring

### Connection Status

Monitor TCP connections to registry:

```bash
# On proxy host
netstat -an | grep :81

# Count active sessions
docker exec -it proxy netstat -an | grep :81 | wc -l
```

### Log Events

Registry logs all events:

```bash
docker service logs proxy_proxy | grep "\[registry\]"
```

Sample output:
```
[registry] Service registered: api-service at api-v2:9000 (session: api-v2-9000-1734532800)
[registry] Route added: api-service at [api.example.com]/v2 -> http://api-v2:9000
[registry] Header added: X-API-Version = 2.0 for api-service
```

---

## Advanced Patterns

### Blue-Green Deployment

```python
# Deploy green (v2)
green = ProxyRegistryClient()
green.connect()
green.register('api-green', 'api-v2', 9000, 9001)
green.add_route(['api.example.com'], '/v2-beta', 'http://api-v2:9000')

# Test green environment...

# Switch traffic: shutdown blue, promote green
blue.shutdown()  # Removes all v1 routes
green.add_route(['api.example.com'], '/v2', 'http://api-v2:9000')
```

### Canary Release

```python
# Keep v1 running
v1_client.add_route(['api.example.com'], '/api', 'http://api-v1:9000')

# Deploy v2 on different path
v2_client.add_route(['api.example.com'], '/api-canary', 'http://api-v2:9000')

# Route 10% of users to canary path in application logic
# Monitor metrics, gradually increase traffic
```

### Maintenance Mode Workflow

```python
# Enter maintenance mode
client.enter_maintenance()

# Perform updates (database migrations, etc.)
# ...

# Exit maintenance mode
client.exit_maintenance()
```

### Multi-Domain Service

```python
# Single service, multiple domains
client.add_route(['api.example.com', 'api.example.org'], '/v2', 'http://api-v2:9000')

# Or separate routes per domain
client.add_route(['api.example.com'], '/v2', 'http://api-v2:9000')
client.add_route(['api.example.org'], '/v2', 'http://api-v2-eu:9000')
```

---

## Protocol Specification

### Message Format

All messages are ASCII text, newline-terminated:

```
<COMMAND>|<arg1>|<arg2>|...|<argN>\n
```

### Character Encoding

- **Encoding:** UTF-8
- **Line Ending:** `\n` (LF, ASCII 10)
- **Separator:** `|` (pipe, ASCII 124)

### Reserved Characters

The pipe character `|` is reserved. Do not use it in:
- Service names
- Hostnames
- Paths
- Header names/values
- Backend URLs

### Maximum Message Size

- **Command:** 16 KB per line (increased for bulk operations and JSON payloads)
- **Response:** 64 KB per line (supports large JSON responses from ROUTE_LIST, STATS_GET)
- **Compressed Response:** 1 MB (base64-encoded gzipped data)

### Timeout

- **Read Timeout:** 30 seconds per command
- **Connection Idle:** 5 minutes (keepalive)
- **Session Expiry:** 60 seconds after disconnect

---

## Related Documentation

- [README.md](README.md) - Overview and quick start
- [CONFIGURATION.md](CONFIGURATION.md) - Configuration reference
- [DEPLOYMENT.md](DEPLOYMENT.md) - Deployment guide
- [MIGRATION.md](MIGRATION.md) - Migration from nginx/Traefik/HAProxy

---

## Support

For integration help:

- Review v2 command reference and client examples above
- Check proxy logs: `docker service logs proxy_proxy`
- Test protocol with `nc` or `telnet`:
  ```bash
  nc proxy 81
  REGISTER|test|test-instance|9001|{}
  ```
- Verify network connectivity between service and proxy

**Common Issues:**
- Connection refused → Check network/firewall
- REREGISTER → Session expired, register again
- ERROR|Invalid session → Wrong session ID
- Routes not active → Did you call CONFIG_APPLY? Check staged config with CONFIG_DIFF
- Backend test failed → Verify backend is reachable from proxy
