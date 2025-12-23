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

// NewRegistryClientV2 connects to the registry and registers the service
func NewRegistryClientV2(registryAddr string, serviceName string, instanceName string, maintenancePort int, metadata map[string]interface{}) (*RegistryClientV2, error) {
	// Detect container IP first
	localIP := getContainerIP()
	if localIP == "" {
		return nil, fmt.Errorf("failed to detect container IP address")
	}
	fmt.Printf("[client] Detected container IP: %s\n", localIP)

	// Use IP as instance name if not provided
	if instanceName == "" {
		instanceName = localIP
	}

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
		localIP:   localIP,
		handlers:  make(map[EventType][]EventHandler),
	}

	// Emit connected event
	client.emit(Event{
		Type:      EventConnected,
		Data:      map[string]interface{}{"session_id": sessionID, "local_ip": localIP},
		Timestamp: time.Now(),
	})

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

// Close closes the connection
func (c *RegistryClientV2) Close() {
	c.conn.Close()
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
