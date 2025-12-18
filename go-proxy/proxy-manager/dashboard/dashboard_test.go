package dashboard

import (
	"testing"
	"time"
)

func TestDashboardNew(t *testing.T) {
	d := New(nil, nil, nil, nil, true)
	if !d.IsEnabled() {
		t.Error("Dashboard should be enabled")
	}
}

func TestDashboardDisabled(t *testing.T) {
	d := New(nil, nil, nil, nil, false)
	if d.IsEnabled() {
		t.Error("Dashboard should be disabled")
	}
}

func TestSystemStats(t *testing.T) {
	d := New(nil, nil, nil, nil, true)
	stats := d.getSystemStats()

	if stats == nil {
		t.Error("Stats should not be nil")
	}

	if stats.Uptime == 0 {
		t.Error("Uptime should be set")
	}

	if stats.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestAIContextGeneration(t *testing.T) {
	d := New(nil, nil, nil, nil, true)
	context := d.generateAIContext()

	if context == "" {
		t.Error("Context should not be empty")
	}

	if !contains(context, "Proxy Manager State Export") {
		t.Error("Context should contain header")
	}

	if !contains(context, "System Statistics") {
		t.Error("Context should contain stats section")
	}

	if !contains(context, "Routes") {
		t.Error("Context should contain routes section")
	}
}

func TestDashboardDataStructures(t *testing.T) {
	data := &DashboardData{
		SystemStats: &SystemStats{
			Uptime:           time.Hour,
			ActiveConnection: 42,
			RequestsPerSec:   15.3,
			ErrorRate:        0.0002,
			TotalRequests:    1000,
			TotalErrors:      0,
			Timestamp:        time.Now(),
		},
		Routes:       []RouteStatus{},
		Certificates: []CertStatus{},
		RecentErrors: []ErrorLog{},
		GeneratedAt:  time.Now(),
	}

	if data.SystemStats.Uptime != time.Hour {
		t.Error("SystemStats not properly set")
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
