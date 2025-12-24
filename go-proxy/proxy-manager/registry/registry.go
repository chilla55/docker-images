package registry

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/chilla55/proxy-manager/proxy"
)

// V2 Protocol - Complete Production Implementation

// ProxyServer interface for adding/removing routes
type ProxyServer interface {
	AddRoute(domains []string, path, backendURL string, headers map[string]string, websocket bool, options map[string]interface{}) error
	RemoveRoute(domains []string, path string)
	SetRouteEnabled(domains []string, path string, enabled bool)
	GetBackendStatus(domain, path string) *proxy.BackendStatus
	SetMaintenance(domains []string, path string, enabled bool, maintenancePageURL string) error
	StartDrain(domains []string, path string, duration time.Duration) error
	CancelDrain(domains []string, path string) error
}

// HealthChecker interface for backend health monitoring
type HealthChecker interface {
	AddService(name, url string, interval, timeout time.Duration, expectedStatus int)
	RemoveService(name string)
}

// RouteID is a unique identifier for a route
type RouteID string

// SessionID is a unique identifier for a service session
type SessionID string

// ServiceV2 represents a service with v2 protocol support
type ServiceV2 struct {
	SessionID       SessionID
	ServiceName     string
	InstanceName    string
	MaintenancePort int
	Metadata        map[string]interface{}
	Connection      net.Conn
	ConnectedAt     time.Time
	LastActivity    time.Time
	DisconnectedAt  *time.Time // When connection was lost (nil if connected)

	// Active configuration
	mu                sync.RWMutex
	activeRoutes      map[RouteID]*RouteV2
	routesDeactivated bool // Routes removed from proxy but kept in session
	activeHeaders     map[string]string
	activeOptions     map[string]interface{}
	activeHealth      map[RouteID]*HealthCheckV2
	activeRateLimit   map[RouteID]*RateLimitV2
	activeCircuit     map[RouteID]*CircuitBreakerV2

	// Staged configuration (pending CONFIG_APPLY)
	stagedRoutes    map[RouteID]*RouteV2
	stagedHeaders   map[string]string
	stagedOptions   map[string]interface{}
	stagedHealth    map[RouteID]*HealthCheckV2
	stagedRateLimit map[RouteID]*RateLimitV2
	stagedCircuit   map[RouteID]*CircuitBreakerV2
	stagedRemovals  map[RouteID]bool // routes staged for removal

	// State
	maintenanceRoutes map[RouteID]bool
	draining          bool
	drainStart        time.Time
	drainDuration     time.Duration
	subscriptions     map[string]bool
	stagedTimeout     time.Time
}

// RouteV2 represents a route in v2 protocol
type RouteV2 struct {
	RouteID      RouteID
	Domains      []string
	Path         string
	BackendURL   string
	Priority     int
	CreatedAt    time.Time
	LastModified time.Time
}

// HealthCheckV2 represents health check config for a route
type HealthCheckV2 struct {
	RouteID  RouteID
	Path     string
	Interval time.Duration
	Timeout  time.Duration
}

// RateLimitV2 represents rate limit config
type RateLimitV2 struct {
	RouteID  RouteID
	Requests int
	Window   time.Duration
}

// CircuitBreakerV2 represents circuit breaker config
type CircuitBreakerV2 struct {
	RouteID          RouteID
	Threshold        int
	Timeout          time.Duration
	HalfOpenRequests int
	State            string
	Failures         int
	LastFailureTime  time.Time
	NextAttemptTime  time.Time
}

// StatsV2 represents route statistics
type StatsV2 struct {
	RouteID       RouteID       `json:"route_id"`
	Requests      int64         `json:"requests"`
	Errors        int64         `json:"errors"`
	AvgLatencyMs  float64       `json:"avg_latency_ms"`
	P95LatencyMs  float64       `json:"p95_latency_ms"`
	P99LatencyMs  float64       `json:"p99_latency_ms"`
	BytesSent     int64         `json:"bytes_sent"`
	BytesReceived int64         `json:"bytes_received"`
	StatusCodes   map[int]int64 `json:"status_codes"`
}

// RegistryV2 is the v2 protocol registry
type RegistryV2 struct {
	mu               sync.RWMutex
	services         map[SessionID]*ServiceV2
	sessionsByConn   map[net.Conn]SessionID
	port             int
	proxyServer      ProxyServer
	healthChecker    HealthChecker
	debug            bool
	nextRouteID      int64
	stats            map[RouteID]*StatsV2
	stagedConfigTTL  time.Duration
	upstreamTimeout  time.Duration
	reconnectTimeout time.Duration // How long to keep routes after disconnect

	// Maintenance verification tasks
	maintTasks    chan *maintenanceTask
	maintCancel   map[SessionID]context.CancelFunc
	maintCancelMu sync.Mutex
}

// maintenanceTask represents a task to verify maintenance URL or backend health
type maintenanceTask struct {
	sessionID SessionID
	target    string
	url       string
	isEnter   bool // true for MAINT_ENTER, false for MAINT_EXIT
	conn      net.Conn
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewRegistryV2 creates a new v2 registry
func NewRegistryV2(port int, proxyServer ProxyServer, debug bool, upstreamTimeout time.Duration, healthChecker HealthChecker) *RegistryV2 {
	r := &RegistryV2{
		services:         make(map[SessionID]*ServiceV2),
		sessionsByConn:   make(map[net.Conn]SessionID),
		port:             port,
		proxyServer:      proxyServer,
		healthChecker:    healthChecker,
		debug:            debug,
		nextRouteID:      1,
		stats:            make(map[RouteID]*StatsV2),
		stagedConfigTTL:  30 * time.Minute,
		upstreamTimeout:  upstreamTimeout,
		reconnectTimeout: 5 * time.Minute, // Grace period for reconnection (matches client retry strategy)
		maintTasks:       make(chan *maintenanceTask, 100),
		maintCancel:      make(map[SessionID]context.CancelFunc),
	}

	// Start maintenance verification workers
	for i := 0; i < 5; i++ {
		go r.maintenanceVerificationWorker()
	}

	return r
}

// StartV2 starts the v2 registry listener
func (r *RegistryV2) StartV2(ctx context.Context) {
	go r.cleanupExpiredStagedConfigs(ctx)
	go r.cleanupDisconnectedSessions(ctx)

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", r.port))
	if err != nil {
		log.Fatalf("[registry-v2] Failed to start listener: %s", err)
	}
	defer listener.Close()

	log.Printf("[registry-v2] Service registry v2 listening on port %d", r.port)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				if r.debug {
					log.Printf("[registry-v2] Accept error: %s", err)
				}
				continue
			}

			// Enable TCP keepalive
			if tcpConn, ok := conn.(*net.TCPConn); ok {
				_ = tcpConn.SetKeepAlive(true)
				_ = tcpConn.SetKeepAlivePeriod(30 * time.Second)
			}

			go r.handleConnectionV2(ctx, conn)
		}
	}
}

