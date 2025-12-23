package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockDB struct{ called int }

func (m *mockDB) RecordHealthCheck(service, url string, success bool, duration time.Duration, statusCode int, err string) error {
	m.called++
	return nil
}
func (m *mockDB) GetHealthCheckHistory(service string, limit int) ([]HealthCheckResult, error) {
	return nil, nil
}

func TestCheckerCheckSuccessAndFailure(t *testing.T) {
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer okSrv.Close()

	badSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer badSrv.Close()

	db := &mockDB{}
	c := NewChecker(db)

	// Directly construct service entries and run check
	s1 := &ServiceHealth{Name: "svc-ok", URL: okSrv.URL, Timeout: 100 * time.Millisecond, ExpectedStatus: 200}
	s2 := &ServiceHealth{Name: "svc-bad", URL: badSrv.URL, Timeout: 100 * time.Millisecond, ExpectedStatus: 200}

	c.check(s1)
	c.check(s2)

	if s1.Status != StatusHealthy || s1.SuccessCount == 0 {
		t.Fatalf("svc-ok should be healthy: %+v", s1)
	}
	if s2.Status == StatusHealthy || s2.FailureCount == 0 {
		t.Fatalf("svc-bad should have failures: %+v", s2)
	}

	// calculateStatus thresholds
	if st := c.calculateStatus(0, 0); st != StatusUnknown {
		t.Fatalf("expected unknown")
	}
	if st := c.calculateStatus(9, 10); st != StatusHealthy {
		t.Fatalf("expected healthy")
	}
	if st := c.calculateStatus(5, 10); st != StatusDegraded {
		t.Fatalf("expected degraded")
	}
	if st := c.calculateStatus(4, 10); st != StatusDown {
		t.Fatalf("expected down")
	}

	// GetAllStatuses on added services
	c.AddService("svc-added", okSrv.URL, 1*time.Second, 100*time.Millisecond, 200)
	statuses := c.GetAllStatuses()
	if len(statuses) == 0 {
		t.Fatalf("expected statuses")
	}
}

func TestGetStatusErrorsAndUnhealthyList(t *testing.T) {
	c := NewChecker(nil)
	if _, err := c.GetStatus("missing"); err == nil {
		t.Fatalf("expected error for missing service")
	}

	s := &ServiceHealth{Name: "svc", Status: StatusDown}
	c.services = map[string]*ServiceHealth{"svc": s}
	if !c.IsHealthy() {
		// since svc is down, IsHealthy should be false
	} else {
		t.Fatalf("expected IsHealthy to be false when service down")
	}

	unhealthy := c.GetUnhealthyServices()
	if len(unhealthy) == 0 {
		t.Fatalf("expected at least one unhealthy service")
	}
}
