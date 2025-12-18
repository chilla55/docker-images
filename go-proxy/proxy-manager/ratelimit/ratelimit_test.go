package ratelimit

import (
    "net/http"
    "net/http/httptest"
    "testing"
)

type mockDB struct{}

func (m *mockDB) LogRateLimitViolation(ip, route, reason string, requestCount int) error { return nil }

func TestAllowMinuteLimit(t *testing.T) {
    cfg := Config{Enabled: true, RequestsPerMin: 2, RequestsPerHour: 100, BurstSize: 0}
    l := NewLimiter(cfg, &mockDB{})

    allowed, _ := l.Allow("1.2.3.4", "/test")
    if !allowed { t.Fatal("first request should be allowed") }

    allowed, _ = l.Allow("1.2.3.4", "/test")
    if !allowed { t.Fatal("second request should be allowed") }

    allowed, reason := l.Allow("1.2.3.4", "/test")
    if allowed { t.Fatal("third request should be blocked by minute limit") }
    if reason == "" { t.Error("expected a blocking reason") }
}

func TestAllowHourlyLimit(t *testing.T) {
    cfg := Config{Enabled: true, RequestsPerMin: 1000, RequestsPerHour: 3}
    l := NewLimiter(cfg, &mockDB{})

    for i := 0; i < 3; i++ {
        allowed, _ := l.Allow("5.6.7.8", "/hour")
        if !allowed { t.Fatalf("request %d should be allowed", i+1) }
    }
    allowed, _ := l.Allow("5.6.7.8", "/hour")
    if allowed { t.Fatal("4th request should be blocked by hourly limit") }
}

func TestMiddlewareWithXFF(t *testing.T) {
    cfg := Config{Enabled: true, RequestsPerMin: 2, RequestsPerHour: 100}
    l := NewLimiter(cfg, &mockDB{})

    hits := 0
    next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.WriteHeader(http.StatusOK) })
    mw := l.Middleware("/route")(next)

    req := httptest.NewRequest("GET", "/route", nil)
    req.Header.Set("X-Forwarded-For", "203.0.113.1")
    rec := httptest.NewRecorder()
    mw.ServeHTTP(rec, req)
    if rec.Code != http.StatusOK { t.Fatalf("expected 200, got %d", rec.Code) }

    rec2 := httptest.NewRecorder()
    mw.ServeHTTP(rec2, req)
    if rec2.Code != http.StatusOK { t.Fatalf("expected 200, got %d", rec2.Code) }

    rec3 := httptest.NewRecorder()
    mw.ServeHTTP(rec3, req)
    if rec3.Code != http.StatusTooManyRequests { t.Fatalf("expected 429, got %d", rec3.Code) }
    if hits != 2 { t.Fatalf("next handler should be called twice, got %d", hits) }
}

func TestWhitelistCIDR(t *testing.T) {
    cfg := Config{Enabled: true, RequestsPerMin: 1, RequestsPerHour: 1}
    l := NewLimiter(cfg, &mockDB{})
    if err := l.AddWhitelist("203.0.113.0/24"); err != nil { t.Fatal(err) }

    hits := 0
    next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++; w.WriteHeader(http.StatusOK) })
    mw := l.Middleware("/wl")(next)

    req := httptest.NewRequest("GET", "/wl", nil)
    req.Header.Set("X-Forwarded-For", "203.0.113.42")
    for i := 0; i < 5; i++ {
        rec := httptest.NewRecorder()
        mw.ServeHTTP(rec, req)
        if rec.Code != http.StatusOK { t.Fatalf("whitelisted request expected 200, got %d", rec.Code) }
    }
    if hits != 5 { t.Fatalf("expected 5 passes to next, got %d", hits) }
}

func TestGetStats(t *testing.T) {
    cfg := Config{Enabled: true, RequestsPerMin: 1, RequestsPerHour: 10}
    l := NewLimiter(cfg, &mockDB{})
    l.Allow("9.9.9.9", "/stats")
    stats := l.GetStats()
    if stats["active_windows"].(int) == 0 { t.Error("expected active windows > 0") }
}