func (r *RegistryV2) handleConnectionV2(ctx context.Context, conn net.Conn) {
	// Close connection promptly if context is cancelled
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	defer func() {
		r.mu.Lock()
		if sid, ok := r.sessionsByConn[conn]; ok {
			delete(r.sessionsByConn, conn)
			r.mu.Unlock()

			r.mu.RLock()
			svc, exists := r.services[sid]
			r.mu.RUnlock()

			if exists {
				// Mark as disconnected and disable routes (keep them but mark as disabled)
				now := time.Now()
				svc.mu.Lock()
				svc.DisconnectedAt = &now
				svc.Connection = nil

				// Disable routes in proxy but keep them in activeRoutes for potential reconnect
				if !svc.routesDeactivated {
					for routeID, route := range svc.activeRoutes {
						r.proxyServer.SetRouteEnabled(route.Domains, route.Path, false)
						log.Printf("[registry-v2] Disabled route %s: %v%s (service disconnected)", routeID, route.Domains, route.Path)
					}
					svc.routesDeactivated = true
				}
				svc.mu.Unlock()

				log.Printf("[registry-v2] Connection lost for session %s (%s) - routes disabled, session valid for %v for potential reconnect",
					sid, svc.ServiceName, r.reconnectTimeout)
			}
		} else {
			r.mu.Unlock()
		}
		conn.Close()
	}()

	scanner := bufio.NewScanner(conn)
	var sessionID SessionID

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) == 0 {
			continue
		}

		command := parts[0]

		// Commands that don't require session
		if command == "REGISTER" {
			sid, err := r.handleRegisterV2(conn, parts)
			if err == nil {
				sessionID = sid
				r.mu.Lock()
				r.sessionsByConn[conn] = sessionID
				r.mu.Unlock()
			}
			continue
		}

		// All other commands require session
		if sessionID == "" {
			conn.Write([]byte("ERROR|no session\n"))
			continue
		}

		// Update last activity
		r.mu.RLock()
		svc, _ := r.services[sessionID]
		r.mu.RUnlock()
		if svc != nil {
			svc.mu.Lock()
			svc.LastActivity = time.Now()
			svc.mu.Unlock()
		}

		// Session-scoped commands
		switch command {
		case "RECONNECT":
			r.handleReconnectV2(conn, sessionID, parts)
		case "PING":
			conn.Write([]byte("PONG\n"))
		case "SESSION_INFO":
			r.handleSessionInfoV2(conn, sessionID)
		case "ROUTE_ADD":
			r.handleRouteAddV2(conn, sessionID, parts)
		case "ROUTE_ADD_BULK":
			r.handleRouteAddBulkV2(conn, sessionID, parts)
		case "ROUTE_UPDATE":
			r.handleRouteUpdateV2(conn, sessionID, parts)
		case "ROUTE_REMOVE":
			r.handleRouteRemoveV2(conn, sessionID, parts)
		case "ROUTE_LIST":
			r.handleRouteListV2(conn, sessionID, parts)
		case "HEADERS_SET":
			r.handleHeadersSetV2(conn, sessionID, parts)
		case "HEADERS_REMOVE":
			r.handleHeadersRemoveV2(conn, sessionID, parts)
		case "OPTIONS_SET":
			r.handleOptionsSetV2(conn, sessionID, parts)
		case "OPTIONS_REMOVE":
			r.handleOptionsRemoveV2(conn, sessionID, parts)
		case "HEALTH_SET":
			r.handleHealthSetV2(conn, sessionID, parts)
		case "RATELIMIT_SET":
			r.handleRateLimitSetV2(conn, sessionID, parts)
		case "CIRCUIT_BREAKER_SET":
			r.handleCircuitBreakerSetV2(conn, sessionID, parts)
		case "CIRCUIT_BREAKER_STATUS":
			r.handleCircuitBreakerStatusV2(conn, sessionID, parts)
		case "CIRCUIT_BREAKER_RESET":
			r.handleCircuitBreakerResetV2(conn, sessionID, parts)
		case "CONFIG_VALIDATE":
			r.handleConfigValidateV2(conn, sessionID)
		case "CONFIG_APPLY":
			r.handleConfigApplyV2(conn, sessionID)
		case "CONFIG_ROLLBACK":
			r.handleConfigRollbackV2(conn, sessionID)
		case "CONFIG_DIFF":
			r.handleConfigDiffV2(conn, sessionID)
		case "CONFIG_APPLY_PARTIAL":
			r.handleConfigApplyPartialV2(conn, sessionID, parts)
		case "STATS_GET":
			r.handleStatsGetV2(conn, sessionID, parts)
		case "BACKEND_TEST":
			r.handleBackendTestV2(conn, sessionID, parts)
		case "DRAIN_START":
			r.handleDrainStartV2(conn, sessionID, parts)
		case "DRAIN_STATUS":
			r.handleDrainStatusV2(conn, sessionID)
		case "DRAIN_CANCEL":
			r.handleDrainCancelV2(conn, sessionID)
		case "SUBSCRIBE":
			r.handleSubscribeV2(conn, sessionID, parts)
		case "UNSUBSCRIBE":
			r.handleUnsubscribeV2(conn, sessionID, parts)
		case "MAINT_ENTER":
			r.handleMaintenanceEnterV2(conn, sessionID, parts)
		case "MAINT_EXIT":
			r.handleMaintenanceExitV2(conn, sessionID, parts)
		case "MAINT_STATUS":
			r.handleMaintenanceStatusV2(conn, sessionID)
		case "CLIENT_SHUTDOWN":
			r.handleClientShutdownV2(conn, sessionID)
		default:
			conn.Write([]byte(fmt.Sprintf("ERROR|unknown command: %s\n", command)))
		}
	}
}

// Handler implementations

