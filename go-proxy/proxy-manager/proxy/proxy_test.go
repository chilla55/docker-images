package proxy

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// dummyConn implements net.Conn for Hijack
type dummyConn struct{ closed bool }

func (d *dummyConn) Read(b []byte) (int, error)         { return 0, nil }
func (d *dummyConn) Write(b []byte) (int, error)        { return len(b), nil }
func (d *dummyConn) Close() error                       { d.closed = true; return nil }
func (d *dummyConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (d *dummyConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (d *dummyConn) SetDeadline(t time.Time) error      { return nil }
func (d *dummyConn) SetReadDeadline(t time.Time) error  { return nil }
func (d *dummyConn) SetWriteDeadline(t time.Time) error { return nil }

type hijackRW struct {
	http.ResponseWriter
	conn *dummyConn
}

func (h *hijackRW) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.conn = &dummyConn{}
	return h.conn, bufio.NewReadWriter(bufio.NewReader(nil), bufio.NewWriter(nil)), nil
}

func TestAddRemoveRouteAndFind(t *testing.T) {
	s := NewServer(Config{})

	// backend target
	u, _ := url.Parse("http://localhost:8080")
	// Add route
	if err := s.AddRoute([]string{"example.com"}, "/api", u.String(), map[string]string{"X-Test": "1"}, false, map[string]interface{}{"timeout": time.Second}); err != nil {
		t.Fatalf("AddRoute error: %v", err)
	}

	// find backend exact
	if b := s.findBackend("example.com", "/api"); b == nil {
		t.Fatalf("backend not found")
	}
	// find route for headers
	r := s.findRoute("example.com", "/api")
	rr := httptest.NewRecorder()
	s.applyHeaders(rr, r)
	if rr.Header().Get("X-Test") != "1" {
		t.Fatalf("route header not applied")
	}

	// Remove
	s.RemoveRoute([]string{"example.com"}, "/api")
	if s.findBackend("example.com", "/api") != nil {
		t.Fatalf("backend should be removed")
	}
}

func TestWildcardAndCertSelection(t *testing.T) {
	s := NewServer(Config{})
	// wildcard matching
	if !s.matchWildcard("*.example.com", "sub.example.com") {
		t.Fatalf("should match one-level subdomain")
	}
	if s.matchWildcard("*.example.com", "deep.sub.example.com") {
		t.Fatalf("should not match deep subdomains")
	}

	// certificate selection
	cert := tls.Certificate{}
	s.UpdateCertificates([]CertMapping{{Domains: []string{"example.com"}, Cert: cert}})
	if _, err := s.getCertificate(&tls.ClientHelloInfo{ServerName: "example.com"}); err != nil {
		t.Fatalf("getCertificate error: %v", err)
	}
}

func TestBlackholeCounts(t *testing.T) {
	s := NewServer(Config{})
	// send request with unknown host, use hijack-capable writer
	req := httptest.NewRequest(http.MethodGet, "http://unknown/", nil)
	rw := &hijackRW{ResponseWriter: httptest.NewRecorder()}
	s.ServeHTTP(rw, req)
	if s.GetBlackholeCount() != 1 {
		t.Fatalf("expected blackhole count=1")
	}
}

func TestCircuitBreakerOpensAndBlocks(t *testing.T) {
	// Backend returns 500 to trigger failures
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream error", http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	s := NewServer(Config{})
	u, _ := url.Parse(failSrv.URL)
	opts := map[string]interface{}{
		"circuit_breaker": map[string]interface{}{
			"enabled":           true,
			"failure_threshold": 3,
			"success_threshold": 1,
			"timeout":           100 * time.Millisecond,
			"window":            1 * time.Second,
		},
	}
	if err := s.AddRoute([]string{"cb.test"}, "/", u.String(), nil, false, opts); err != nil {
		t.Fatalf("AddRoute error: %v", err)
	}

	// Perform 3 failing requests (should proxy to upstream and get 500)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://cb.test/", nil)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusInternalServerError && rr.Code != http.StatusBadGateway {
			t.Fatalf("expected upstream failure status, got %d", rr.Code)
		}
	}

	// Next request should be blocked by open circuit (503)
	req := httptest.NewRequest(http.MethodGet, "http://cb.test/", nil)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when circuit open, got %d (body=%s)", rr.Code, rr.Body.String())
	}

	// After timeout, circuit moves to half-open; if backend still fails, it should re-open
	time.Sleep(120 * time.Millisecond)
	req2 := httptest.NewRequest(http.MethodGet, "http://cb.test/", nil)
	rr2 := httptest.NewRecorder()
	s.ServeHTTP(rr2, req2)
	// This request goes to upstream again (half-open), still 500, but then circuit should open again
	if rr2.Code != http.StatusInternalServerError && rr2.Code != http.StatusBadGateway {
		t.Fatalf("expected upstream failure during half-open, got %d", rr2.Code)
	}
}

func TestCircuitBreakerHalfOpenRecovery(t *testing.T) {
	var count int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count <= 3 {
			http.Error(w, "fail", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	s := NewServer(Config{})
	u, _ := url.Parse(srv.URL)
	opts := map[string]interface{}{
		"circuit_breaker": map[string]interface{}{
			"enabled":           true,
			"failure_threshold": 3,
			"success_threshold": 2,
			"timeout":           50 * time.Millisecond,
			"window":            1 * time.Second,
		},
	}
	if err := s.AddRoute([]string{"cb2.test"}, "/", u.String(), nil, false, opts); err != nil {
		t.Fatalf("AddRoute error: %v", err)
	}

	// Trip the circuit with 3 failures
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://cb2.test/", nil)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
	}

	// Should be open now
	reqOpen := httptest.NewRequest(http.MethodGet, "http://cb2.test/", nil)
	rrOpen := httptest.NewRecorder()
	s.ServeHTTP(rrOpen, reqOpen)
	if rrOpen.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when open, got %d", rrOpen.Code)
	}

	// Wait for half-open
	time.Sleep(60 * time.Millisecond)
	// Two successful requests should close the circuit
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "http://cb2.test/", nil)
		rr := httptest.NewRecorder()
		s.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 during recovery, got %d", rr.Code)
		}
	}

	// Circuit should be closed; requests continue normally
	reqFinal := httptest.NewRequest(http.MethodGet, "http://cb2.test/", nil)
	rrFinal := httptest.NewRecorder()
	s.ServeHTTP(rrFinal, reqFinal)
	if rrFinal.Code != http.StatusOK {
		t.Fatalf("expected 200 after close, got %d", rrFinal.Code)
	}
}
