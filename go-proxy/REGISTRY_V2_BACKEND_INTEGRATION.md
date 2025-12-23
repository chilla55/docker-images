# Registry V2 Backend Integration Opportunities

## Current Architecture Analysis

### Components Overview

1. **Registry V2** (`registry/registry.go`)
   - Manages route registration via TCP protocol
   - Stages health checks, rate limits, circuit breakers
   - Calls `proxyServer.AddRoute()` with options map during `CONFIG_APPLY`
   - Has `upstreamTimeout` for `BACKEND_TEST` command

2. **Proxy Server** (`proxy/proxy.go`)
   - Creates `Backend` structs for each route
   - Parses options map to configure backend features
   - Implements circuit breaker logic directly in `Backend`
   - Tracks slow requests, compression, websockets, retries

3. **Health Checker** (`health/health.go`)
   - Standalone component with its own service tracking
   - Not integrated with registry v2 route registration
   - Uses HTTP GET checks with configurable intervals/timeouts
   - Records results to database

### Current Flow
```
Service (orbat) 
  → TCP Registry v2 
    → ROUTE_ADD + HEALTH_SET + CONFIG_APPLY 
      → proxyServer.AddRoute(options)
        → Backend created with circuit breaker config
          → [No health checker integration]
```

## Integration Gaps

### 1. Health Check Integration
**Problem:**
- Registry v2 accepts `HEALTH_SET|session_id|route_id|path|interval|timeout`
- This data is staged but never wired to the `health.Checker`
- Proxy backends have health tracking, but it's not coordinated

**Opportunity:**
- When `CONFIG_APPLY` is called, register routes with health checker
- Pass `healthChecker` interface to registry v2
- On route add with health config: `healthChecker.AddService(routeID, backendURL+healthPath, interval, timeout, 200)`
- On route remove: `healthChecker.RemoveService(routeID)`

### 2. Backend Circuit Breaker State Visibility
**Problem:**
- Circuit breaker state lives in `Backend` struct (private to proxy package)
- Registry v2 `CIRCUIT_BREAKER_STATUS` command can't query actual runtime state
- Services have no visibility into whether their routes are circuit-broken

**Opportunity:**
- Add `GetBackendStatus(domains, path) (*BackendStatus, error)` to proxy Server
- Return struct with: `Healthy bool, CircuitState string, Failures int, LastCheck time`
- Wire this into `CIRCUIT_BREAKER_STATUS` command to return real-time data
- Add `ROUTE_STATUS|session_id|route_id` command for comprehensive route health

### 3. Backend Timeout Configuration
**Problem:**
- `upstreamTimeout` only used in `BACKEND_TEST` command
- HTTP transport timeout not exposed via registry v2
- Services can't tune timeouts per-route

**Opportunity:**
- Add `timeout` option support in `ROUTE_ADD` or as separate command
- Parse into Backend's Timeout field
- Apply to HTTP client transport: `transport.ResponseHeaderTimeout`

### 4. Rate Limit Enforcement
**Problem:**
- Registry v2 stages rate limits but doesn't enforce them
- No actual rate limiting middleware in proxy
- Data is staged but unused

**Opportunity:**
- Add rate limiting middleware to proxy package
- Integrate with `ratelimit` package (already exists in codebase)
- On `CONFIG_APPLY`, configure rate limiter per route
- Return 429 Too Many Requests when limit exceeded

### 5. Maintenance Mode Coordination
**Problem:**
- `MAINT_ENTER` marks routes as in maintenance
- Proxy has no visibility—continues routing traffic
- No maintenance page served

**Opportunity:**
- Add `SetMaintenance(domains, path, enabled bool)` to proxy Server
- Store maintenance state in Backend or Route
- In `ServeHTTP`, check maintenance flag before proxying
- Return 503 with maintenance page HTML

### 6. Drain Mode Integration
**Problem:**
- Registry v2 tracks drain state per session
- Proxy doesn't reduce traffic or reject new connections
- No actual graceful shutdown behavior

**Opportunity:**
- Add drain interface to proxy Server
- `StartDrain(sessionID, duration) error`
- Gradually reduce connection acceptance rate
- After drain period, stop accepting new requests for those routes
- Send drain events back to services via registry connection

