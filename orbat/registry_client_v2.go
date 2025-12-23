package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// EventType represents different registry events
type EventType string

const (
	EventMaintenanceEnter EventType = "MAINT_ENTER"
	EventMaintenanceOK    EventType = "MAINT_OK"
	EventMaintenanceExit  EventType = "MAINT_EXIT"
	EventConnected        EventType = "CONNECTED"
	EventDisconnected     EventType = "DISCONNECTED"
	EventRouteAdded       EventType = "ROUTE_ADDED"
	EventRouteRemoved     EventType = "ROUTE_REMOVED"
	EventHealthCheckSet   EventType = "HEALTH_CHECK_SET"
	EventConfigApplied    EventType = "CONFIG_APPLIED"
	EventRetrying         EventType = "RETRYING"
	EventExtendedRetry    EventType = "EXTENDED_RETRY"
	EventReconnected      EventType = "RECONNECTED"
)

// Event represents a registry event with associated data
type Event struct {
	Type      EventType
	Data      map[string]interface{}
	Timestamp time.Time
}

// EventHandler is a function that handles registry events
type EventHandler func(event Event)

// RegistryClientV2 handles v2 protocol communication with the registry
type RegistryClientV2 struct {
	conn      net.Conn
	sessionID string
	scanner   *bufio.Scanner
	localIP   string // The container's IP on the web-net network

	// Event handling
	handlers map[EventType][]EventHandler
	mu       sync.RWMutex

	// Retry mechanism
	registryAddr    string
	serviceName     string
	instanceName    string
	maintenancePort int
	metadata        map[string]interface{}
	failureCount    int
	inExtendedRetry bool
	done            chan struct{}
}

// getContainerIP detects the container's IP address on the web-net network (10.2.2.0/24)
func getContainerIP() string {
	// Try to get IP from network interfaces
	// We specifically want the IP on web-net (10.2.2.0/24)
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		fmt.Printf("[client] Warning: failed to get interface addresses: %v\n", err)
		return ""
	}

	// Look for IP in 10.2.2.0/24 subnet (web-net)
	_, webNet, _ := net.ParseCIDR("10.2.2.0/24")

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil && webNet != nil && webNet.Contains(ipnet.IP) {
				return ipnet.IP.String()
			}
		}
	}

	// Fallback: return first non-loopback IPv4
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				fmt.Printf("[client] Warning: using IP outside web-net: %s\n", ipnet.IP.String())
				return ipnet.IP.String()
			}
		}
	}

	fmt.Printf("[client] Warning: no non-loopback IP found\n")
	return ""
}

// NewRegistryClient creates a new registry client without connecting
// Call Init() to establish the connection
func NewRegistryClient(registryAddr string, serviceName string, instanceName string, maintenancePort int, metadata map[string]interface{}) *RegistryClientV2 {
	return &RegistryClientV2{
		registryAddr:    registryAddr,
		serviceName:     serviceName,
		instanceName:    instanceName,
		maintenancePort: maintenancePort,
		metadata:        metadata,
		handlers:        make(map[EventType][]EventHandler),
		done:            make(chan struct{}),
	}
}

// Init establishes connection to the registry and registers the service
// Set cleanupOldRoutes to true to remove routes from previous sessions
func (c *RegistryClientV2) Init() error {
	return c.InitWithCleanup(true)
}

// InitWithCleanup establishes connection with optional route cleanup
func (c *RegistryClientV2) InitWithCleanup(cleanupOldRoutes bool) error {
	// Detect container IP first
	localIP := getContainerIP()
	if localIP == "" {
		return fmt.Errorf("failed to detect container IP address")
	}
	fmt.Printf("[client] Detected container IP: %s\n", localIP)
	c.localIP = localIP

	// Use IP as instance name if not provided
	if c.instanceName == "" {
		c.instanceName = localIP
	}

	if err := c.connect(); err != nil {
		return err
	}

	// Cleanup old routes if requested
	if cleanupOldRoutes {
		if err := c.CleanupOldRoutes(); err != nil {
			fmt.Printf("[client] Warning: failed to cleanup old routes: %v\n", err)
			// Don't fail Init if cleanup fails
		}
	}

	return nil
}

