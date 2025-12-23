package maintenance

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewMaintenanceManager(t *testing.T) {
	m := New()
	if m == nil {
		t.Error("Manager should not be nil")
	}
}

func TestSetMaintenanceMode(t *testing.T) {
	m := New()
	domain := "example.com"

	err := m.SetMaintenanceMode(domain, true, "", "Server upgrades", time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("Failed to enable maintenance: %v", err)
	}

	if !m.IsMaintenanceMode(domain) {
		t.Error("Domain should be in maintenance mode")
	}
}

func TestMaintenanceModeDisable(t *testing.T) {
	m := New()
	domain := "example.com"

	m.SetMaintenanceMode(domain, true, "", "Test", time.Now().Add(24*time.Hour))
	if !m.IsMaintenanceMode(domain) {
		t.Error("Should be in maintenance mode")
	}

	m.SetMaintenanceMode(domain, false, "", "", time.Time{})
	if m.IsMaintenanceMode(domain) {
		t.Error("Should not be in maintenance mode after disable")
	}
}

func TestScheduledDisable(t *testing.T) {
	m := New()
	domain := "example.com"

	// Schedule maintenance to end 100ms from now
	endTime := time.Now().Add(100 * time.Millisecond)
	m.SetMaintenanceMode(domain, true, "", "Test", endTime)

	if !m.IsMaintenanceMode(domain) {
		t.Error("Should be in maintenance mode immediately")
	}

	// Wait for scheduled disable
	time.Sleep(200 * time.Millisecond)

	if m.IsMaintenanceMode(domain) {
		t.Error("Should not be in maintenance mode after scheduled end time")
	}
}

func TestRenderMaintenancePage(t *testing.T) {
	m := New()
	domain := "example.com"

	m.SetMaintenanceMode(domain, true, "<h1>Custom Maintenance</h1>", "Upgrades", time.Now().Add(24*time.Hour))

	w := httptest.NewRecorder()
	m.RenderMaintenancePage(w, domain)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	if !contains(w.Body.String(), "<h1>Custom Maintenance</h1>") {
		t.Error("Response should contain custom HTML")
	}
}

func TestRenderDefaultMaintenancePage(t *testing.T) {
	m := New()
	domain := "api.example.com"

	m.SetMaintenanceMode(domain, true, "", "Database migration", time.Now().Add(24*time.Hour))

	w := httptest.NewRecorder()
	m.RenderMaintenancePage(w, domain)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	body := w.Body.String()
	if !contains(body, "Maintenance in Progress") {
		t.Error("Should contain maintenance title")
	}

	if !contains(body, "api.example.com") {
		t.Error("Should contain domain name")
	}

	if !contains(body, "Database migration") {
		t.Error("Should contain reason")
	}
}

func TestGetMaintenanceState(t *testing.T) {
	m := New()
	domain := "example.com"

	m.SetMaintenanceMode(domain, true, "<h1>Test</h1>", "Test reason", time.Now().Add(24*time.Hour))

	state := m.GetMaintenanceState(domain)
	if state == nil {
		t.Error("State should not be nil")
	}

	if state.HTMLContent != "<h1>Test</h1>" {
		t.Error("HTML content mismatch")
	}

	if state.Reason != "Test reason" {
		t.Error("Reason mismatch")
	}
}

func TestGetAll(t *testing.T) {
	m := New()

	m.SetMaintenanceMode("example.com", true, "", "", time.Now().Add(24*time.Hour))
	m.SetMaintenanceMode("api.example.com", true, "", "", time.Now().Add(24*time.Hour))
	m.SetMaintenanceMode("www.example.com", false, "", "", time.Time{})

	states := m.GetAll()
	if len(states) != 2 {
		t.Errorf("Expected 2 maintenance states, got %d", len(states))
	}

	if _, ok := states["example.com"]; !ok {
		t.Error("Should contain example.com")
	}

	if _, ok := states["api.example.com"]; !ok {
		t.Error("Should contain api.example.com")
	}
}

func TestDisableAll(t *testing.T) {
	m := New()

	m.SetMaintenanceMode("example.com", true, "", "", time.Now().Add(24*time.Hour))
	m.SetMaintenanceMode("api.example.com", true, "", "", time.Now().Add(24*time.Hour))

	m.DisableAll()

	if m.IsMaintenanceMode("example.com") {
		t.Error("example.com should not be in maintenance mode")
	}

	if m.IsMaintenanceMode("api.example.com") {
		t.Error("api.example.com should not be in maintenance mode")
	}
}

func TestOnStateChange(t *testing.T) {
	m := New()
	domain := "example.com"

	m.OnStateChange(domain, func() {
		// Just verify it accepts callbacks without panic
	})

	m.SetMaintenanceMode(domain, true, "", "", time.Now().Add(time.Hour))

	if !m.IsMaintenanceMode(domain) {
		t.Error("Should be in maintenance mode after SetMaintenanceMode")
	}
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
