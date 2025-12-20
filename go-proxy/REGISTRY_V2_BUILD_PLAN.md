# Registry V2 Implementation Build Plan

## Overview

Complete rewrite of the service registry to implement Protocol v2. The v2 registry provides:
- Staged configuration management (route/header/option changes don't apply until CONFIG_APPLY)
- Route IDs for scoped operations
- Advanced features: circuit breakers, health checks, rate limiting, drain mode, event subscriptions
- Comprehensive validation, rollback, and diff capabilities
- Per-route maintenance mode

## Architecture

### Core Types (registry_v2.go)
✅ **Done:**
- `ServiceV2` - service with staged and active config
- `RouteV2` - route with ID, domains, path, backend, priority
- `HealthCheckV2`, `RateLimitV2`, `CircuitBreakerV2` - config types
- `RegistryV2` - registry manager
- `SessionID`, `RouteID` - type-safe identifiers
- Command dispatcher with all 30+ v2 commands

## Implementation Phases

### Phase 1: Core Routing (HIGH PRIORITY)
**Goal:** Enable basic route registration and configuration apply

**Files:** `registry_v2.go`

**Commands to implement:**
1. ✅ `REGISTER` - create session
2. ✅ `RECONNECT` - restore session
3. ✅ `PING`/`PONG` - keepalive
4. ✅ `SESSION_INFO` - session stats (stub added)
5. `ROUTE_ADD` - stage route (partial)
6. `ROUTE_REMOVE` - stage route removal
7. `ROUTE_LIST` - list all routes
8. `CONFIG_VALIDATE` - validate staged routes
9. `CONFIG_APPLY` - apply all staged routes to proxy
10. `CONFIG_ROLLBACK` - discard staged changes

**ProxyServer interface updates needed:**
- Keep existing `AddRoute()`, `RemoveRoute()` calls compatible
- Or extend interface if needed for route IDs

### Phase 2: Configuration Management
**Goal:** Headers, options, and multi-stage apply

**Commands:**
1. `HEADERS_SET` - stage header
2. `HEADERS_REMOVE` - remove header
3. `OPTIONS_SET` - stage option
4. `OPTIONS_REMOVE` - remove option
5. `CONFIG_APPLY_PARTIAL` - apply only specific types
6. `CONFIG_DIFF` - preview staged changes

**Key Logic:**
- Merge headers/options when applying routes
- Validate header/option values
- Support `ALL` target for global headers/options

### Phase 3: Advanced Configuration
**Goal:** Health checks, rate limiting, circuit breakers

**Commands:**
1. `HEALTH_SET` - configure health checks per route
2. `RATELIMIT_SET` - configure rate limiting
3. `CIRCUIT_BREAKER_SET` - configure circuit breaker
4. `CIRCUIT_BREAKER_STATUS` - get circuit breaker state
5. `CIRCUIT_BREAKER_RESET` - manually reset circuit breaker

**Integration Points:**
- Pass health check configs to proxy
- Pass rate limit configs to proxy
- Pass circuit breaker configs to proxy
- Proxy implements the actual health checking logic

### Phase 4: Operational Features
**Goal:** Drain mode, maintenance, drain tracking

**Commands:**
1. `DRAIN_START` - gradually reduce traffic
2. `DRAIN_STATUS` - check drain progress
3. `DRAIN_CANCEL` - stop drain
4. `MAINT_ENTER` - enter maintenance (route-specific)
5. `MAINT_EXIT` - exit maintenance (route-specific)
6. `MAINT_STATUS` - check maintenance status
7. `CLIENT_SHUTDOWN` - graceful shutdown

**Key Logic:**
- Drain uses weighted traffic reduction over time
- Maintenance per route using route ID targeting
- Track drain state and progress

### Phase 5: Observability & Events
**Goal:** Stats, backend testing, event subscriptions

**Commands:**
1. `STATS_GET` - get request/error/latency stats
2. `BACKEND_TEST` - test backend connectivity
3. `SUBSCRIBE` - subscribe to events
4. `UNSUBSCRIBE` - unsubscribe from events

**Event Types:**
- `cert_renewed` - certificate renewal
- `global_config_changed` - global config reload
- `backend_health_changed` - backend health status
- `all` - all events

**Key Logic:**
- Collect stats from proxy
- Make HTTP test requests to backends
- Queue events for subscribed sessions
- Send events async on same TCP connection

### Phase 6: Bulk Operations (OPTIONAL)
**Goal:** Efficient multi-route operations

**Commands:**
1. `ROUTE_ADD_BULK` - add multiple routes at once
2. `ROUTE_UPDATE` - update route fields efficiently

## Implementation Strategy

### Step 1: Phase 1 Implementation (CRITICAL PATH)
1. Implement `handleRouteAddV2` - stage route, generate route ID
2. Implement `handleRouteRemoveV2` - mark route for removal
3. Implement `handleRouteListV2` - return JSON of all routes (active + staged)
4. Implement `handleConfigValidateV2` - validate all staged changes
5. Implement `handleConfigApplyV2` - atomically apply staged → active
6. Implement `handleConfigRollbackV2` - clear staged changes
7. Test with orbat service

### Step 2: Phase 2 Implementation
1. Implement header/option staging
2. Implement `CONFIG_APPLY` to merge headers with routes
3. Implement `CONFIG_DIFF` for preview

### Step 3: Phases 3-6
Build out remaining handlers progressively.

## Data Flow Example

**Scenario: Add route and configure headers**

1. Client connects: `REGISTER|orbat|orbat.1|3001|{}`
   - Returns: `ACK|sess123`

2. Add route: `ROUTE_ADD|sess123|orbat.chilla55.de|/|http://orbat:3000|0`
   - Stages route `r1` in `svc.stagedRoutes`
   - Returns: `ROUTE_OK|r1`

3. Set header: `HEADERS_SET|sess123|ALL|X-Service|orbat`
   - Stages header in `svc.stagedHeaders`
   - Returns: `HEADERS_OK`

4. Validate: `CONFIG_VALIDATE|sess123`
   - Checks all staged routes for errors (domain format, URL scheme, path, etc.)
   - Returns: `OK` or `ERROR|details`

5. Apply: `CONFIG_APPLY|sess123`
   - Validates first
   - Calls `proxyServer.AddRoute()` with merged route + headers
   - Moves staged → active
   - Returns: `OK` or `ERROR|details`

6. List: `ROUTE_LIST|sess123`
   - Returns: `ROUTE_LIST_OK|[{route_id:r1, status:active, ...}]`

## Testing Strategy

### Unit Tests (registry_test.go)
- ✅ Existing tests keep working (or migrate to v2 equivalents)
- Test REGISTER/RECONNECT with different metadata
- Test ROUTE_ADD with various domains/paths
- Test CONFIG_APPLY atomicity (all or nothing)
- Test CONFIG_ROLLBACK discards staged
- Test route ID generation uniqueness

### Integration Tests
- Orbat registers and adds routes
- Proxy receives routes via `ProxyServer.AddRoute()`
- Traffic flows to orbat
- Orbat adds route via v2, traffic routes correctly
- Orbat maintenance mode works per-route

### Manual Testing
```bash
# Connect to registry
nc proxy 81

# Register
REGISTER|test|test-1|3001|{"version":"1.0"}
# → ACK|sess123

# Add route
ROUTE_ADD|sess123|example.com|/|http://backend:8000|0
# → ROUTE_OK|r1

# Add another route
ROUTE_ADD|sess123|example.com|/api|http://backend:9000|10
# → ROUTE_OK|r2

# List routes (shows staged)
ROUTE_LIST|sess123
# → ROUTE_LIST_OK|[...]

# Validate
CONFIG_VALIDATE|sess123
# → OK

# Apply all
CONFIG_APPLY|sess123
# → OK

# List again (shows active)
ROUTE_LIST|sess123
# → ROUTE_LIST_OK|[...]
```

## Migration Path

### Option A: Parallel V1 + V2 (Recommended for safety)
- Keep old `registry.go` as v1
- Run new `registry_v2.go` on same port 81
- Proxy accepts both v1 and v2 connections
- Migrate orbat to v2 only
- Eventually remove v1

### Option B: Full Cutover (Aggressive)
- Replace registry.go with v2-only implementation
- Update main.go to use RegistryV2
- Requires immediate orbat update to v2
- Risky but cleaner

**Recommendation:** Use Option A first, then migrate.

## Dependencies

**Internal:**
- `ProxyServer` interface - may need extension for route IDs
- `proxy/` package - needs to handle route IDs if we use them

**External:**
- `net` - TCP handling
- `encoding/json` - JSON marshaling
- `bufio` - line scanning
- `sync` - mutexes
- `time` - timestamps

## Compilation Status

✅ Registry v2 compiles without errors (skeleton with stubs)

```bash
cd go-proxy/proxy-manager
go build ./...
# → Success
```

## Next Steps

1. **Phase 1 Implementation** - start with core routing (ROUTE_ADD, CONFIG_APPLY)
2. **Orbat integration** - update orbat/main.go to use v2 protocol
3. **Testing** - verify routing works end-to-end
4. **Phase 2-6** - incrementally add remaining features
5. **Cleanup** - remove v1 when stable

## Files Modified/Created

- ✅ Created: `proxy-manager/registry/registry_v2.go` (704 lines)
- Updated: `SERVICE_REGISTRY.md` (v1 removed, v2 documented)
- To update: `proxy-manager/main.go` (to start RegistryV2 instead of old Registry)
- To update: `orbat/main.go` (to use v2 protocol)
- To create: `orbat/registry_client.go` (v2 TCP client for orbat)

---

**Status:** Ready for Phase 1 implementation
**Est. Time:** Phase 1 = 2-3 hours, Phases 2-5 = 4-6 hours each
