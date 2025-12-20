package registry

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chilla55/proxy-manager/proxy"
)

// mockProxy implements ProxyServer for testing
type mockProxy struct {
	addCalls []struct {
		domains   []string
		path      string
		backend   string
		headers   map[string]string
		websocket bool
		options   map[string]interface{}
	}
	removeCalls []struct {
		domains []string
		path    string
	}
	maintenanceCalls []struct {
		domains []string
		path    string
		enabled bool
	}
	drainCalls []struct {
		domains  []string
		path     string
		duration time.Duration
	}
	backendStatus *proxy.BackendStatus
}

func (m *mockProxy) AddRoute(domains []string, path, backendURL string, headers map[string]string, websocket bool, options map[string]interface{}) error {
	m.addCalls = append(m.addCalls, struct {
		domains   []string
		path      string
		backend   string
		headers   map[string]string
		websocket bool
		options   map[string]interface{}
	}{
		domains:   domains,
		path:      path,
		backend:   backendURL,
		headers:   headers,
		websocket: websocket,
		options:   options,
	})
	return nil
}

func (m *mockProxy) RemoveRoute(domains []string, path string) {
	m.removeCalls = append(m.removeCalls, struct {
		domains []string
		path    string
	}{domains: domains, path: path})
}

func (m *mockProxy) GetBackendStatus(domain, path string) *proxy.BackendStatus {
	if m.backendStatus != nil {
		return m.backendStatus
	}
	return &proxy.BackendStatus{
		Healthy:      true,
		CircuitState: "closed",
		Failures:     0,
	}
}

func (m *mockProxy) SetMaintenance(domains []string, path string, enabled bool, maintenancePageURL string) error {
	m.maintenanceCalls = append(m.maintenanceCalls, struct {
		domains []string
		path    string
		enabled bool
	}{domains: domains, path: path, enabled: enabled})
	return nil
}

func (m *mockProxy) StartDrain(domains []string, path string, duration time.Duration) error {
	m.drainCalls = append(m.drainCalls, struct {
		domains  []string
		path     string
		duration time.Duration
	}{domains: domains, path: path, duration: duration})
	return nil
}

func (m *mockProxy) CancelDrain(domains []string, path string) error {
	m.drainCalls = append(m.drainCalls, struct {
		domains  []string
		path     string
		duration time.Duration
	}{domains: domains, path: path, duration: 0}) // duration=0 indicates cancel
	return nil
}

// mockHealthChecker implements HealthChecker for testing
type mockHealthChecker struct {
	addCalls []struct {
		name           string
		url            string
		interval       time.Duration
		timeout        time.Duration
		expectedStatus int
	}
	removeCalls []string
}

func (m *mockHealthChecker) AddService(name, url string, interval, timeout time.Duration, expectedStatus int) {
	m.addCalls = append(m.addCalls, struct {
		name           string
		url            string
		interval       time.Duration
		timeout        time.Duration
		expectedStatus int
	}{name: name, url: url, interval: interval, timeout: timeout, expectedStatus: expectedStatus})
}

func (m *mockHealthChecker) RemoveService(name string) {
	m.removeCalls = append(m.removeCalls, name)
}

// helper to send a command and read one line response
func send(conn net.Conn, s string) (string, error) {
	_, err := conn.Write([]byte(s + "\n"))
	if err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return "", scanner.Err()
	}
	return scanner.Text(), nil
}

// recv reads a single line response without sending
func recv(conn net.Conn) (string, error) {
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return "", scanner.Err()
	}
	return scanner.Text(), nil
}

