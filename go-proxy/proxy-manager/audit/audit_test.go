package audit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockDB struct {
	entries []AuditEntry
	logged  int
}

func (m *mockDB) LogAudit(user, action, resourceType, resourceID, oldValue, newValue, ipAddress, metadata string) error {
	m.logged++
	return nil
}
func (m *mockDB) GetAuditLogs(limit int, action, resourceType string, since time.Time) ([]AuditEntry, error) {
	return m.entries, nil
}

func TestAuditLoggerBasic(t *testing.T) {
	db := &mockDB{}
	l := NewLogger(db, true)

	if err := l.LogConfigReload("/etc/proxy/config.yaml", "manual"); err != nil {
		t.Fatalf("LogConfigReload error: %v", err)
	}
	if err := l.LogRouteChange(ActionRouteAdd, []string{"a.com"}, "/x", "http://up"); err != nil {
		t.Fatalf("LogRouteChange error: %v", err)
	}
	if err := l.LogCertUpdate([]string{"a.com"}); err != nil {
		t.Fatalf("LogCertUpdate error: %v", err)
	}
	if err := l.LogServiceChange(ActionServiceRegister, "svc", "host", 80); err != nil {
		t.Fatalf("LogServiceChange error: %v", err)
	}
	if err := l.LogStartup("v1"); err != nil {
		t.Fatalf("LogStartup error: %v", err)
	}
	if err := l.LogShutdown("tests"); err != nil {
		t.Fatalf("LogShutdown error: %v", err)
	}

	if db.logged < 6 {
		t.Fatalf("expected >=6 logged entries, got %d", db.logged)
	}

	stats := l.GetStats()
	if stats["enabled"].(bool) != true {
		t.Fatalf("stats enabled mismatch")
	}

	// API handler returns JSON
	db.entries = []AuditEntry{{ID: 1, User: "u", Action: ActionStartup}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/audit?limit=10", nil)
	h := l.APIHandler()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Count   int
		Entries []AuditEntry
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json parse: %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("expected count=1")
	}
}