func (r *RegistryV2) handleRegisterV2(conn net.Conn, parts []string) (SessionID, error) {
	// REGISTER|service_name|instance_name|maintenance_port|metadata
	if len(parts) < 4 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return "", fmt.Errorf("invalid format")
	}

	serviceName := parts[1]
	instanceName := parts[2]
	var maintenancePort int
	fmt.Sscanf(parts[3], "%d", &maintenancePort)

	metadata := make(map[string]interface{})
	if len(parts) > 4 && parts[4] != "" && parts[4] != "{}" {
		if err := json.Unmarshal([]byte(parts[4]), &metadata); err != nil {
			conn.Write([]byte("ERROR|invalid metadata json\n"))
			return "", err
		}
	}

	// Cleanup old sessions for the same service/instance (handles fast restarts)
	r.mu.Lock()
	var oldSessions []SessionID
	for sid, svc := range r.services {
		if svc.ServiceName == serviceName && svc.InstanceName == instanceName {
			oldSessions = append(oldSessions, sid)
		}
	}
	r.mu.Unlock()

	// Remove routes from old sessions
	for _, oldSID := range oldSessions {
		r.mu.RLock()
		oldSvc, exists := r.services[oldSID]
		r.mu.RUnlock()

		if exists {
			oldSvc.mu.Lock()
			log.Printf("[registry-v2] Cleaning up old session %s for %s/%s", oldSID, serviceName, instanceName)
			// Remove all active routes from proxy (only if not already deactivated)
			if !oldSvc.routesDeactivated {
				for routeID, route := range oldSvc.activeRoutes {
					r.proxyServer.RemoveRoute(route.Domains, route.Path)
					log.Printf("[registry-v2] Removed old route %s: %v%s", routeID, route.Domains, route.Path)
				}
			} else {
				log.Printf("[registry-v2] Old session routes already deactivated, skipping removal")
			}
			oldSvc.mu.Unlock()

			// Remove old session
			r.mu.Lock()
			delete(r.services, oldSID)
			// Also cleanup sessionsByConn if present
			for c, sid := range r.sessionsByConn {
				if sid == oldSID {
					delete(r.sessionsByConn, c)
				}
			}
			r.mu.Unlock()
		}
	}

	sessionID := SessionID(fmt.Sprintf("%s-%d-%d", instanceName, time.Now().Unix(), rand64()))

	service := &ServiceV2{
		SessionID:         sessionID,
		ServiceName:       serviceName,
		InstanceName:      instanceName,
		MaintenancePort:   maintenancePort,
		Metadata:          metadata,
		Connection:        conn,
		ConnectedAt:       time.Now(),
		LastActivity:      time.Now(),
		activeRoutes:      make(map[RouteID]*RouteV2),
		activeHeaders:     make(map[string]string),
		activeOptions:     make(map[string]interface{}),
		activeHealth:      make(map[RouteID]*HealthCheckV2),
		activeRateLimit:   make(map[RouteID]*RateLimitV2),
		activeCircuit:     make(map[RouteID]*CircuitBreakerV2),
		stagedRoutes:      make(map[RouteID]*RouteV2),
		stagedHeaders:     make(map[string]string),
		stagedOptions:     make(map[string]interface{}),
		stagedHealth:      make(map[RouteID]*HealthCheckV2),
		stagedRateLimit:   make(map[RouteID]*RateLimitV2),
		stagedCircuit:     make(map[RouteID]*CircuitBreakerV2),
		stagedRemovals:    make(map[RouteID]bool),
		maintenanceRoutes: make(map[RouteID]bool),
		subscriptions:     make(map[string]bool),
		stagedTimeout:     time.Now().Add(r.stagedConfigTTL),
	}

	r.mu.Lock()
	r.services[sessionID] = service
	r.mu.Unlock()

	if len(oldSessions) > 0 {
		log.Printf("[registry-v2] Service re-registered (cleaned up %d old session(s)): %s/%s (new session: %s)", len(oldSessions), serviceName, instanceName, sessionID)
	} else {
		log.Printf("[registry-v2] Service registered: %s/%s (session: %s)", serviceName, instanceName, sessionID)
	}
	conn.Write([]byte(fmt.Sprintf("ACK|%s\n", sessionID)))

	return sessionID, nil
}

func (r *RegistryV2) handleReconnectV2(conn net.Conn, sessionID SessionID, parts []string) {
	_ = parts
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("REREGISTER\n"))
		return
	}

	// Restore connection and clear disconnected state
	svc.mu.Lock()
	svc.Connection = conn
	svc.DisconnectedAt = nil
	svc.LastActivity = time.Now()

	// Re-enable routes if they were deactivated
	if svc.routesDeactivated {
		for routeID, route := range svc.activeRoutes {
			r.proxyServer.SetRouteEnabled(route.Domains, route.Path, true)
			log.Printf("[registry-v2] Re-enabled route %s: %v%s -> %s", routeID, route.Domains, route.Path, route.BackendURL)
		}
		svc.routesDeactivated = false
	}
	svc.mu.Unlock()

	r.mu.Lock()
	r.sessionsByConn[conn] = sessionID
	r.mu.Unlock()

	log.Printf("[registry-v2] Session %s (%s) reconnected successfully", sessionID, svc.ServiceName)
	conn.Write([]byte("OK\n"))
}

func (r *RegistryV2) handleSessionInfoV2(conn net.Conn, sessionID SessionID) {
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.RLock()
	info := map[string]interface{}{
		"session_id":     string(sessionID),
		"service_name":   svc.ServiceName,
		"instance_name":  svc.InstanceName,
		"connected_at":   svc.ConnectedAt.Format(time.RFC3339),
		"uptime_seconds": int(time.Since(svc.ConnectedAt).Seconds()),
		"routes_active":  len(svc.activeRoutes),
		"routes_staged":  len(svc.stagedRoutes),
		"last_activity":  svc.LastActivity.Format(time.RFC3339),
		"metadata":       svc.Metadata,
	}
	svc.mu.RUnlock()

	data, _ := json.Marshal(info)
	conn.Write([]byte(fmt.Sprintf("SESSION_OK|%s\n", string(data))))
}

func (r *RegistryV2) handleRouteAddV2(conn net.Conn, sessionID SessionID, parts []string) {
	// ROUTE_ADD|session_id|domains|path|backend_url|priority
	if len(parts) < 6 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	domains := strings.Split(parts[2], ",")
	for i := range domains {
		domains[i] = strings.TrimSpace(domains[i])
	}
	path := parts[3]
	backendURL := parts[4]
	var priority int
	fmt.Sscanf(parts[5], "%d", &priority)

	if err := validateRoute(domains, path, backendURL); err != nil {
		conn.Write([]byte(fmt.Sprintf("ERROR|%s\n", err)))
		return
	}

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	routeID := r.generateRouteID()

	svc.mu.Lock()
	svc.stagedRoutes[routeID] = &RouteV2{
		RouteID:      routeID,
		Domains:      domains,
		Path:         path,
		BackendURL:   backendURL,
		Priority:     priority,
		CreatedAt:    time.Now(),
		LastModified: time.Now(),
	}
	svc.stagedTimeout = time.Now().Add(r.stagedConfigTTL)
	svc.mu.Unlock()

	conn.Write([]byte(fmt.Sprintf("ROUTE_OK|%s\n", routeID)))
}

func (r *RegistryV2) handleRouteAddBulkV2(conn net.Conn, sessionID SessionID, parts []string) {
	// ROUTE_ADD_BULK|session_id|json_array
	if len(parts) < 3 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	var routes []map[string]interface{}
	if err := json.Unmarshal([]byte(parts[2]), &routes); err != nil {
		conn.Write([]byte("ERROR|invalid json\n"))
		return
	}

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	results := make([]map[string]string, 0)

	for _, route := range routes {
		domains := parseStringArray(route["domains"])
		path, _ := route["path"].(string)
		backendURL, _ := route["backend_url"].(string)
		priority := int(getFloat64(route["priority"]))

		if err := validateRoute(domains, path, backendURL); err != nil {
			svc.mu.Unlock()
			conn.Write([]byte(fmt.Sprintf("ERROR|%s\n", err)))
			return
		}

		routeID := r.generateRouteID()
		svc.stagedRoutes[routeID] = &RouteV2{
			RouteID:      routeID,
			Domains:      domains,
			Path:         path,
			BackendURL:   backendURL,
			Priority:     priority,
			CreatedAt:    time.Now(),
			LastModified: time.Now(),
		}

		results = append(results, map[string]string{
			"route_id": string(routeID),
			"status":   "ok",
		})
	}

	svc.stagedTimeout = time.Now().Add(r.stagedConfigTTL)
	svc.mu.Unlock()

	data, _ := json.Marshal(results)
	conn.Write([]byte(fmt.Sprintf("ROUTE_BULK_OK|%s\n", string(data))))
}