func TestRegistryV2_RegisterRouteApplyList(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Start handler
	go reg.handleConnectionV2(ctx, server)

	// REGISTER
	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if !strings.HasPrefix(resp, "ACK|") {
		t.Fatalf("expected ACK, got %q", resp)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")
	if sessionID == "" {
		t.Fatalf("empty session id")
	}

	// ROUTE_ADD
	resp, err = send(client, "ROUTE_ADD|"+sessionID+"|example.com|/api|http://localhost:8080|10")
	if err != nil {
		t.Fatalf("route add error: %v", err)
	}
	if !strings.HasPrefix(resp, "ROUTE_OK|") {
		t.Fatalf("expected ROUTE_OK, got %q", resp)
	}

	// CONFIG_VALIDATE
	resp, err = send(client, "CONFIG_VALIDATE|"+sessionID)
	if err != nil {
		t.Fatalf("validate error: %v", err)
	}
	if resp != "OK" {
		t.Fatalf("expected OK, got %q", resp)
	}

	// CONFIG_APPLY
	resp, err = send(client, "CONFIG_APPLY|"+sessionID)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if resp != "OK" {
		t.Fatalf("expected OK, got %q", resp)
	}

	// Ensure proxy received AddRoute
	if len(mp.addCalls) != 1 {
		t.Fatalf("expected 1 AddRoute call, got %d", len(mp.addCalls))
	}
	call := mp.addCalls[0]
	if call.path != "/api" || call.backend != "http://localhost:8080" {
		t.Fatalf("unexpected route applied: %+v", call)
	}

	// ROUTE_LIST
	resp, err = send(client, "ROUTE_LIST|"+sessionID)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if !strings.HasPrefix(resp, "ROUTE_LIST_OK|") {
		t.Fatalf("expected ROUTE_LIST_OK, got %q", resp)
	}
}

func TestRegistryV2_PingReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// PING
	resp, err = send(client, "PING|"+sessionID)
	if err != nil {
		t.Fatalf("ping error: %v", err)
	}
	if resp != "PONG" {
		t.Fatalf("expected PONG, got %q", resp)
	}

	// RECONNECT
	resp, err = send(client, "RECONNECT|"+sessionID)
	if err != nil {
		t.Fatalf("reconnect error: %v", err)
	}
	if resp != "OK" {
		t.Fatalf("expected OK, got %q", resp)
	}
}

func TestRegistryV2_RouteRemoveApply(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	// REGISTER
	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// Add and apply
	resp, err = send(client, "ROUTE_ADD|"+sessionID+"|example.com|/r|http://localhost:8083|5")
	if err != nil {
		t.Fatalf("route add error: %v", err)
	}
	if !strings.HasPrefix(resp, "ROUTE_OK|") {
		t.Fatalf("expected ROUTE_OK|, got %q", resp)
	}
	routeID := strings.TrimPrefix(resp, "ROUTE_OK|")

	resp, err = send(client, "CONFIG_APPLY|"+sessionID)
	if err != nil || resp != "OK" {
		t.Fatalf("apply err=%v resp=%q", err, resp)
	}

	// Stage removal and apply
	resp, err = send(client, "ROUTE_REMOVE|"+sessionID+"|"+routeID)
	if err != nil || resp != "ROUTE_OK" {
		t.Fatalf("remove err=%v resp=%q", err, resp)
	}

	resp, err = send(client, "CONFIG_APPLY|"+sessionID)
	if err != nil || resp != "OK" {
		t.Fatalf("apply err=%v resp=%q", err, resp)
	}

	if len(mp.removeCalls) != 1 {
		t.Fatalf("expected 1 RemoveRoute call, got %d", len(mp.removeCalls))
	}
}

func TestRegistryV2_RouteRemoveRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// Add and apply
	resp, err = send(client, "ROUTE_ADD|"+sessionID+"|example.com|/r|http://localhost:8083|5")
	if err != nil {
		t.Fatalf("route add error: %v", err)
	}
	if !strings.HasPrefix(resp, "ROUTE_OK|") {
		t.Fatalf("expected ROUTE_OK|, got %q", resp)
	}
	routeID := strings.TrimPrefix(resp, "ROUTE_OK|")
	resp, err = send(client, "CONFIG_APPLY|"+sessionID)
	if err != nil || resp != "OK" {
		t.Fatalf("apply err=%v resp=%q", err, resp)
	}

	// Stage removal then rollback
	resp, err = send(client, "ROUTE_REMOVE|"+sessionID+"|"+routeID)
	if err != nil || resp != "ROUTE_OK" {
		t.Fatalf("remove err=%v resp=%q", err, resp)
	}
	resp, err = send(client, "CONFIG_ROLLBACK|"+sessionID)
	if err != nil || resp != "OK" {
		t.Fatalf("rollback err=%v resp=%q", err, resp)
	}

	// Apply should not remove
	resp, err = send(client, "CONFIG_APPLY|"+sessionID)
	if err != nil || resp != "OK" {
		t.Fatalf("apply err=%v resp=%q", err, resp)
	}
	if len(mp.removeCalls) != 0 {
		t.Fatalf("expected 0 RemoveRoute calls, got %d", len(mp.removeCalls))
	}
}

