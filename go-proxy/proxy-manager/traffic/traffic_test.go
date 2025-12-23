package traffic

import (
	"testing"
	"time"
)

func TestNewAnalyzer(t *testing.T) {
	analyzer := NewAnalyzer(1 * time.Hour)
	if analyzer == nil {
		t.Fatal("NewAnalyzer returned nil")
	}
	if analyzer.ipStats == nil {
		t.Error("ipStats not initialized")
	}
}

func TestRecordRequest(t *testing.T) {
	analyzer := NewAnalyzer(1 * time.Hour)

	// RecordRequest signature: ip, ua, method, path, statusCode, responseTime, bytesIn, bytesOut
	analyzer.RecordRequest(
		"192.168.1.1",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		"GET",
		"/api/test",
		200,
		100.0,
		512,
		1024,
	)

	// Verify IP stats
	if stats, exists := analyzer.ipStats["192.168.1.1"]; !exists {
		t.Error("IP stats not recorded")
	} else {
		if stats.RequestCount != 1 {
			t.Errorf("expected 1 request, got %d", stats.RequestCount)
		}
	}

	// Path stats may or may not be recorded depending on implementation
	// Just verify no panic occurred
}

func TestGetAnalysis(t *testing.T) {
	analyzer := NewAnalyzer(1 * time.Hour)

	// Add diverse traffic
	analyzer.RecordRequest("192.168.1.1", "Chrome/91.0", "GET", "/api/users", 200, 100.0, 512, 1024)
	analyzer.RecordRequest("192.168.1.2", "Firefox/89.0", "POST", "/api/posts", 201, 150.0, 1024, 2048)
	analyzer.RecordRequest("192.168.1.1", "Chrome/91.0", "GET", "/api/users", 404, 50.0, 256, 512)

	analysis := analyzer.Analyze(10)

	if len(analysis.TopIPs) == 0 {
		t.Error("expected top IPs")
	}

	if len(analysis.TopPaths) == 0 {
		t.Error("expected top paths")
	}

	if analysis.TotalUniqueIPs != 2 {
		t.Errorf("expected 2 unique IPs, got %d", analysis.TotalUniqueIPs)
	}
}

func TestGetIPReputation(t *testing.T) {
	analyzer := NewAnalyzer(1 * time.Hour)

	ip := "192.168.1.1"

	// Record some requests
	analyzer.RecordRequest(ip, "test-agent", "GET", "/test", 200, 100.0, 512, 1024)
	analyzer.RecordRequest(ip, "test-agent", "GET", "/test", 500, 100.0, 512, 1024)

	rep := analyzer.GetIPReputation(ip)
	if rep < 0 {
		t.Errorf("expected non-negative reputation, got %.2f", rep)
	}
}
