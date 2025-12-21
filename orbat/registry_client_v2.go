package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

// RegistryClientV2 handles v2 protocol communication with the registry
type RegistryClientV2 struct {
	conn      net.Conn
	sessionID string
	scanner   *bufio.Scanner
}

// NewRegistryClientV2 connects to the registry and registers the service
func NewRegistryClientV2(registryAddr string, serviceName string, instanceName string, maintenancePort int, metadata map[string]interface{}) (*RegistryClientV2, error) {
	conn, err := net.Dial("tcp", registryAddr)
	if err != nil {
		return nil, err
	}

	// Enable TCP keepalive on client side
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	scanner := bufio.NewScanner(conn)

	// Register
	metadataJSON, _ := json.Marshal(metadata)
	registerCmd := fmt.Sprintf("REGISTER|%s|%s|%d|%s\n", serviceName, instanceName, maintenancePort, string(metadataJSON))
	conn.Write([]byte(registerCmd))

	if !scanner.Scan() {
		conn.Close()
		return nil, fmt.Errorf("no response from registry")
	}

	response := scanner.Text()
	parts := strings.Split(response, "|")
	if len(parts) < 2 || parts[0] != "ACK" {
		conn.Close()
		return nil, fmt.Errorf("registration failed: %s", response)
	}

	sessionID := parts[1]
	fmt.Printf("[client] Registered with session ID: %s\n", sessionID)

	client := &RegistryClientV2{
		conn:      conn,
		sessionID: sessionID,
		scanner:   scanner,
	}

	return client, nil
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
	return nil
}

// Close closes the connection
func (c *RegistryClientV2) Close() {
	c.conn.Close()
}