func TestRegistryV2_RouteBulkApply(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	payload := `[{"domains":["example.com"],"path":"/a","backend_url":"http://localhost:8081","priority":10},{"domains":["example.com"],"path":"/b","backend_url":"http://localhost:8082","priority":10}]`
	resp, err = send(client, "ROUTE_ADD_BULK|"+sessionID+"|"+payload)
	if err != nil {
		t.Fatalf("bulk add error: %v", err)
	}
	if !strings.HasPrefix(resp, "ROUTE_BULK_OK|") {
		t.Fatalf("expected ROUTE_BULK_OK|, got %q", resp)
	}

	resp, err = send(client, "CONFIG_VALIDATE|"+sessionID)
	if err != nil || resp != "OK" {
		t.Fatalf("validate err=%v resp=%q", err, resp)
	}
	resp, err = send(client, "CONFIG_APPLY|"+sessionID)
	if err != nil || resp != "OK" {
		t.Fatalf("apply err=%v resp=%q", err, resp)
	}

	if len(mp.addCalls) != 2 {
		t.Fatalf("expected 2 AddRoute calls, got %d", len(mp.addCalls))
	}
}

func TestRegistryV2_MaintenanceFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// Enter ALL
	resp, err = send(client, "MAINT_ENTER|"+sessionID+"|ALL|http://localhost:9999")
	if err != nil {
		t.Fatalf("maint enter error: %v", err)
	}
	if resp != "ACK" {
		t.Fatalf("expected ACK, got %q", resp)
	}
	// Maintenance event line
	evt, err := recv(client)
	if err != nil {
		t.Fatalf("maint event recv error: %v", err)
	}
	if !strings.HasPrefix(evt, "MAINT_OK|") {
		t.Fatalf("expected MAINT_OK|, got %q", evt)
	}

	// Status
	resp, err = send(client, "MAINT_STATUS|"+sessionID)
	if err != nil {
		t.Fatalf("maint status error: %v", err)
	}
	if !strings.HasPrefix(resp, "MAINT_STATUS_OK|") {
		t.Fatalf("expected MAINT_STATUS_OK|, got %q", resp)
	}

	// Exit ALL
	resp, err = send(client, "MAINT_EXIT|"+sessionID+"|ALL")
	if err != nil {
		t.Fatalf("maint exit error: %v", err)
	}
	if resp != "ACK" {
		t.Fatalf("expected ACK, got %q", resp)
	}
	evt, err = recv(client)
	if err != nil {
		t.Fatalf("maint event recv error: %v", err)
	}
	if !strings.HasPrefix(evt, "MAINT_OK|") {
		t.Fatalf("expected MAINT_OK|, got %q", evt)
	}
}

func TestRegistryV2_DrainFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	resp, err = send(client, "DRAIN_START|"+sessionID+"|200ms")
	if err != nil {
		t.Fatalf("drain start error: %v", err)
	}
	if !strings.HasPrefix(resp, "DRAIN_OK|") {
		t.Fatalf("expected DRAIN_OK|, got %q", resp)
	}

	resp, err = send(client, "DRAIN_STATUS|"+sessionID)
	if err != nil {
		t.Fatalf("drain status error: %v", err)
	}
	if !strings.HasPrefix(resp, "DRAIN_STATUS_OK|") {
		t.Fatalf("expected DRAIN_STATUS_OK|, got %q", resp)
	}

	resp, err = send(client, "DRAIN_CANCEL|"+sessionID)
	if err != nil {
		t.Fatalf("drain cancel error: %v", err)
	}
	if resp != "DRAIN_OK" {
		t.Fatalf("expected DRAIN_OK, got %q", resp)
	}
}