// connect performs the actual TCP connection and registration
func (c *RegistryClientV2) connect() error {
	conn, err := net.Dial("tcp", c.registryAddr)
	if err != nil {
		return err
	}

	// Enable TCP keepalive on client side
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	scanner := bufio.NewScanner(conn)

	// Register
	metadataJSON, _ := json.Marshal(c.metadata)
	registerCmd := fmt.Sprintf("REGISTER|%s|%s|%d|%s\n", c.serviceName, c.instanceName, c.maintenancePort, string(metadataJSON))
	conn.Write([]byte(registerCmd))

	if !scanner.Scan() {
		conn.Close()
		return fmt.Errorf("no response from registry")
	}

	response := scanner.Text()
	parts := strings.Split(response, "|")
	if len(parts) < 2 || parts[0] != "ACK" {
		conn.Close()
		return fmt.Errorf("registration failed: %s", response)
	}

	sessionID := parts[1]
	fmt.Printf("[client] Registered with session ID: %s\n", sessionID)

	c.conn = conn
	c.sessionID = sessionID
	c.scanner = scanner

	// Emit connected event
	c.emit(Event{
		Type:      EventConnected,
		Data:      map[string]interface{}{"session_id": sessionID, "local_ip": c.localIP},
		Timestamp: time.Now(),
	})

	return nil
}

// CleanupOldRoutes removes all routes from previous sessions
// This is useful when restarting to avoid stale route configurations
func (c *RegistryClientV2) CleanupOldRoutes() error {
	fmt.Printf("[client] Checking for old routes to cleanup...\n")

	// List all current routes
	routes, err := c.ListRoutes()
	if err != nil {
		return fmt.Errorf("failed to list routes: %w", err)
	}

	if len(routes) == 0 {
		fmt.Printf("[client] No old routes found\n")
		return nil
	}

	fmt.Printf("[client] Found %d routes from previous session(s)\n", len(routes))

	// Remove all old routes
	for _, route := range routes {
		if routeMap, ok := route.(map[string]interface{}); ok {
			if routeID, ok := routeMap["id"].(string); ok {
				fmt.Printf("[client] Removing old route: %s\n", routeID)
				if err := c.RemoveRoute(routeID); err != nil {
					fmt.Printf("[client] Warning: failed to remove route %s: %v\n", routeID, err)
					// Continue removing other routes
				}
			}
		}
	}

	// Apply the removal
	if err := c.ApplyConfig(); err != nil {
		return fmt.Errorf("failed to apply route cleanup: %w", err)
	}

	fmt.Printf("[client] Old routes cleaned up successfully\n")
	return nil
}

// AddRoute stages and applies a new route
func (c *RegistryClientV2) AddRoute(domains []string, path string, backendURL string, priority int) (string, error) {
	domainStr := strings.Join(domains, ",")
	cmd := fmt.Sprintf("ROUTE_ADD|%s|%s|%s|%s|%d\n", c.sessionID, domainStr, path, backendURL, priority)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return "", fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	parts := strings.Split(response, "|")
	if parts[0] == "ERROR" {
		return "", fmt.Errorf("add route failed: %s", response)
	}

	if parts[0] != "ROUTE_OK" {
		return "", fmt.Errorf("unexpected response: %s", response)
	}

	routeID := ""
	if len(parts) > 1 {
		routeID = parts[1]
	}

	// Emit route added event
	c.emit(Event{
		Type: EventRouteAdded,
		Data: map[string]interface{}{
			"route_id":    routeID,
			"domains":     domains,
			"path":        path,
			"backend_url": backendURL,
			"priority":    priority,
		},
		Timestamp: time.Now(),
	})

	return routeID, nil
}

// AddRouteBulk adds multiple routes in bulk
func (c *RegistryClientV2) AddRouteBulk(routes []map[string]interface{}) ([]map[string]string, error) {
	routesJSON, _ := json.Marshal(routes)
	cmd := fmt.Sprintf("ROUTE_ADD_BULK|%s|%s\n", c.sessionID, string(routesJSON))
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return nil, fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	parts := strings.Split(response, "|")
	if parts[0] == "ERROR" {
		return nil, fmt.Errorf("bulk add failed: %s", response)
	}

	var results []map[string]string
	if len(parts) > 1 {
		json.Unmarshal([]byte(parts[1]), &results)
	}

	return results, nil
}

