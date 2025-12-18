package registry

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Service represents a registered service with persistent connection
type Service struct {
	Name            string
	Hostname        string
	Port            int
	MaintenancePort int
	Connection      net.Conn
	Connected       time.Time
	SessionID       string
	
	// Routes and config
	Routes  []ServiceRoute
	Headers map[string]string
	Options map[string]interface{}
}

// ServiceRoute represents a single route for a service
type ServiceRoute struct {
	Domains   []string
	Path      string
	Backend   string
	WebSocket bool
	Headers   map[string]string
}

// Registry manages service registrations and maintenance handshakes
type Registry struct {
	mu               sync.RWMutex
	services         map[string]*Service          // key: hostname:port
	sessions         map[string]*Service          // key: sessionID for reconnect
	disconnected     map[string]time.Time         // track unexpected disconnections
	port             int
	upstreamTimeout  time.Duration
	gracefulPeriod   time.Duration
	proxyServer      ProxyServer
	debug            bool
	maintenanceReqs  chan maintenanceRequest
	switchbackReqs   chan switchbackRequest
}

type ProxyServer interface {
	AddRoute(domains []string, path, backendURL string, headers map[string]string, websocket bool, options map[string]interface{}) error
	RemoveRoute(domains []string, path string)
}

type maintenanceRequest struct {
	service string
	port    int
}

type switchbackRequest struct {
	service string
	port    int
}

func NewRegistry(port int, timeout time.Duration, proxyServer ProxyServer, debug bool) *Registry {
	return &Registry{
		services:        make(map[string]*Service),
		sessions:        make(map[string]*Service),
		disconnected:    make(map[string]time.Time),
		port:            port,
		upstreamTimeout: timeout,
		gracefulPeriod:  5 * time.Minute,
		proxyServer:     proxyServer,
		debug:           debug,
		maintenanceReqs: make(chan maintenanceRequest, 10),
		switchbackReqs:  make(chan switchbackRequest, 10),
	}
}

func (r *Registry) Start(ctx context.Context) {
	go r.processMaintenanceRequests(ctx)
	go r.processSwitchbackRequests(ctx)
	go r.cleanupExpiredSessions(ctx)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", r.port))
	if err != nil {
		log.Fatalf("[registry] Failed to start listener: %s", err)
	}
	defer listener.Close()

	log.Printf("[registry] Service registry listening on port %d", r.port)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				if r.debug {
					log.Printf("[registry] Accept error: %s", err)
				}
				continue
			}
			go r.handleConnection(ctx, conn)
		}
	}
}

func (r *Registry) handleConnection(ctx context.Context, conn net.Conn) {
	scanner := bufio.NewScanner(conn)
	
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "|")
		
		if len(parts) == 0 {
			continue
		}

		command := parts[0]
		
		switch command {
		case "REGISTER":
			r.handleRegister(ctx, conn, parts)
			return // handleRegister takes over the connection
		case "RECONNECT":
			r.handleReconnect(ctx, conn, parts)
			return
		case "ROUTE":
			r.handleRoute(ctx, conn, parts, scanner)
		case "HEADER":
			r.handleHeader(ctx, conn, parts)
		case "OPTIONS":
			r.handleOptions(ctx, conn, parts)
		case "VALIDATE":
			r.handleValidate(ctx, conn, parts)
		case "SHUTDOWN":
			r.handleShutdown(ctx, conn, parts)
		case "MAINT_ENTER":
			r.handleMaintenanceEnter(conn, parts)
		case "MAINT_EXIT":
			r.handleMaintenanceExit(conn, parts)
		default:
			if r.debug {
				log.Printf("[registry] Unknown command: %s", command)
			}
		}
	}
}

func (r *Registry) handleRegister(ctx context.Context, conn net.Conn, parts []string) {
	// Format: REGISTER|service_name|hostname|service_port|maintenance_port
	if len(parts) != 5 {
		conn.Write([]byte("ERROR|Invalid format\n"))
		return
	}

	serviceName := parts[1]
	hostname := parts[2]
	var servicePort, maintPort int
	fmt.Sscanf(parts[3], "%d", &servicePort)
	fmt.Sscanf(parts[4], "%d", &maintPort)

	serviceKey := fmt.Sprintf("%s:%d", hostname, servicePort)
	sessionID := fmt.Sprintf("%s-%d-%d", hostname, servicePort, time.Now().Unix())

	service := &Service{
		Name:            serviceName,
		Hostname:        hostname,
		Port:            servicePort,
		MaintenancePort: maintPort,
		Connection:      conn,
		Connected:       time.Now(),
		SessionID:       sessionID,
		Routes:          make([]ServiceRoute, 0),
		Headers:         make(map[string]string),
		Options:         make(map[string]interface{}),
	}

	r.mu.Lock()
	r.services[serviceKey] = service
	r.sessions[sessionID] = service
	delete(r.disconnected, serviceKey) // Clear any pending disconnection
	r.mu.Unlock()

	log.Printf("[registry] Service registered: %s at %s (session: %s)", serviceName, serviceKey, sessionID)

	conn.Write([]byte(fmt.Sprintf("ACK|%s\n", sessionID)))

	r.monitorConnection(serviceKey, sessionID, serviceName, conn)
}