func TestRegistryV2_CircuitBreakerFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// Create a route to target
	resp, err = send(client, "ROUTE_ADD|"+sessionID+"|example.com|/cb|http://localhost:8084|5")
	if err != nil {
		t.Fatalf("route add error: %v", err)
	}
	if !strings.HasPrefix(resp, "ROUTE_OK|") {
		t.Fatalf("expected ROUTE_OK|, got %q", resp)
	}
	routeID := strings.TrimPrefix(resp, "ROUTE_OK|")

	// Stage circuit and apply
	resp, err = send(client, "CIRCUIT_BREAKER_SET|"+sessionID+"|"+routeID+"|5|500ms|2")
	if err != nil {
		t.Fatalf("circuit set error: %v", err)
	}
	if resp != "CIRCUIT_OK" {
		t.Fatalf("expected CIRCUIT_OK, got %q", resp)
	}
	resp, err = send(client, "CONFIG_APPLY|"+sessionID)
	if err != nil || resp != "OK" {
		t.Fatalf("apply err=%v resp=%q", err, resp)
	}

	// Status
	resp, err = send(client, "CIRCUIT_BREAKER_STATUS|"+sessionID+"|"+routeID)
	if err != nil {
		t.Fatalf("circuit status error: %v", err)
	}
	if !strings.HasPrefix(resp, "CIRCUIT_STATUS_OK|") {
		t.Fatalf("expected CIRCUIT_STATUS_OK|, got %q", resp)
	}

	// Reset
	resp, err = send(client, "CIRCUIT_BREAKER_RESET|"+sessionID+"|"+routeID)
	if err != nil {
		t.Fatalf("circuit reset error: %v", err)
	}
	if resp != "CIRCUIT_OK" {
		t.Fatalf("expected CIRCUIT_OK, got %q", resp)
	}
}

func TestRegistryV2_BackendTestOK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// Start a test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	resp, err = send(client, "BACKEND_TEST|"+sessionID+"|"+ts.URL)
	if err != nil {
		t.Fatalf("backend test error: %v", err)
	}
	if !strings.HasPrefix(resp, "BACKEND_OK|") {
		t.Fatalf("expected BACKEND_OK|, got %q", resp)
	}
}

func TestRegistryV2_SessionInfoAndShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	resp, err = send(client, "SESSION_INFO|"+sessionID)
	if err != nil {
		t.Fatalf("session info error: %v", err)
	}
	if !strings.HasPrefix(resp, "SESSION_OK|") {
		t.Fatalf("expected SESSION_OK|, got %q", resp)
	}

	resp, err = send(client, "CLIENT_SHUTDOWN|"+sessionID)
	if err != nil {
		t.Fatalf("client shutdown error: %v", err)
	}
	if resp != "SHUTDOWN_OK" {
		t.Fatalf("expected SHUTDOWN_OK, got %q", resp)
	}
}

func TestRegistryV2_NegativeInvalidFormat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	// REGISTER missing parts
	resp, err := send(client, "REGISTER|svc")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	if !strings.HasPrefix(resp, "ERROR|") {
		t.Fatalf("expected ERROR|, got %q", resp)
	}

	// Valid REGISTER
	resp, err = send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// ROUTE_ADD invalid format
	resp, err = send(client, "ROUTE_ADD|"+sessionID+"|example.com")
	if err != nil {
		t.Fatalf("route add error: %v", err)
	}
	if !strings.HasPrefix(resp, "ERROR|") {
		t.Fatalf("expected ERROR|, got %q", resp)
	}

	// ROUTE_ADD invalid backend URL
	resp, err = send(client, "ROUTE_ADD|"+sessionID+"|example.com|/api|notaurl|5")
	if err != nil {
		t.Fatalf("route add error: %v", err)
	}
	if !strings.HasPrefix(resp, "ERROR|") {
		t.Fatalf("expected ERROR|, got %q", resp)
	}
}

func TestRegistryV2_NegativeUnknownSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	// ROUTE_ADD with unknown session
	resp, err := send(client, "ROUTE_ADD|fakesession|example.com|/api|http://localhost:8080|5")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if !strings.HasPrefix(resp, "ERROR|") {
		t.Fatalf("expected ERROR|, got %q", resp)
	}
}

func TestRegistryV2_HeadersFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// Set header
	resp, err = send(client, "HEADERS_SET|"+sessionID+"|ALL|X-Custom|value1")
	if err != nil {
		t.Fatalf("headers set error: %v", err)
	}
	if resp != "HEADERS_OK" {
		t.Fatalf("expected HEADERS_OK, got %q", resp)
	}

	// Remove header
	resp, err = send(client, "HEADERS_REMOVE|"+sessionID+"|ALL|X-Custom")
	if err != nil {
		t.Fatalf("headers remove error: %v", err)
	}
	if resp != "HEADERS_OK" {
		t.Fatalf("expected HEADERS_OK, got %q", resp)
	}
}

