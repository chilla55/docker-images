package proxy

import (
	"bufio"
	"crypto/tls"
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