func (r *Registry) handleReconnect(ctx context.Context, conn net.Conn, parts []string) {
	// Format: RECONNECT|session_id
	if len(parts) != 2 {
		conn.Write([]byte("ERROR|Invalid format\n"))
		return
	}

	sessionID := parts[1]

	r.mu.Lock()
	service, exists := r.sessions[sessionID]
	r.mu.Unlock()

	if !exists {
		conn.Write([]byte("REREGISTER\n"))
		return
	}

	// Update connection
	service.Connection = conn
	service.Connected = time.Now()

	serviceKey := fmt.Sprintf("%s:%d", service.Hostname, service.Port)
	
	r.mu.Lock()
	r.services[serviceKey] = service
	delete(r.disconnected, serviceKey)
	r.mu.Unlock()

	log.Printf("[registry] Service reconnected: %s at %s", service.Name, serviceKey)

	conn.Write([]byte("OK\n"))

	r.monitorConnection(serviceKey, sessionID, service.Name, conn)
}

func (r *Registry) handleRoute(ctx context.Context, conn net.Conn, parts []string, scanner *bufio.Scanner) {
	// Format: ROUTE|session_id|domains|path|backend
	// domains can be comma-separated: domain1,domain2,domain3
	if len(parts) != 5 {
		conn.Write([]byte("ERROR|Invalid format\n"))
		return
	}

	sessionID := parts[1]
	domainsStr := parts[2]
	path := parts[3]
	backend := parts[4]

	domains := strings.Split(domainsStr, ",")
	for i := range domains {
		domains[i] = strings.TrimSpace(domains[i])
	}

	r.mu.Lock()
	service, exists := r.sessions[sessionID]
	r.mu.Unlock()

	if !exists {
		conn.Write([]byte("ERROR|Invalid session\n"))
		return
	}

	// Create route
	route := ServiceRoute{
		Domains: domains,
		Path:    path,
		Backend: backend,
	}

	// Add to service
	service.Routes = append(service.Routes, route)

	// Apply to proxy
	err := r.proxyServer.AddRoute(domains, path, backend, service.Headers, false, service.Options)
	if err != nil {
		conn.Write([]byte(fmt.Sprintf("ERROR|%s\n", err.Error())))
		return
	}

	log.Printf("[registry] Route added: %s at %v%s -> %s", service.Name, domains, path, backend)

	conn.Write([]byte("ROUTE_OK\n"))
}

func (r *Registry) handleHeader(ctx context.Context, conn net.Conn, parts []string) {
	// Format: HEADER|session_id|header_name|header_value
	if len(parts) != 4 {
		conn.Write([]byte("ERROR|Invalid format\n"))
		return
	}

	sessionID := parts[1]
	headerName := parts[2]
	headerValue := parts[3]

	r.mu.Lock()
	service, exists := r.sessions[sessionID]
	r.mu.Unlock()

	if !exists {
		conn.Write([]byte("ERROR|Invalid session\n"))
		return
	}

	// Add header to service
	service.Headers[headerName] = headerValue

	// Re-apply all routes with new headers
	for _, route := range service.Routes {
		r.proxyServer.AddRoute(route.Domains, route.Path, route.Backend, service.Headers, route.WebSocket, service.Options)
	}

	log.Printf("[registry] Header added: %s = %s for %s", headerName, headerValue, service.Name)

	conn.Write([]byte("HEADER_OK\n"))
}