func (r *RegistryV2) handleRouteUpdateV2(conn net.Conn, sessionID SessionID, parts []string) {
	// ROUTE_UPDATE|session_id|route_id|field|value
	if len(parts) < 5 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	routeID := RouteID(parts[2])
	field := parts[3]
	value := parts[4]

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	route, found := svc.stagedRoutes[routeID]
	if !found {
		route, found = svc.activeRoutes[routeID]
		if !found {
			svc.mu.Unlock()
			conn.Write([]byte("ERROR|route not found\n"))
			return
		}
		// Copy active to staged for modification
		newRoute := *route
		svc.stagedRoutes[routeID] = &newRoute
		route = &newRoute
	}

	switch field {
	case "backend_url":
		if !strings.Contains(value, "://") {
			svc.mu.Unlock()
			conn.Write([]byte("ERROR|invalid backend url\n"))
			return
		}
		route.BackendURL = value
	case "priority":
		fmt.Sscanf(value, "%d", &route.Priority)
	case "domains":
		domains := strings.Split(value, ",")
		for i := range domains {
			domains[i] = strings.TrimSpace(domains[i])
		}
		route.Domains = domains
	case "path":
		route.Path = value
	default:
		svc.mu.Unlock()
		conn.Write([]byte("ERROR|unknown field\n"))
		return
	}

	route.LastModified = time.Now()
	svc.stagedTimeout = time.Now().Add(r.stagedConfigTTL)
	svc.mu.Unlock()

	conn.Write([]byte("ROUTE_OK\n"))
}

func (r *RegistryV2) handleRouteRemoveV2(conn net.Conn, sessionID SessionID, parts []string) {
	// ROUTE_REMOVE|session_id|route_id
	if len(parts) < 3 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	routeID := RouteID(parts[2])

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	_, stagFound := svc.stagedRoutes[routeID]
	_, activeFound := svc.activeRoutes[routeID]

	if !stagFound && !activeFound {
		svc.mu.Unlock()
		conn.Write([]byte("ERROR|route not found\n"))
		return
	}

	delete(svc.stagedRoutes, routeID)
	svc.stagedRemovals[routeID] = true
	svc.stagedTimeout = time.Now().Add(r.stagedConfigTTL)
	svc.mu.Unlock()

	conn.Write([]byte("ROUTE_OK\n"))
}

func (r *RegistryV2) handleRouteListV2(conn net.Conn, sessionID SessionID, parts []string) {
	_ = parts
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.RLock()
	result := make([]map[string]interface{}, 0)

	for rid, route := range svc.activeRoutes {
		result = append(result, map[string]interface{}{
			"route_id": string(rid),
			"domains":  route.Domains,
			"path":     route.Path,
			"backend":  route.BackendURL,
			"priority": route.Priority,
			"status":   "active",
		})
	}

	for rid, route := range svc.stagedRoutes {
		result = append(result, map[string]interface{}{
			"route_id": string(rid),
			"domains":  route.Domains,
			"path":     route.Path,
			"backend":  route.BackendURL,
			"priority": route.Priority,
			"status":   "staged",
		})
	}

	svc.mu.RUnlock()

	data, _ := json.Marshal(result)
	conn.Write([]byte(fmt.Sprintf("ROUTE_LIST_OK|%s\n", string(data))))
}

func (r *RegistryV2) handleHeadersSetV2(conn net.Conn, sessionID SessionID, parts []string) {
	// HEADERS_SET|session_id|target|header_name|header_value
	if len(parts) < 5 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	target := parts[2]
	name := parts[3]
	value := parts[4]

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	if target == "ALL" {
		svc.stagedHeaders[name] = value
	} else {
		// Route-specific headers would go here (future enhancement)
		svc.mu.Unlock()
		conn.Write([]byte("ERROR|route-specific headers not yet supported\n"))
		return
	}
	svc.stagedTimeout = time.Now().Add(r.stagedConfigTTL)
	svc.mu.Unlock()

	conn.Write([]byte("HEADERS_OK\n"))
}

func (r *RegistryV2) handleHeadersRemoveV2(conn net.Conn, sessionID SessionID, parts []string) {
	// HEADERS_REMOVE|session_id|target|header_name
	if len(parts) < 4 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	target := parts[2]
	name := parts[3]

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	if target == "ALL" {
		delete(svc.stagedHeaders, name)
		svc.stagedTimeout = time.Now().Add(r.stagedConfigTTL)
		svc.mu.Unlock()
		conn.Write([]byte("HEADERS_OK\n"))
	} else {
		svc.mu.Unlock()
		conn.Write([]byte("ERROR|route-specific headers not yet supported\n"))
	}
}

func (r *RegistryV2) handleOptionsSetV2(conn net.Conn, sessionID SessionID, parts []string) {
	// OPTIONS_SET|session_id|target|key|value
	if len(parts) < 5 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	target := parts[2]
	key := parts[3]
	value := parts[4]

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	if target == "ALL" {
		// Parse value based on key
		var parsed interface{} = value
		switch key {
		case "timeout", "health_check_interval", "health_check_timeout":
			parsed = parseDuration(value)
		case "websocket", "compression", "http2", "http3":
			parsed = value == "true"
		}
		svc.stagedOptions[key] = parsed
		svc.stagedTimeout = time.Now().Add(r.stagedConfigTTL)
		svc.mu.Unlock()
		conn.Write([]byte("OPTIONS_OK\n"))
	} else {
		svc.mu.Unlock()
		conn.Write([]byte("ERROR|route-specific options not yet supported\n"))
	}
}

func (r *RegistryV2) handleOptionsRemoveV2(conn net.Conn, sessionID SessionID, parts []string) {
	// OPTIONS_REMOVE|session_id|target|key
	if len(parts) < 4 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	target := parts[2]
	key := parts[3]

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	if target == "ALL" {
		delete(svc.stagedOptions, key)
		svc.stagedTimeout = time.Now().Add(r.stagedConfigTTL)
		svc.mu.Unlock()
		conn.Write([]byte("OPTIONS_OK\n"))
	} else {
		svc.mu.Unlock()
		conn.Write([]byte("ERROR|route-specific options not yet supported\n"))
	}
}