### 7. Health Check Results → Circuit Breaker
**Problem:**
- Health checker runs independent checks
- Circuit breaker reacts to proxy errors
- No coordination between components

**Opportunity:**
- Health checker should notify proxy of persistent failures
- Proxy can preemptively open circuit before errors cascade
- Add callback: `healthChecker.OnStatusChange(routeID, status)`
- Proxy listens and updates Backend.Healthy flag

## Recommended Implementation Priority

### Phase 1: Core Visibility (Highest Impact)
1. **Backend Status Query**
   - Add `GetBackendStatus()` to proxy Server
   - Wire into `CIRCUIT_BREAKER_STATUS` and new `ROUTE_STATUS` command
   - Services can now query real-time backend health

2. **Health Checker Integration**
   - Pass `healthChecker` to registry v2
   - On `CONFIG_APPLY` with health config → `AddService()`
   - On `ROUTE_REMOVE` → `RemoveService()`

### Phase 2: Traffic Management
3. **Maintenance Mode**
   - Add maintenance flag to Backend/Route
   - Check in `ServeHTTP` before proxying
   - Return 503 with custom page

4. **Rate Limiting**
   - Integrate existing `ratelimit` package
   - Apply per-route limits from staged config
   - Return 429 on exceeded

### Phase 3: Advanced Features
5. **Drain Mode**
   - Implement progressive traffic reduction
   - Coordinate with service shutdown

6. **Health → Circuit Breaker Sync**
   - Health check failures notify proxy
   - Preemptive circuit opening

7. **Timeout Configuration**
   - Per-route backend timeout tuning
   - Wire into HTTP transport

## Code Changes Required

### 1. Registry V2 Changes
```go
// Add to RegistryV2 struct
type RegistryV2 struct {
    // ... existing fields
    healthChecker HealthCheckInterface
}

type HealthCheckInterface interface {
    AddService(name, url string, interval, timeout time.Duration, expectedStatus int)
    RemoveService(name string)
}

// Update NewRegistryV2 signature
func NewRegistryV2(
    port int, 
    proxyServer ProxyServer, 
    debug bool, 
    upstreamTimeout time.Duration,
    healthChecker HealthCheckInterface,
) *RegistryV2

// In handleConfigApplyV2, after route add:
if hc, found := svc.stagedHealth[routeID]; found {
    r.healthChecker.AddService(
        string(routeID), 
        route.BackendURL + hc.Path, 
        hc.Interval, 
        hc.Timeout, 
        200,
    )
}
```

### 2. Proxy Server Changes
```go
// Add methods
func (s *Server) GetBackendStatus(domain, path string) *BackendStatus {
    backend := s.findBackend(domain, path)
    if backend == nil {
        return nil
    }
    backend.mu.RLock()
    defer backend.mu.RUnlock()
    return &BackendStatus{
        Healthy:     backend.Healthy,
        CircuitState: backend.cbState,
        Failures:    backend.cbFailures,
        Successes:   backend.cbSuccesses,
        OpenedAt:    backend.cbOpenedAt,
    }
}

func (s *Server) SetMaintenance(domains []string, path string, enabled bool) {
    // Find and update backend
}
```

### 3. Main.go Wiring
```go
// Pass healthChecker to registry
regV2 := registry.NewRegistryV2(
    *registryPort, 
    proxyServer, 
    *debug, 
    *upstreamTimeout,
    healthChecker, // Add this
)
```

## Benefits

1. **Real-time Visibility**: Services can query actual backend health via TCP
2. **Coordinated Health**: Health checks drive circuit breaker decisions
3. **Traffic Control**: Maintenance and drain modes actually affect routing
4. **Better Reliability**: Rate limits prevent overload, timeouts prevent hangs
5. **Simplified Operations**: All backend tuning via TCP v2 protocol

## Next Steps

1. Review this document and prioritize features
2. Implement Phase 1 (Backend Status + Health Integration)
3. Add integration tests verifying end-to-end flow
4. Update SERVICE_REGISTRY.md with new capabilities
5. Deploy and test with orbat service