func (r *Registry) handleOptions(ctx context.Context, conn net.Conn, parts []string) {
	// Format: OPTIONS|session_id|key|value
	if len(parts) != 4 {
		conn.Write([]byte("ERROR|Invalid format\n"))
		return
	}

	sessionID := parts[1]
	key := parts[2]
	value := parts[3]

	r.mu.Lock()
	service, exists := r.sessions[sessionID]
	r.mu.Unlock()

	if !exists {
		conn.Write([]byte("ERROR|Invalid session\n"))
		return
	}

	// Parse value based on key
	var parsedValue interface{} = value
	
	switch key {
	case "timeout", "health_check_interval", "health_check_timeout":
		if dur, err := time.ParseDuration(value); err == nil {
			parsedValue = dur
		}
	case "websocket", "compression", "http2", "http3":
		parsedValue = value == "true"
	}

	service.Options[key] = parsedValue

	// Re-apply all routes with new options
	for _, route := range service.Routes {
		r.proxyServer.AddRoute(route.Domains, route.Path, route.Backend, service.Headers, route.WebSocket, service.Options)
	}

	log.Printf("[registry] Option set: %s = %v for %s", key, parsedValue, service.Name)

	conn.Write([]byte("OPTIONS_OK\n"))
}

func (r *Registry) handleValidate(ctx context.Context, conn net.Conn, parts []string) {
	// Format: VALIDATE|session_id|hash
	if len(parts) != 3 {
		conn.Write([]byte("ERROR|Invalid format\n"))
		return
	}

	sessionID := parts[1]
	clientHash := parts[2]

	r.mu.RLock()
	service, exists := r.sessions[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|Invalid session\n"))
		return
	}

	// Calculate server hash from routes
	serverHash := r.calculateRoutesHash(service)
	
	if clientHash == serverHash {
		parity := r.calculateParity(serverHash)
		conn.Write([]byte(fmt.Sprintf("VALID|%d\n", parity)))
	} else {
		conn.Write([]byte(fmt.Sprintf("MISMATCH|%s\n", serverHash)))
	}
}

func (r *Registry) handleShutdown(ctx context.Context, conn net.Conn, parts []string) {
	// Format: SHUTDOWN|session_id
	if len(parts) != 2 {
		conn.Write([]byte("ERROR|Invalid format\n"))
		return
	}

	sessionID := parts[1]

	r.mu.Lock()
	service, exists := r.sessions[sessionID]
	if !exists {
		r.mu.Unlock()
		conn.Write([]byte("ERROR|Invalid session\n"))
		return
	}

	serviceKey := fmt.Sprintf("%s:%d", service.Hostname, service.Port)
	
	// Remove all routes
	for _, route := range service.Routes {
		r.proxyServer.RemoveRoute(route.Domains, route.Path)
	}
	
	// Graceful shutdown - remove immediately
	delete(r.services, serviceKey)
	delete(r.sessions, sessionID)
	delete(r.disconnected, serviceKey)
	r.mu.Unlock()

	log.Printf("[registry] Graceful shutdown: %s at %s (routes removed immediately)", service.Name, serviceKey)
	
	conn.Write([]byte("SHUTDOWN_OK\n"))
	conn.Close()
}

func (r *Registry) monitorConnection(serviceKey, sessionID, serviceName string, conn net.Conn) {
	buf := make([]byte, 1)
	_, err := conn.Read(buf)

	r.mu.Lock()
	delete(r.services, serviceKey)
	r.disconnected[serviceKey] = time.Now()
	r.mu.Unlock()

	if err != nil {
		log.Printf("[registry] Service unexpectedly disconnected: %s at %s (config retained for %v)", serviceName, serviceKey, r.gracefulPeriod)
	} else {
		log.Printf("[registry] Service connection closed: %s at %s (config retained for %v)", serviceName, serviceKey, r.gracefulPeriod)
	}
}

func (r *Registry) handleMaintenanceEnter(conn net.Conn, parts []string) {
	// Format: MAINT_ENTER|hostname:port|maintenance_port
	if len(parts) != 3 {
		return
	}

	service := parts[1]
	var maintPort int
	fmt.Sscanf(parts[2], "%d", &maintPort)

	conn.Write([]byte("ACK\n"))

	r.maintenanceReqs <- maintenanceRequest{service: service, port: maintPort}
}

func (r *Registry) handleMaintenanceExit(conn net.Conn, parts []string) {
	// Format: MAINT_EXIT|hostname:port
	if len(parts) != 2 {
		return
	}

	service := parts[1]
	parts2 := strings.Split(service, ":")
	var port int
	if len(parts2) == 2 {
		fmt.Sscanf(parts2[1], "%d", &port)
	}

	conn.Write([]byte("ACK\n"))

	r.switchbackReqs <- switchbackRequest{service: service, port: port + 1}
}