func (r *RegistryV2) handleHealthSetV2(conn net.Conn, sessionID SessionID, parts []string) {
	// HEALTH_SET|session_id|route_id|path|interval|timeout
	if len(parts) < 6 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	routeID := RouteID(parts[2])
	path := parts[3]
	interval := parseDuration(parts[4])
	timeout := parseDuration(parts[5])

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	svc.stagedHealth[routeID] = &HealthCheckV2{
		RouteID:  routeID,
		Path:     path,
		Interval: interval,
		Timeout:  timeout,
	}
	svc.stagedTimeout = time.Now().Add(r.stagedConfigTTL)
	svc.mu.Unlock()

	conn.Write([]byte("HEALTH_OK\n"))
}

func (r *RegistryV2) handleRateLimitSetV2(conn net.Conn, sessionID SessionID, parts []string) {
	// RATELIMIT_SET|session_id|route_id|requests|window
	if len(parts) < 5 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	routeID := RouteID(parts[2])
	var requests int
	fmt.Sscanf(parts[3], "%d", &requests)
	window := parseDuration(parts[4])

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	svc.stagedRateLimit[routeID] = &RateLimitV2{
		RouteID:  routeID,
		Requests: requests,
		Window:   window,
	}
	svc.stagedTimeout = time.Now().Add(r.stagedConfigTTL)
	svc.mu.Unlock()

	conn.Write([]byte("RATELIMIT_OK\n"))
}

func (r *RegistryV2) handleCircuitBreakerSetV2(conn net.Conn, sessionID SessionID, parts []string) {
	// CIRCUIT_BREAKER_SET|session_id|route_id|threshold|timeout|half_open_requests
	if len(parts) < 6 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	routeID := RouteID(parts[2])
	var threshold, halfOpen int
	fmt.Sscanf(parts[3], "%d", &threshold)
	timeout := parseDuration(parts[4])
	fmt.Sscanf(parts[5], "%d", &halfOpen)

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	svc.stagedCircuit[routeID] = &CircuitBreakerV2{
		RouteID:          routeID,
		Threshold:        threshold,
		Timeout:          timeout,
		HalfOpenRequests: halfOpen,
		State:            "closed",
	}
	svc.stagedTimeout = time.Now().Add(r.stagedConfigTTL)
	svc.mu.Unlock()

	conn.Write([]byte("CIRCUIT_OK\n"))
}

func (r *RegistryV2) handleCircuitBreakerStatusV2(conn net.Conn, sessionID SessionID, parts []string) {
	// CIRCUIT_BREAKER_STATUS|session_id|route_id
	if len(parts) < 3 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	routeID := RouteID(parts[2])

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.RLock()
	route, found := svc.activeRoutes[routeID]
	svc.mu.RUnlock()

	if !found {
		conn.Write([]byte("ERROR|route not found\n"))
		return
	}

	// Query actual backend status from proxy
	backendStatus := r.proxyServer.GetBackendStatus(route.Domains[0], route.Path)
	if backendStatus == nil {
		conn.Write([]byte("ERROR|backend status unavailable\n"))
		return
	}

	status := map[string]interface{}{
		"state":          backendStatus.CircuitState,
		"failures":       backendStatus.Failures,
		"successes":      backendStatus.Successes,
		"last_failure":   backendStatus.LastFailure.Format(time.RFC3339),
		"healthy":        backendStatus.Healthy,
		"in_maintenance": backendStatus.InMaintenance,
		"draining":       backendStatus.Draining,
	}

	data, _ := json.Marshal(status)
	conn.Write([]byte(fmt.Sprintf("CIRCUIT_STATUS_OK|%s\n", string(data))))
}

func (r *RegistryV2) handleCircuitBreakerResetV2(conn net.Conn, sessionID SessionID, parts []string) {
	// CIRCUIT_BREAKER_RESET|session_id|route_id
	if len(parts) < 3 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	routeID := RouteID(parts[2])

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	if cb, found := svc.activeCircuit[routeID]; found {
		cb.State = "closed"
		cb.Failures = 0
		svc.mu.Unlock()
		conn.Write([]byte("CIRCUIT_OK\n"))
	} else {
		svc.mu.Unlock()
		conn.Write([]byte("ERROR|circuit breaker not found\n"))
	}
}

func (r *RegistryV2) handleConfigValidateV2(conn net.Conn, sessionID SessionID) {
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.RLock()
	defer svc.mu.RUnlock()

	// Validate all staged routes
	for routeID, route := range svc.stagedRoutes {
		if err := validateRoute(route.Domains, route.Path, route.BackendURL); err != nil {
			conn.Write([]byte(fmt.Sprintf("ERROR|route %s: %s\n", routeID, err)))
			return
		}
	}

	conn.Write([]byte("OK\n"))
}

func (r *RegistryV2) handleConfigApplyV2(conn net.Conn, sessionID SessionID) {
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()

	// Validate first
	for routeID, route := range svc.stagedRoutes {
		if err := validateRoute(route.Domains, route.Path, route.BackendURL); err != nil {
			svc.mu.Unlock()
			conn.Write([]byte(fmt.Sprintf("ERROR|route %s: %s\n", routeID, err)))
			return
		}
	}

	// Apply routes
	for routeID, route := range svc.stagedRoutes {
		opts := svc.stagedOptions
		if opts == nil {
			opts = make(map[string]interface{})
		}

		// Include health check and rate limit in options
		if hc, found := svc.stagedHealth[routeID]; found {
			opts["health_check_path"] = hc.Path
			opts["health_check_interval"] = hc.Interval.String()
		}
		if rl, found := svc.stagedRateLimit[routeID]; found {
			opts["rate_limit_requests"] = rl.Requests
			opts["rate_limit_window"] = rl.Window.String()
		}

		if err := r.proxyServer.AddRoute(route.Domains, route.Path, route.BackendURL, svc.stagedHeaders, false, opts); err != nil {
			svc.mu.Unlock()
			conn.Write([]byte(fmt.Sprintf("ERROR|failed to add route %s: %s\n", routeID, err)))
			return
		}

		svc.activeRoutes[routeID] = route

		// Register health check if configured
		if hc, found := svc.stagedHealth[routeID]; found && r.healthChecker != nil {
			healthURL := route.BackendURL
			if !strings.HasSuffix(healthURL, "/") && !strings.HasPrefix(hc.Path, "/") {
				healthURL += "/"
			}
			if strings.HasPrefix(hc.Path, "/") {
				healthURL += hc.Path[1:]
			} else {
				healthURL += hc.Path
			}
			r.healthChecker.AddService(string(routeID), healthURL, hc.Interval, hc.Timeout, 200)
			log.Printf("[registry-v2] Health check registered for %s: %s", routeID, healthURL)
		}
	}

	// Apply removals
	for routeID := range svc.stagedRemovals {
		if route, found := svc.activeRoutes[routeID]; found {
			r.proxyServer.RemoveRoute(route.Domains, route.Path)
			delete(svc.activeRoutes, routeID)
			// Remove health check
			if r.healthChecker != nil {
				r.healthChecker.RemoveService(string(routeID))
				log.Printf("[registry-v2] Health check removed for %s", routeID)
			}
		}
	}

	// Apply headers and options
	if len(svc.stagedHeaders) > 0 {
		svc.activeHeaders = make(map[string]string)
		for k, v := range svc.stagedHeaders {
			svc.activeHeaders[k] = v
		}
	}

	if len(svc.stagedOptions) > 0 {
		svc.activeOptions = make(map[string]interface{})
		for k, v := range svc.stagedOptions {
			svc.activeOptions[k] = v
		}
	}

	// Apply health checks, rate limits, circuit breakers
	for k, v := range svc.stagedHealth {
		svc.activeHealth[k] = v
	}
	for k, v := range svc.stagedRateLimit {
		svc.activeRateLimit[k] = v
	}
	for k, v := range svc.stagedCircuit {
		svc.activeCircuit[k] = v
	}

	// Clear staged
	svc.stagedRoutes = make(map[RouteID]*RouteV2)
	svc.stagedHeaders = make(map[string]string)
	svc.stagedOptions = make(map[string]interface{})
	svc.stagedHealth = make(map[RouteID]*HealthCheckV2)
	svc.stagedRateLimit = make(map[RouteID]*RateLimitV2)
	svc.stagedCircuit = make(map[RouteID]*CircuitBreakerV2)
	svc.stagedRemovals = make(map[RouteID]bool)

	svc.mu.Unlock()

	log.Printf("[registry-v2] Config applied for session %s", sessionID)
	conn.Write([]byte("OK\n"))
}