// ListRoutes gets all routes (active and staged)
func (c *RegistryClientV2) ListRoutes() ([]interface{}, error) {
	cmd := fmt.Sprintf("ROUTE_LIST|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return nil, fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	parts := strings.Split(response, "|")
	if parts[0] == "ERROR" {
		return nil, fmt.Errorf("list routes failed: %s", response)
	}

	var routes []interface{}
	if len(parts) > 1 {
		json.Unmarshal([]byte(parts[1]), &routes)
	}

	return routes, nil
}

// RemoveRoute removes a route
func (c *RegistryClientV2) RemoveRoute(routeID string) error {
	cmd := fmt.Sprintf("ROUTE_REMOVE|%s|%s\n", c.sessionID, routeID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("remove route failed: %s", response)
	}

	return nil
}

// UpdateRoute updates a route field
func (c *RegistryClientV2) UpdateRoute(routeID string, field string, value string) error {
	cmd := fmt.Sprintf("ROUTE_UPDATE|%s|%s|%s|%s\n", c.sessionID, routeID, field, value)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("update route failed: %s", response)
	}

	return nil
}

// SetHeaders sets response headers
func (c *RegistryClientV2) SetHeaders(headerName string, headerValue string) error {
	cmd := fmt.Sprintf("HEADERS_SET|%s|ALL|%s|%s\n", c.sessionID, headerName, headerValue)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("set headers failed: %s", response)
	}

	return nil
}

// SetOptions sets configuration options
func (c *RegistryClientV2) SetOptions(key string, value string) error {
	cmd := fmt.Sprintf("OPTIONS_SET|%s|ALL|%s|%s\n", c.sessionID, key, value)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("set options failed: %s", response)
	}

	return nil
}

// SetHealthCheck configures health checks for a route
func (c *RegistryClientV2) SetHealthCheck(routeID string, path string, interval string, timeout string) error {
	cmd := fmt.Sprintf("HEALTH_SET|%s|%s|%s|%s|%s\n", c.sessionID, routeID, path, interval, timeout)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("set health check failed: %s", response)
	}

	// Emit health check set event
	c.emit(Event{
		Type: EventHealthCheckSet,
		Data: map[string]interface{}{
			"route_id": routeID,
			"path":     path,
			"interval": interval,
			"timeout":  timeout,
		},
		Timestamp: time.Now(),
	})

	return nil
}

// SetRateLimit configures rate limiting for a route
func (c *RegistryClientV2) SetRateLimit(routeID string, requests int, window string) error {
	cmd := fmt.Sprintf("RATELIMIT_SET|%s|%s|%d|%s\n", c.sessionID, routeID, requests, window)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("set rate limit failed: %s", response)
	}

	return nil
}

// SetCircuitBreaker configures circuit breaker for a route
func (c *RegistryClientV2) SetCircuitBreaker(routeID string, threshold int, timeout string, halfOpenRequests int) error {
	cmd := fmt.Sprintf("CIRCUIT_BREAKER_SET|%s|%s|%d|%s|%d\n", c.sessionID, routeID, threshold, timeout, halfOpenRequests)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("set circuit breaker failed: %s", response)
	}

	return nil
}

// ValidateConfig validates the staged configuration
func (c *RegistryClientV2) ValidateConfig() error {
	cmd := fmt.Sprintf("CONFIG_VALIDATE|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("validation failed: %s", response)
	}

	return nil
}

// ApplyConfig applies all staged configuration changes
func (c *RegistryClientV2) ApplyConfig() error {
	cmd := fmt.Sprintf("CONFIG_APPLY|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("apply config failed: %s", response)
	}

	fmt.Printf("[client] Configuration applied\n")

	// Emit config applied event
	c.emit(Event{
		Type:      EventConfigApplied,
		Data:      map[string]interface{}{},
		Timestamp: time.Now(),
	})

	return nil
}

// RollbackConfig discards all staged changes
func (c *RegistryClientV2) RollbackConfig() error {
	cmd := fmt.Sprintf("CONFIG_ROLLBACK|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("rollback config failed: %s", response)
	}

	return nil
}