func (r *Registry) processMaintenanceRequests(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-r.maintenanceReqs:
			go r.approveMaintenanceMode(req.service, req.port)
		}
	}
}

func (r *Registry) processSwitchbackRequests(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-r.switchbackReqs:
			go r.approveSwitchback(req.service, req.port)
		}
	}
}

func (r *Registry) approveMaintenanceMode(service string, maintPort int) {
	parts := strings.Split(service, ":")
	if len(parts) != 2 {
		return
	}

	hostname := parts[0]
	log.Printf("[registry] Approving maintenance mode for %s (port %d)", service, maintPort)

	// TODO: Disable routes for this service in proxy

	time.Sleep(2 * time.Second)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", hostname, maintPort), r.upstreamTimeout)
	if err != nil {
		if r.debug {
			log.Printf("[registry] Failed to connect to maintenance server: %s", err)
		}
		return
	}
	defer conn.Close()

	conn.Write([]byte("MAINT_APPROVED\n"))
	
	buf := make([]byte, 4)
	conn.SetReadDeadline(time.Now().Add(r.upstreamTimeout))
	conn.Read(buf)

	log.Printf("[registry] Maintenance mode approved for %s", service)
}

func (r *Registry) approveSwitchback(service string, maintPort int) {
	parts := strings.Split(service, ":")
	if len(parts) != 2 {
		return
	}

	hostname := parts[0]
	log.Printf("[registry] Approving switchback for %s (port %d)", service, maintPort)

	// TODO: Re-enable routes for this service in proxy

	time.Sleep(2 * time.Second)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", hostname, maintPort), r.upstreamTimeout)
	if err != nil {
		if r.debug {
			log.Printf("[registry] Failed to connect to maintenance server: %s", err)
		}
		return
	}
	defer conn.Close()

	conn.Write([]byte("SWITCHBACK_APPROVED\n"))
	
	buf := make([]byte, 4)
	conn.SetReadDeadline(time.Now().Add(r.upstreamTimeout))
	conn.Read(buf)

	log.Printf("[registry] Switchback approved for %s", service)
}

func (r *Registry) cleanupExpiredSessions(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.Lock()
			now := time.Now()
			for serviceKey, disconnectTime := range r.disconnected {
				if now.Sub(disconnectTime) > r.gracefulPeriod {
					for sessionID, service := range r.sessions {
						if fmt.Sprintf("%s:%d", service.Hostname, service.Port) == serviceKey {
							// Remove all routes
							for _, route := range service.Routes {
								r.proxyServer.RemoveRoute(route.Domains, route.Path)
							}
							
							delete(r.sessions, sessionID)
							log.Printf("[registry] Expired session cleanup: %s at %s (disconnected %v ago)", service.Name, serviceKey, now.Sub(disconnectTime))
							break
						}
					}
					delete(r.disconnected, serviceKey)
				}
			}
			r.mu.Unlock()
		}
	}
}

func (r *Registry) NotifyShutdown() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	log.Printf("[registry] Notifying %d connected services of shutdown", len(r.services))
	
	for serviceKey, service := range r.services {
		if service.Connection != nil {
			_, err := service.Connection.Write([]byte("SHUTDOWN\n"))
			if err != nil {
				log.Printf("[registry] Failed to notify %s of shutdown: %s", serviceKey, err)
			} else {
				log.Printf("[registry] Shutdown notification sent to %s", serviceKey)
			}
		}
	}
}

func (r *Registry) IsRegistered(hostname string, port int) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s:%d", hostname, port)
	_, exists := r.services[key]
	return exists
}

func (r *Registry) GetMaintenancePort(hostname string, port int) (int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s:%d", hostname, port)
	if service, exists := r.services[key]; exists {
		return service.MaintenancePort, true
	}
	return port + 1, false
}

func (r *Registry) calculateRoutesHash(service *Service) string {
	var data strings.Builder
	
	for _, route := range service.Routes {
		data.WriteString(strings.Join(route.Domains, ","))
		data.WriteString(route.Path)
		data.WriteString(route.Backend)
	}
	
	for k, v := range service.Headers {
		data.WriteString(k)
		data.WriteString(v)
	}
	
	hash := sha256.Sum256([]byte(data.String()))
	return hex.EncodeToString(hash[:])
}

func (r *Registry) calculateParity(hash string) int {
	ones := 0
	for _, c := range hash {
		switch c {
		case '1', '3', '5', '7', '9', 'b', 'd', 'f':
			ones++
		}
	}
	if ones%2 == 0 {
		return 0
	}
	return 1
}
