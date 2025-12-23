# Registry Client V2 - Usage Patterns

The RegistryClientV2 now supports two usage patterns:

## Pattern 1: All-in-One (Backward Compatible)

This is the original pattern that creates the client and connects in one step:

```go
registryAddr := "go-proxy:81"
metadata := map[string]interface{}{
    "version": "1.0.0",
    "git_repo": repoURL,
}

// Creates client AND connects to registry
client, err := NewRegistryClientV2(registryAddr, "my-service", "", 3001, metadata)
if err != nil {
    return fmt.Errorf("failed to register: %w", err)
}

// Use the client
routeID, err := client.AddRoute(domains, "/", backendURL, 10)
```

## Pattern 2: Init Pattern (New)

This pattern separates creation from connection, allowing configuration before connecting:

```go
registryAddr := "go-proxy:81"
metadata := map[string]interface{}{
    "version": "1.0.0",
    "git_repo": repoURL,
}

// Step 1: Create the client (no connection yet)
registry := NewRegistryClient(registryAddr, "my-service", "", 3001, metadata)

// Step 2: Configure event handlers before connecting
registry.On(EventConnected, func(event Event) {
    fmt.Printf("Connected with session: %v\n", event.Data["session_id"])
})

registry.On(EventRetrying, func(event Event) {
    fmt.Printf("Retrying connection (attempt %v)...\n", event.Data["attempt"])
})

// Step 3: Initialize connection to registry
if err := registry.Init(); err != nil {
    return fmt.Errorf("failed to initialize: %w", err)
}

// Step 4: Use the client methods
routeID, err := registry.AddRoute(domains, "/", backendURL, 10)
if err != nil {
    return fmt.Errorf("failed to add route: %w", err)
}

err = registry.SetHealthCheck(routeID, "/", "30s", "5s")
err = registry.ApplyConfig()

// Step 5: Start keepalive with automatic retry
go registry.StartKeepalive()
```

## Available Methods

After initialization, all methods are called on the client instance:

### Connection Management
- `registry.Init()` - Establish connection
- `registry.StartKeepalive()` - Start keepalive with auto-retry
- `registry.Close()` - Close connection
- `registry.Ping()` - Send ping

### Route Management  
- `registry.AddRoute(domains, path, backendURL, priority)` - Add route
- `registry.AddRouteBulk(routes)` - Add multiple routes
- `registry.UpdateRoute(routeID, field, value)` - Update route
- `registry.RemoveRoute(routeID)` - Remove route
- `registry.ListRoutes()` - List all routes

### Configuration
- `registry.SetHealthCheck(routeID, path, interval, timeout)` - Configure health check
- `registry.SetRateLimit(routeID, requests, window)` - Set rate limit
- `registry.SetCircuitBreaker(routeID, threshold, timeout, halfOpen)` - Configure circuit breaker
- `registry.SetOptions(key, value)` - Set options
- `registry.ApplyConfig()` - Apply staged configuration
- `registry.RollbackConfig()` - Rollback changes
- `registry.ValidateConfig()` - Validate staged config

### Maintenance
- `registry.MaintenanceEnter(target, reason)` - Enter maintenance mode
- `registry.MaintenanceExit(target)` - Exit maintenance mode
- `registry.MaintenanceStatus()` - Get maintenance status

### Events
- `registry.On(EventType, handler)` - Register event handler

## Benefits of Init Pattern

1. **Separation of Concerns**: Create object first, configure it, then connect
2. **Event Handlers**: Set up handlers before connection events fire
3. **Testing**: Easier to mock and test
4. **Flexibility**: Can modify configuration before connecting
5. **Cleaner API**: Follows common Go patterns like `database/sql`

## Automatic Retry Mechanism

Both patterns include automatic retry with exponential backoff:

- **Attempts 1-5**: 5s, 10s, 15s, 20s, 25s delays
- **Attempts 6+**: 1-minute intervals indefinitely
- **On Success**: Returns to normal 30s ping interval

Events are emitted for retry states:
- `EventRetrying` - Connection lost, retrying
- `EventExtendedRetry` - Switched to 1-minute intervals
- `EventReconnected` - Successfully reconnected