// ConfigDiff shows differences between staged and active
func (c *RegistryClientV2) ConfigDiff() (map[string]interface{}, error) {
	cmd := fmt.Sprintf("CONFIG_DIFF|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return nil, fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	parts := strings.Split(response, "|")
	if parts[0] == "ERROR" {
		return nil, fmt.Errorf("config diff failed: %s", response)
	}

	var diff map[string]interface{}
	if len(parts) > 1 {
		json.Unmarshal([]byte(parts[1]), &diff)
	}

	return diff, nil
}

// DrainStart initiates graceful drain
func (c *RegistryClientV2) DrainStart(durationSeconds int) (time.Time, error) {
	cmd := fmt.Sprintf("DRAIN_START|%s|%d\n", c.sessionID, durationSeconds)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return time.Time{}, fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	parts := strings.Split(response, "|")
	if parts[0] == "ERROR" {
		return time.Time{}, fmt.Errorf("drain start failed: %s", response)
	}

	// Response format: DRAIN_OK|completion_time
	var completionTime time.Time
	if parts[0] == "DRAIN_OK" && len(parts) > 1 {
		completionTime, _ = time.Parse(time.RFC3339, parts[1])
	}

	return completionTime, nil
}

// DrainStatus gets the current drain status
func (c *RegistryClientV2) DrainStatus() (map[string]interface{}, error) {
	cmd := fmt.Sprintf("DRAIN_STATUS|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return nil, fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	parts := strings.Split(response, "|")
	if parts[0] == "ERROR" {
		return nil, fmt.Errorf("drain status failed: %s", response)
	}

	var status map[string]interface{}
	if len(parts) > 1 {
		json.Unmarshal([]byte(parts[1]), &status)
	}

	return status, nil
}

// DrainCancel cancels the ongoing drain
func (c *RegistryClientV2) DrainCancel() error {
	cmd := fmt.Sprintf("DRAIN_CANCEL|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("drain cancel failed: %s", response)
	}

	return nil
}

// MaintenanceEnter puts service in maintenance mode
func (c *RegistryClientV2) MaintenanceEnter(target string) error {
	return c.MaintenanceEnterWithURL(target, "")
}

// MaintenanceEnterWithURL puts service in maintenance mode with custom page URL
func (c *RegistryClientV2) MaintenanceEnterWithURL(target string, maintenancePageURL string) error {
	cmd := fmt.Sprintf("MAINT_ENTER|%s|%s|%s\n", c.sessionID, target, maintenancePageURL)
	c.conn.Write([]byte(cmd))

	// Emit maintenance enter event
	c.emit(Event{
		Type: EventMaintenanceEnter,
		Data: map[string]interface{}{
			"target":               target,
			"maintenance_page_url": maintenancePageURL,
		},
		Timestamp: time.Now(),
	})

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if response != "ACK" {
		if strings.HasPrefix(response, "ERROR") {
			return fmt.Errorf("maintenance enter failed: %s", response)
		}
	}

	// Read the event line (expecting MAINT_OK). Wait up to a short timeout
	// for the registry to confirm maintenance was applied.
	waitTimeout := 10 * time.Second
	// Set read deadline on the underlying connection so scanner.Scan() will
	// unblock on timeout.
	if conn, ok := c.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
		_ = conn.SetReadDeadline(time.Now().Add(waitTimeout))
		defer conn.SetReadDeadline(time.Time{})
	}

	for c.scanner.Scan() {
		event := c.scanner.Text()
		fmt.Printf("[client] Maintenance event: %s\n", event)
		// go-proxy sends "MAINT_OK|target" so use prefix check
		if strings.HasPrefix(event, "MAINT_OK") {
			// Emit maintenance OK event
			c.emit(Event{
				Type:      EventMaintenanceOK,
				Data:      map[string]interface{}{"target": target, "event": event},
				Timestamp: time.Now(),
			})
			return nil
		}
		if strings.HasPrefix(event, "ERROR") {
			return fmt.Errorf("maintenance enter failed: %s", event)
		}
		// Ignore other events and continue waiting until timeout or MAINT_OK
	}

	if err := c.scanner.Err(); err != nil {
		return fmt.Errorf("no MAINT_OK received within timeout: %w", err)
	}

	return fmt.Errorf("no MAINT_OK received")
}

// MaintenanceExit exits maintenance mode
func (c *RegistryClientV2) MaintenanceExit(target string) error {
	cmd := fmt.Sprintf("MAINT_EXIT|%s|%s\n", c.sessionID, target)
	c.conn.Write([]byte(cmd))

	// Emit maintenance exit event
	c.emit(Event{
		Type:      EventMaintenanceExit,
		Data:      map[string]interface{}{"target": target},
		Timestamp: time.Now(),
	})

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if response != "ACK" {
		if strings.HasPrefix(response, "ERROR") {
			return fmt.Errorf("maintenance exit failed: %s", response)
		}
	}

	// Read the event line (expecting MAINT_OK). Wait up to a short timeout
	waitTimeout := 10 * time.Second
	if conn, ok := c.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
		_ = conn.SetReadDeadline(time.Now().Add(waitTimeout))
		defer conn.SetReadDeadline(time.Time{})
	}

	for c.scanner.Scan() {
		event := c.scanner.Text()
		fmt.Printf("[client] Maintenance event: %s\n", event)
		// go-proxy sends "MAINT_OK|target" so use prefix check
		if strings.HasPrefix(event, "MAINT_OK") {
			// Emit maintenance OK event
			c.emit(Event{
				Type:      EventMaintenanceOK,
				Data:      map[string]interface{}{"target": target, "event": event},
				Timestamp: time.Now(),
			})
			return nil
		}
		if strings.HasPrefix(event, "ERROR") {
			return fmt.Errorf("maintenance exit failed: %s", event)
		}
	}

	if err := c.scanner.Err(); err != nil {
		return fmt.Errorf("no MAINT_OK received within timeout: %w", err)
	}

	return fmt.Errorf("no MAINT_OK received")
}