func (r *RegistryV2) handleConfigRollbackV2(conn net.Conn, sessionID SessionID) {
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	svc.stagedRoutes = make(map[RouteID]*RouteV2)
	svc.stagedHeaders = make(map[string]string)
	svc.stagedOptions = make(map[string]interface{})
	svc.stagedHealth = make(map[RouteID]*HealthCheckV2)
	svc.stagedRateLimit = make(map[RouteID]*RateLimitV2)
	svc.stagedCircuit = make(map[RouteID]*CircuitBreakerV2)
	svc.stagedRemovals = make(map[RouteID]bool)
	svc.mu.Unlock()

	conn.Write([]byte("OK\n"))
}

func (r *RegistryV2) handleConfigDiffV2(conn net.Conn, sessionID SessionID) {
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.RLock()
	diff := map[string]interface{}{
		"routes": map[string]interface{}{
			"added":   len(svc.stagedRoutes),
			"removed": len(svc.stagedRemovals),
		},
		"headers": len(svc.stagedHeaders),
		"options": len(svc.stagedOptions),
	}
	svc.mu.RUnlock()

	data, _ := json.Marshal(diff)
	conn.Write([]byte(fmt.Sprintf("DIFF_OK|%s\n", string(data))))
}

func (r *RegistryV2) handleConfigApplyPartialV2(conn net.Conn, sessionID SessionID, parts []string) {
	// CONFIG_APPLY_PARTIAL|session_id|scope
	if len(parts) < 3 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	scope := parts[2]

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()

	scopes := strings.Split(scope, ",")
	for _, s := range scopes {
		s = strings.TrimSpace(s)
		switch s {
		case "routes":
			for routeID, route := range svc.stagedRoutes {
				r.proxyServer.AddRoute(route.Domains, route.Path, route.BackendURL, nil, false, nil)
				svc.activeRoutes[routeID] = route
			}
			svc.stagedRoutes = make(map[RouteID]*RouteV2)
		case "headers":
			for k, v := range svc.stagedHeaders {
				svc.activeHeaders[k] = v
			}
			svc.stagedHeaders = make(map[string]string)
		case "options":
			for k, v := range svc.stagedOptions {
				svc.activeOptions[k] = v
			}
			svc.stagedOptions = make(map[string]interface{})
		case "health":
			for k, v := range svc.stagedHealth {
				svc.activeHealth[k] = v
			}
			svc.stagedHealth = make(map[RouteID]*HealthCheckV2)
		case "ratelimit":
			for k, v := range svc.stagedRateLimit {
				svc.activeRateLimit[k] = v
			}
			svc.stagedRateLimit = make(map[RouteID]*RateLimitV2)
		}
	}

	svc.mu.Unlock()
	conn.Write([]byte("OK\n"))
}

func (r *RegistryV2) handleStatsGetV2(conn net.Conn, sessionID SessionID, parts []string) {
	_ = parts
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	var stats []interface{}

	r.mu.RLock()
	for routeID, stat := range r.stats {
		svc.mu.RLock()
		_, found := svc.activeRoutes[routeID]
		svc.mu.RUnlock()

		if found {
			stats = append(stats, stat)
		}
	}
	r.mu.RUnlock()

	if stats == nil {
		stats = make([]interface{}, 0)
	}

	data, _ := json.Marshal(stats)
	conn.Write([]byte(fmt.Sprintf("STATS_OK|%s\n", string(data))))
}

func (r *RegistryV2) handleBackendTestV2(conn net.Conn, sessionID SessionID, parts []string) {
	_ = sessionID
	// BACKEND_TEST|session_id|backend_url
	if len(parts) < 3 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	backendURL := parts[2]

	// Make HTTP GET request to backend
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", backendURL+"/", nil)
	timeout := r.upstreamTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)

	if err != nil {
		result := map[string]interface{}{
			"reachable": false,
			"error":     err.Error(),
		}
		data, _ := json.Marshal(result)
		conn.Write([]byte(fmt.Sprintf("BACKEND_FAIL|%s\n", string(data))))
		return
	}
	defer resp.Body.Close()

	result := map[string]interface{}{
		"reachable":   true,
		"status_code": resp.StatusCode,
		"tls_valid":   resp.TLS != nil,
	}
	data, _ := json.Marshal(result)
	conn.Write([]byte(fmt.Sprintf("BACKEND_OK|%s\n", string(data))))
}

func (r *RegistryV2) handleDrainStartV2(conn net.Conn, sessionID SessionID, parts []string) {
	// DRAIN_START|session_id|duration
	if len(parts) < 3 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	duration := parseDuration(parts[2])

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	svc.draining = true
	svc.drainStart = time.Now()
	svc.drainDuration = duration

	// Start drain in proxy for all active routes
	for _, route := range svc.activeRoutes {
		if err := r.proxyServer.StartDrain(route.Domains, route.Path, duration); err != nil {
			log.Printf("[registry-v2] Warning: failed to start drain for route: %s", err)
		}
	}
	svc.mu.Unlock()

	completionTime := time.Now().Add(duration)
	conn.Write([]byte(fmt.Sprintf("DRAIN_OK|%s\n", completionTime.Format(time.RFC3339))))
}

