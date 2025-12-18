package waf

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

type mockDB struct{}

func (m *mockDB) LogWAFBlock(ip, route, attackType, payload, userAgent string) error { return nil }

func TestMiddlewareBlocksMaliciousPath(t *testing.T) {
    cfg := Config{Enabled: true, BlockMode: true, CheckPath: true}
    w := NewWAF(cfg, &mockDB{})

    next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { rw.WriteHeader(http.StatusOK) })
    handler := w.Middleware("/api")(next)

    req := httptest.NewRequest("GET", "/search/<script>alert('x')</script>", nil)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)
    if rec.Code != http.StatusForbidden {
        t.Fatalf("expected 403, got %d", rec.Code)
    }
}

func TestLogOnlyModeAllows(t *testing.T) {
    cfg := Config{Enabled: true, BlockMode: false, CheckPath: true}
    w := NewWAF(cfg, &mockDB{})

    next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { rw.WriteHeader(http.StatusOK) })
    handler := w.Middleware("/api")(next)

    req := httptest.NewRequest("GET", "/search/<script>alert('x')</script>", nil)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK {
        t.Fatalf("expected 200 in log-only mode, got %d", rec.Code)
    }
}

func TestHeaderInspection(t *testing.T) {
    cfg := Config{Enabled: true, BlockMode: true, CheckHeaders: true}
    w := NewWAF(cfg, &mockDB{})

    next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { rw.WriteHeader(http.StatusOK) })
    handler := w.Middleware("/hdr")(next)

    req := httptest.NewRequest("GET", "/hdr", nil)
    req.Header.Set("X-Whatever", "javascript:alert(1)")
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)
    if rec.Code != http.StatusForbidden {
        t.Fatalf("expected 403 for malicious header, got %d", rec.Code)
    }
}

func TestQueryInspection(t *testing.T) {
    cfg := Config{Enabled: true, BlockMode: true, CheckQuery: true}
    w := NewWAF(cfg, &mockDB{})

    next := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { rw.WriteHeader(http.StatusOK) })
    handler := w.Middleware("/q")(next)

    req := httptest.NewRequest("GET", "/q?q=..%2F..%2Fetc%2Fpasswd", nil)
    rec := httptest.NewRecorder()
    handler.ServeHTTP(rec, req)
    if rec.Code != http.StatusForbidden {
        t.Fatalf("expected 403 for path traversal in query, got %d", rec.Code)
    }
}

func TestGetStats(t *testing.T) {
    cfg := Config{Enabled: true, BlockMode: true, CheckPath: true}
    w := NewWAF(cfg, &mockDB{})
    stats := w.GetStats()
    if stats["enabled"].(bool) != true { t.Error("expected WAF enabled") }
    if stats["rules_count"].(int) == 0 { t.Error("expected non-zero rules count") }
}