// MaintenanceStatus gets maintenance status
func (c *RegistryClientV2) MaintenanceStatus() (map[string]interface{}, error) {
	cmd := fmt.Sprintf("MAINT_STATUS|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return nil, fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	parts := strings.Split(response, "|")
	if parts[0] == "ERROR" {
		return nil, fmt.Errorf("maintenance status failed: %s", response)
	}

	var status map[string]interface{}
	if len(parts) > 1 {
		json.Unmarshal([]byte(parts[1]), &status)
	}

	return status, nil
}

// GetStats retrieves statistics for the service
func (c *RegistryClientV2) GetStats() ([]interface{}, error) {
	cmd := fmt.Sprintf("STATS_GET|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return nil, fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	parts := strings.Split(response, "|")
	if parts[0] == "ERROR" {
		return nil, fmt.Errorf("get stats failed: %s", response)
	}

	var stats []interface{}
	if len(parts) > 1 {
		json.Unmarshal([]byte(parts[1]), &stats)
	}

	return stats, nil
}

// TestBackend tests if a backend is reachable
func (c *RegistryClientV2) TestBackend(backendURL string) (map[string]interface{}, error) {
	cmd := fmt.Sprintf("BACKEND_TEST|%s|%s\n", c.sessionID, backendURL)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return nil, fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	parts := strings.Split(response, "|")
	if len(parts) < 1 {
		return nil, fmt.Errorf("invalid response")
	}

	var result map[string]interface{}
	if len(parts) > 1 {
		json.Unmarshal([]byte(parts[1]), &result)
	}

	return result, nil
}

// SessionInfo gets session information
func (c *RegistryClientV2) SessionInfo() (map[string]interface{}, error) {
	cmd := fmt.Sprintf("SESSION_INFO|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return nil, fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	parts := strings.Split(response, "|")
	if parts[0] == "ERROR" {
		return nil, fmt.Errorf("session info failed: %s", response)
	}

	var info map[string]interface{}
	if len(parts) > 1 {
		json.Unmarshal([]byte(parts[1]), &info)
	}

	return info, nil
}

// Ping keeps the connection alive
func (c *RegistryClientV2) Ping() error {
	cmd := fmt.Sprintf("PING|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if response != "PONG" {
		return fmt.Errorf("unexpected response to ping: %s", response)
	}

	return nil
}

// Shutdown gracefully shuts down the service
func (c *RegistryClientV2) Shutdown() error {
	cmd := fmt.Sprintf("CLIENT_SHUTDOWN|%s\n", c.sessionID)
	c.conn.Write([]byte(cmd))

	if !c.scanner.Scan() {
		return fmt.Errorf("no response from registry")
	}

	response := c.scanner.Text()
	if strings.HasPrefix(response, "ERROR") {
		return fmt.Errorf("shutdown failed: %s", response)
	}

	c.conn.Close()

	// Emit disconnected event
	c.emit(Event{
		Type:      EventDisconnected,
		Data:      map[string]interface{}{"reason": "shutdown"},
		Timestamp: time.Now(),
	})

	return nil
}

// Close closes the connection and stops the keepalive loop
func (c *RegistryClientV2) Close() {
	close(c.done)
	if c.conn != nil {
		c.conn.Close()
	}
	c.emit(Event{
		Type:      EventDisconnected,
		Data:      map[string]interface{}{"reason": "close"},
		Timestamp: time.Now(),
	})
}

// On registers an event handler for a specific event type
func (c *RegistryClientV2) On(eventType EventType, handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[eventType] = append(c.handlers[eventType], handler)
}

// Off removes all handlers for a specific event type
func (c *RegistryClientV2) Off(eventType EventType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.handlers, eventType)
}

// emit triggers all handlers for a specific event
func (c *RegistryClientV2) emit(event Event) {
	c.mu.RLock()
	handlers := c.handlers[event.Type]
	c.mu.RUnlock()

	for _, handler := range handlers {
		// Run handlers in goroutines to avoid blocking
		go handler(event)
	}
}

// GetLocalIP returns the detected container IP address
func (c *RegistryClientV2) GetLocalIP() string {
	return c.localIP
}

// BuildBackendURL builds a backend URL using the container's IP and specified port
func (c *RegistryClientV2) BuildBackendURL(port string) string {
	return fmt.Sprintf("http://%s:%s", c.localIP, port)
}

// BuildMaintenanceURL builds a maintenance URL using the container's IP and specified port
func (c *RegistryClientV2) BuildMaintenanceURL(port string) string {
	return fmt.Sprintf("http://%s:%s/", c.localIP, port)
}

// StartKeepalive starts the keepalive loop with automatic retry mechanism
func (c *RegistryClientV2) StartKeepalive() {
	normalInterval := 30 * time.Second
	extendedRetryInterval := 60 * time.Second
	maxQuickRetries := 5

	ticker := time.NewTicker(normalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := c.Ping()
			if err != nil {
				c.failureCount++
				fmt.Printf("[client] Keepalive ping failed (attempt %d): %v\n", c.failureCount, err)

				// Emit retry event
				c.emit(Event{
					Type: EventRetrying,
					Data: map[string]interface{}{
						"attempt": c.failureCount,
						"error":   err.Error(),
					},
					Timestamp: time.Now(),
				})

				// Try to reconnect with backoff strategy
				go c.reconnectWithBackoff(maxQuickRetries, &ticker, normalInterval, extendedRetryInterval)
			} else {
				// Successful ping - reset failure count
				if c.failureCount > 0 {
					c.failureCount = 0
					if c.inExtendedRetry {
						fmt.Printf("[client] Connection stable, returning to normal ping interval\n")
						c.inExtendedRetry = false
						ticker.Reset(normalInterval)
					}
				}
			}
		case <-c.done:
			return
		}
	}
}

// reconnectWithBackoff attempts to reconnect with exponential backoff
func (c *RegistryClientV2) reconnectWithBackoff(maxQuickRetries int, ticker **time.Ticker, normalInterval, extendedRetryInterval time.Duration) {
	attempt := c.failureCount
	var retryDelay time.Duration

	// First 5 attempts: exponential backoff (5s, 10s, 15s, 20s, 25s)
	if attempt <= maxQuickRetries {
		retryDelay = time.Duration(attempt*5) * time.Second
	} else {
		// After 5 failures: switch to 1-minute retries
		if !c.inExtendedRetry {
			fmt.Printf("[client] Switching to extended retry mode (1-minute intervals)\n")
			c.inExtendedRetry = true
			(*ticker).Reset(extendedRetryInterval)

			// Emit extended retry event
			c.emit(Event{
				Type:      EventExtendedRetry,
				Data:      map[string]interface{}{"interval": extendedRetryInterval.String()},
				Timestamp: time.Now(),
			})
		}
		retryDelay = 5 * time.Second
	}

	time.Sleep(retryDelay)
	fmt.Printf("[client] Attempting reconnection (attempt %d, after %v delay)...\n", attempt, retryDelay)

	if err := c.reconnect(); err != nil {
		fmt.Printf("[client] Reconnection attempt %d failed: %v\n", attempt, err)
	} else {
		// Connection successful - reset to normal behavior
		c.failureCount = 0
		if c.inExtendedRetry {
			fmt.Printf("[client] Connection restored! Returning to normal ping interval\n")
			c.inExtendedRetry = false
			(*ticker).Reset(normalInterval)
		}
		fmt.Printf("[client] Successfully reconnected to registry\n")

		// Emit reconnected event
		c.emit(Event{
			Type:      EventReconnected,
			Data:      map[string]interface{}{"attempt": attempt, "session_id": c.sessionID},
			Timestamp: time.Now(),
		})
	}
}

// reconnect attempts to reconnect to the registry
func (c *RegistryClientV2) reconnect() error {
	// Close old connection if it exists
	if c.conn != nil {
		c.conn.Close()
	}

	// Detect container IP
	localIP := getContainerIP()
	if localIP == "" {
		return fmt.Errorf("failed to detect container IP address")
	}
	c.localIP = localIP

	// Use connect method for reconnection
	return c.connect()
}

// Close stops the keepalive loop and closes the connection