func (r *RegistryV2) handleDrainStatusV2(conn net.Conn, sessionID SessionID) {
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.RLock()
	if !svc.draining {
		svc.mu.RUnlock()
		conn.Write([]byte("ERROR|no drain in progress\n"))
		return
	}

	elapsed := time.Since(svc.drainStart)
	remaining := svc.drainDuration - elapsed
	if remaining < 0 {
		remaining = 0
	}

	trafficPercent := 100 - int((elapsed.Seconds()/svc.drainDuration.Seconds())*100)
	if trafficPercent < 0 {
		trafficPercent = 0
	}

	status := map[string]interface{}{
		"active":            svc.draining,
		"started_at":        svc.drainStart.Format(time.RFC3339),
		"duration_seconds":  int(svc.drainDuration.Seconds()),
		"elapsed_seconds":   int(elapsed.Seconds()),
		"remaining_seconds": int(remaining.Seconds()),
		"traffic_percent":   trafficPercent,
	}
	svc.mu.RUnlock()

	data, _ := json.Marshal(status)
	conn.Write([]byte(fmt.Sprintf("DRAIN_STATUS_OK|%s\n", string(data))))
}

func (r *RegistryV2) handleDrainCancelV2(conn net.Conn, sessionID SessionID) {
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	if !svc.draining {
		svc.mu.Unlock()
		conn.Write([]byte("ERROR|no drain in progress\n"))
		return
	}

	svc.draining = false

	// Cancel drain in proxy for all active routes
	for _, route := range svc.activeRoutes {
		if err := r.proxyServer.CancelDrain(route.Domains, route.Path); err != nil {
			log.Printf("[registry-v2] Warning: failed to cancel drain for route: %s", err)
		}
	}
	svc.mu.Unlock()

	conn.Write([]byte("DRAIN_OK\n"))
}

func (r *RegistryV2) handleSubscribeV2(conn net.Conn, sessionID SessionID, parts []string) {
	// SUBSCRIBE|session_id|event_type
	if len(parts) < 3 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	eventType := parts[2]

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	svc.subscriptions[eventType] = true
	svc.mu.Unlock()

	conn.Write([]byte("SUBSCRIBE_OK\n"))
}

func (r *RegistryV2) handleUnsubscribeV2(conn net.Conn, sessionID SessionID, parts []string) {
	// UNSUBSCRIBE|session_id|event_type
	if len(parts) < 3 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	eventType := parts[2]

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	delete(svc.subscriptions, eventType)
	svc.mu.Unlock()

	conn.Write([]byte("UNSUBSCRIBE_OK\n"))
}

// maintenanceVerificationWorker processes maintenance verification tasks
func (r *RegistryV2) maintenanceVerificationWorker() {
	for task := range r.maintTasks {
		r.processMaintenanceTask(task)
	}
}

// processMaintenanceTask verifies URL reachability and sends MAINT_OK or ERROR
func (r *RegistryV2) processMaintenanceTask(task *maintenanceTask) {
	maxRetries := 60 // Try for up to 60 seconds
	retryDelay := 1 * time.Second

	log.Printf("[registry-v2] Starting maintenance verification for %s: %s", task.sessionID, task.url)

	for i := 0; i < maxRetries; i++ {
		select {
		case <-task.ctx.Done():
			log.Printf("[registry-v2] Maintenance verification cancelled for %s", task.sessionID)
			return
		default:
		}

		if err := r.verifyMaintenanceURL(task.url); err == nil {
			// URL is reachable!
			log.Printf("[registry-v2] %s verified: %s",
				map[bool]string{true: "Maintenance URL", false: "Backend"}[task.isEnter],
				task.url)

			// Send success
			task.conn.Write([]byte(fmt.Sprintf("MAINT_OK|%s\n", task.target)))

			// Clean up cancel function
			r.maintCancelMu.Lock()
			delete(r.maintCancel, task.sessionID)
			r.maintCancelMu.Unlock()
			return
		}

		// Not ready yet, wait and retry
		time.Sleep(retryDelay)
	}

	// Failed to verify after all retries
	log.Printf("[registry-v2] Maintenance verification timeout for %s: %s", task.sessionID, task.url)
	task.conn.Write([]byte(fmt.Sprintf("ERROR|timeout waiting for %s to become reachable\n",
		map[bool]string{true: "maintenance page", false: "backend"}[task.isEnter])))

	// Clean up
	r.maintCancelMu.Lock()
	delete(r.maintCancel, task.sessionID)
	r.maintCancelMu.Unlock()
}

// verifyMaintenanceURL checks if the maintenance URL is reachable
func (r *RegistryV2) verifyMaintenanceURL(maintenanceURL string) error {
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 3 * time.Second,
			}).DialContext,
		},
	}

	resp, err := client.Get(maintenanceURL)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return fmt.Errorf("server error: %d", resp.StatusCode)
	}

	return nil
}

func (r *RegistryV2) handleMaintenanceEnterV2(conn net.Conn, sessionID SessionID, parts []string) {
	// MAINT_ENTER|session_id|target|maintenance_page_url
	if len(parts) < 4 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	target := parts[2]
	maintenancePageURL := parts[3] // Custom maintenance page URL (can be empty for default)

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	// Set maintenance mode immediately
	svc.mu.Lock()
	if target == "ALL" {
		// All routes in maintenance
		for routeID, route := range svc.activeRoutes {
			svc.maintenanceRoutes[routeID] = true
			// Set maintenance in proxy with custom page URL
			if err := r.proxyServer.SetMaintenance(route.Domains, route.Path, true, maintenancePageURL); err != nil {
				log.Printf("[registry-v2] Warning: failed to set maintenance for %s: %s", routeID, err)
			}
		}
	} else {
		// Specific routes
		targets := strings.Split(target, ",")
		for _, t := range targets {
			routeID := RouteID(strings.TrimSpace(t))
			svc.maintenanceRoutes[routeID] = true
			if route, found := svc.activeRoutes[routeID]; found {
				if err := r.proxyServer.SetMaintenance(route.Domains, route.Path, true, maintenancePageURL); err != nil {
					log.Printf("[registry-v2] Warning: failed to set maintenance for %s: %s", routeID, err)
				}
			}
		}
	}
	svc.mu.Unlock()

	// Send immediate ACK
	conn.Write([]byte("ACK\n"))

	// If maintenance URL is provided, verify it asynchronously
	if maintenancePageURL != "" {
		// Cancel any existing verification task for this session
		r.maintCancelMu.Lock()
		if cancel, exists := r.maintCancel[sessionID]; exists {
			cancel()
		}
		ctx, cancel := context.WithCancel(context.Background())
		r.maintCancel[sessionID] = cancel
		r.maintCancelMu.Unlock()

		// Submit verification task to worker pool
		task := &maintenanceTask{
			sessionID: sessionID,
			target:    target,
			url:       maintenancePageURL,
			isEnter:   true,
			conn:      conn,
			ctx:       ctx,
			cancel:    cancel,
		}
		r.maintTasks <- task
	} else {
		// No URL to verify, send MAINT_OK immediately
		conn.Write([]byte(fmt.Sprintf("MAINT_OK|%s\n", target)))
	}
}