func TestRegistryV2_OptionsFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// Set option
	resp, err = send(client, "OPTIONS_SET|"+sessionID+"|ALL|timeout|30")
	if err != nil {
		t.Fatalf("options set error: %v", err)
	}
	if resp != "OPTIONS_OK" {
		t.Fatalf("expected OPTIONS_OK, got %q", resp)
	}

	// Remove option
	resp, err = send(client, "OPTIONS_REMOVE|"+sessionID+"|ALL|timeout")
	if err != nil {
		t.Fatalf("options remove error: %v", err)
	}
	if resp != "OPTIONS_OK" {
		t.Fatalf("expected OPTIONS_OK, got %q", resp)
	}
}

func TestRegistryV2_HealthSetFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// Add route
	resp, err = send(client, "ROUTE_ADD|"+sessionID+"|example.com|/health|http://localhost:8085|5")
	if err != nil {
		t.Fatalf("route add error: %v", err)
	}
	if !strings.HasPrefix(resp, "ROUTE_OK|") {
		t.Fatalf("expected ROUTE_OK|, got %q", resp)
	}
	routeID := strings.TrimPrefix(resp, "ROUTE_OK|")

	// Set health
	resp, err = send(client, "HEALTH_SET|"+sessionID+"|"+routeID+"|/health|10s|2s")
	if err != nil {
		t.Fatalf("health set error: %v", err)
	}
	if resp != "HEALTH_OK" {
		t.Fatalf("expected HEALTH_OK, got %q", resp)
	}

	resp, err = send(client, "CONFIG_APPLY|"+sessionID)
	if err != nil || resp != "OK" {
		t.Fatalf("apply err=%v resp=%q", err, resp)
	}
}

func TestRegistryV2_RateLimitSetFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// Add route
	resp, err = send(client, "ROUTE_ADD|"+sessionID+"|example.com|/rl|http://localhost:8086|5")
	if err != nil {
		t.Fatalf("route add error: %v", err)
	}
	if !strings.HasPrefix(resp, "ROUTE_OK|") {
		t.Fatalf("expected ROUTE_OK|, got %q", resp)
	}
	routeID := strings.TrimPrefix(resp, "ROUTE_OK|")

	// Set rate limit
	resp, err = send(client, "RATELIMIT_SET|"+sessionID+"|"+routeID+"|100|60s")
	if err != nil {
		t.Fatalf("ratelimit set error: %v", err)
	}
	if resp != "RATELIMIT_OK" {
		t.Fatalf("expected RATELIMIT_OK, got %q", resp)
	}

	resp, err = send(client, "CONFIG_APPLY|"+sessionID)
	if err != nil || resp != "OK" {
		t.Fatalf("apply err=%v resp=%q", err, resp)
	}
}

func TestRegistryV2_ConfigDiffFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	mp := &mockProxy{}
	reg := NewRegistryV2(0, mp, false, 100*time.Millisecond, &mockHealthChecker{})

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go reg.handleConnectionV2(ctx, server)

	resp, err := send(client, "REGISTER|svc|inst1|9000|{}")
	if err != nil {
		t.Fatalf("register error: %v", err)
	}
	sessionID := strings.TrimPrefix(resp, "ACK|")

	// Add route
	resp, err = send(client, "ROUTE_ADD|"+sessionID+"|example.com|/diff|http://localhost:8087|5")
	if err != nil {
		t.Fatalf("route add error: %v", err)
	}

	// Add headers
	resp, err = send(client, "HEADERS_SET|"+sessionID+"|ALL|X-Test|val")
	if err != nil {
		t.Fatalf("headers set error: %v", err)
	}

	// Get diff
	resp, err = send(client, "CONFIG_DIFF|"+sessionID)
	if err != nil {
		t.Fatalf("config diff error: %v", err)
	}
	if !strings.HasPrefix(resp, "DIFF_OK|") {
		t.Fatalf("expected DIFF_OK|, got %q", resp)
	}
	if !strings.Contains(resp, "routes") {
		t.Fatalf("expected diff to contain routes, got %q", resp)
	}
}
