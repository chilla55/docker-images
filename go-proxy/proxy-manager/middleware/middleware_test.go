package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestID(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Context().Value("request_id")
		if id == nil {
			t.Fatalf("request_id missing in context")
		}
		w.WriteHeader(200)
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	RequestID(next).ServeHTTP(rr, req)
	if rr.Header().Get("X-Request-ID") == "" {
		t.Fatalf("X-Request-ID header not set")
	}
}

func TestTimeout(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(200)
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	Timeout(10*time.Millisecond)(next).ServeHTTP(rr, req)
	if rr.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 504, got %d", rr.Code)
	}
}

func TestLimitRequestBody(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", NewBody("0123456789"))
	req.ContentLength = 10
	LimitRequestBody(5)(next).ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rr.Code)
	}
}

func TestLimitResponseBodySize(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789"))
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	LimitResponseBodySize(5)(next).ServeHTTP(rr, req)
	if rr.Code != http.StatusInsufficientStorage {
		t.Fatalf("expected 507, got %d", rr.Code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	headers := map[string]string{"X-Test": "1", "X-Frame-Options": "DENY"}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	SecurityHeaders(headers)(next).ServeHTTP(rr, req)
	if rr.Header().Get("X-Test") != "1" || rr.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatalf("security headers not set correctly")
	}
}

// helper for building bodies quickly
type body struct{ data []byte }

func (b body) Read(p []byte) (int, error) { copy(p, b.data); return len(b.data), nil }
func (b body) Close() error               { return nil }
func NewBody(s string) body               { return body{data: []byte(s)} }
