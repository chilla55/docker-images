package accesslog

import (
	"sync/atomic"
	"testing"
	"time"
)

type mockDB struct{ calls int64 }

func (m *mockDB) LogAccessRequest(entry AccessLogEntry) error {
	atomic.AddInt64(&m.calls, 1)
	return nil
}
func (m *mockDB) GetRecentRequests(limit int) ([]AccessLogEntry, error) { return nil, nil }
func (m *mockDB) GetRequestsByRoute(route string, limit int) ([]AccessLogEntry, error) {
	return nil, nil
}
func (m *mockDB) GetErrorRequests(limit int) ([]AccessLogEntry, error) { return nil, nil }

func TestLoggerBasic(t *testing.T) {
	db := &mockDB{}
	l := NewLogger(db, 10)

	e1 := AccessLogEntry{Domain: "example.com", Method: "GET", Path: "/", Status: 200, ClientIP: "1.2.3.4", ResponseTimeMs: 12}
	e2 := AccessLogEntry{Domain: "example.com", Method: "POST", Path: "/login", Status: 401, ClientIP: "1.2.3.4", ResponseTimeMs: 30}

	l.LogRequest(e1)
	l.LogRequest(e2)

	// wait for async DB writes
	time.Sleep(20 * time.Millisecond)
	if atomic.LoadInt64(&db.calls) < 2 {
		t.Fatalf("expected db log calls >=2, got %d", db.calls)
	}

	recent := l.GetRecentRequests(10)
	if len(recent) == 0 {
		t.Fatalf("expected recent entries")
	}

	errs := l.GetRecentErrors(10)
	foundErr := false
	for _, e := range errs {
		if e.Status >= 400 {
			foundErr = true
			break
		}
	}
	if !foundErr {
		t.Fatalf("expected at least one error entry")
	}

	stats := l.GetStats()
	if stats.TotalEntries == 0 || stats.BufferSize != 10 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	if !l.IsEnabled() {
		t.Fatalf("logger should be enabled")
	}
	l.Disable()
	if l.IsEnabled() {
		t.Fatalf("logger should be disabled")
	}
	l.Enable()
	l.Clear()
}