func (r *RegistryV2) handleMaintenanceExitV2(conn net.Conn, sessionID SessionID, parts []string) {
	// MAINT_EXIT|session_id|target
	if len(parts) < 3 {
		conn.Write([]byte("ERROR|invalid format\n"))
		return
	}

	target := parts[2]

	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	// Cancel any pending maintenance verification for this session
	r.maintCancelMu.Lock()
	if cancel, exists := r.maintCancel[sessionID]; exists {
		cancel()
		delete(r.maintCancel, sessionID)
	}
	r.maintCancelMu.Unlock()

	// Get backend URL to verify
	svc.mu.RLock()
	var backendURLToCheck string
	if target == "ALL" {
		// Check the first route's backend
		for _, route := range svc.activeRoutes {
			backendURLToCheck = route.BackendURL
			break
		}
	} else {
		// Check specific route
		routeID := RouteID(strings.TrimSpace(target))
		if route, found := svc.activeRoutes[routeID]; found {
			backendURLToCheck = route.BackendURL
		}
	}
	svc.mu.RUnlock()

	// Exit maintenance mode immediately (but don't send MAINT_OK yet)
	svc.mu.Lock()
	if target == "ALL" {
		// Exit all from maintenance
		for routeID, route := range svc.activeRoutes {
			if svc.maintenanceRoutes[routeID] {
				if err := r.proxyServer.SetMaintenance(route.Domains, route.Path, false, ""); err != nil {
					log.Printf("[registry-v2] Warning: failed to exit maintenance for %s: %s", routeID, err)
				}
			}
		}
		svc.maintenanceRoutes = make(map[RouteID]bool)
	} else {
		targets := strings.Split(target, ",")
		for _, t := range targets {
			routeID := RouteID(strings.TrimSpace(t))
			if route, found := svc.activeRoutes[routeID]; found {
				if err := r.proxyServer.SetMaintenance(route.Domains, route.Path, false, ""); err != nil {
					log.Printf("[registry-v2] Warning: failed to exit maintenance for %s: %s", routeID, err)
				}
			}
			delete(svc.maintenanceRoutes, routeID)
		}
	}
	svc.mu.Unlock()

	// Send immediate ACK
	conn.Write([]byte("ACK\n"))

	// Verify backend is healthy asynchronously before sending MAINT_OK
	if backendURLToCheck != "" {
		ctx, cancel := context.WithCancel(context.Background())
		r.maintCancelMu.Lock()
		r.maintCancel[sessionID] = cancel
		r.maintCancelMu.Unlock()

		task := &maintenanceTask{
			sessionID: sessionID,
			target:    target,
			url:       backendURLToCheck,
			isEnter:   false,
			conn:      conn,
			ctx:       ctx,
			cancel:    cancel,
		}
		r.maintTasks <- task
	} else {
		// No backend to verify, send MAINT_OK immediately
		conn.Write([]byte(fmt.Sprintf("MAINT_OK|%s\n", target)))
	}
}

func (r *RegistryV2) handleMaintenanceStatusV2(conn net.Conn, sessionID SessionID) {
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.RLock()
	inMaint := make([]string, 0)
	for routeID := range svc.maintenanceRoutes {
		inMaint = append(inMaint, string(routeID))
	}

	status := map[string]interface{}{
		"in_maintenance": inMaint,
	}
	svc.mu.RUnlock()

	data, _ := json.Marshal(status)
	conn.Write([]byte(fmt.Sprintf("MAINT_STATUS_OK|%s\n", string(data))))
}

func (r *RegistryV2) handleClientShutdownV2(conn net.Conn, sessionID SessionID) {
	r.mu.RLock()
	svc, exists := r.services[sessionID]
	r.mu.RUnlock()

	if !exists {
		conn.Write([]byte("ERROR|session not found\n"))
		return
	}

	svc.mu.Lock()
	// Remove all active routes
	for routeID, route := range svc.activeRoutes {
		r.proxyServer.RemoveRoute(route.Domains, route.Path)
		delete(svc.activeRoutes, routeID)
	}
	svc.mu.Unlock()

	r.mu.Lock()
	delete(r.services, sessionID)
	r.mu.Unlock()

	log.Printf("[registry-v2] Client shutdown: %s", sessionID)
	conn.Write([]byte("SHUTDOWN_OK\n"))
	conn.Close()
}

// Helper functions

func (r *RegistryV2) generateRouteID() RouteID {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.nextRouteID
	r.nextRouteID++
	return RouteID(fmt.Sprintf("r%d", id))
}

func (r *RegistryV2) cleanupExpiredStagedConfigs(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.RLock()
			for _, svc := range r.services {
				svc.mu.Lock()
				if time.Now().After(svc.stagedTimeout) && len(svc.stagedRoutes) > 0 {
					log.Printf("[registry-v2] Expiring staged config for session %s", svc.SessionID)
					svc.stagedRoutes = make(map[RouteID]*RouteV2)
					svc.stagedHeaders = make(map[string]string)
					svc.stagedOptions = make(map[string]interface{})
				}
				svc.mu.Unlock()
			}
			r.mu.RUnlock()
		}
	}
}

func (r *RegistryV2) cleanupDisconnectedSessions(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second) // Check frequently for expired sessions
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			r.mu.RLock()
			var expiredSessions []SessionID
			for sid, svc := range r.services {
				svc.mu.RLock()
				if svc.DisconnectedAt != nil && now.Sub(*svc.DisconnectedAt) > r.reconnectTimeout {
					expiredSessions = append(expiredSessions, sid)
				}
				svc.mu.RUnlock()
			}
			r.mu.RUnlock()

			// Cleanup expired sessions
			for _, sid := range expiredSessions {
				r.mu.RLock()
				svc, exists := r.services[sid]
				r.mu.RUnlock()

				if exists {
					log.Printf("[registry-v2] Grace period expired for session %s (%s) - deleting session and removing routes", sid, svc.ServiceName)

					svc.mu.Lock()
					// Remove all routes from proxy (they were disabled, now fully remove them)
					for routeID, route := range svc.activeRoutes {
						r.proxyServer.RemoveRoute(route.Domains, route.Path)
						log.Printf("[registry-v2] Removed route %s: %v%s (grace period expired)", routeID, route.Domains, route.Path)
					}

					// Remove health checks
					for routeID := range svc.activeHealth {
						r.healthChecker.RemoveService(string(routeID))
					}
					svc.mu.Unlock()

					// Remove service from registry
					r.mu.Lock()
					delete(r.services, sid)
					r.mu.Unlock()
				}
			}
		}
	}
}

func validateRoute(domains []string, path string, backendURL string) error {
	if len(domains) == 0 {
		return fmt.Errorf("no domains specified")
	}
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	if !strings.Contains(backendURL, "://") {
		return fmt.Errorf("invalid backend url format")
	}
	return nil
}

func parseDuration(s string) time.Duration {
	d, _ := time.ParseDuration(s)
	return d
}

func parseStringArray(v interface{}) []string {
	switch val := v.(type) {
	case []interface{}:
		result := make([]string, len(val))
		for i, item := range val {
			result[i] = fmt.Sprintf("%v", item)
		}
		return result
	case []string:
		return val
	default:
		return make([]string, 0)
	}
}

func getFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	default:
		return 0
	}
}

func rand64() int64 {
	return time.Now().UnixNano() % 1000000
}
