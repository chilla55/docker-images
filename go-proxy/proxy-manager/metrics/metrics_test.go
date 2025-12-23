package metrics

import (
	"strings"
	"testing"
	"time"
)

func TestCollectorRecordRequestAndStats(t *testing.T) {
	c := NewCollector()
	c.RecordRequest("/api", "GET", 200, 120*time.Millisecond, 1024, 2048)
	c.RecordRequest("/api", "GET", 500, 2*time.Second, 256, 512)

	stats := c.GetStats()
	if stats.TotalRequests != 2 {
		t.Fatalf("expected 2 requests, got %d", stats.TotalRequests)
	}
	if stats.TotalErrors != 1 {
		t.Fatalf("expected 1 error, got %d", stats.TotalErrors)
	}
	if stats.TotalBytesSent == 0 || stats.TotalBytesReceived == 0 {
		t.Fatal("expected bandwidth counters > 0")
	}

	rs, ok := stats.RouteMetrics["/api:GET"]
	if !ok {
		t.Fatal("expected route metrics for /api:GET")
	}
	if rs.Requests != 2 {
		t.Fatalf("expected 2 route requests, got %d", rs.Requests)
	}
	if rs.Errors != 1 {
		t.Fatalf("expected 1 route error, got %d", rs.Errors)
	}
	if rs.AverageDuration <= 0 {
		t.Fatal("expected positive average duration")
	}
}

func TestPrometheusMetricsContainsKeys(t *testing.T) {
	c := NewCollector()
	c.RecordRequest("/health", "GET", 200, 50*time.Millisecond, 0, 0)
	out := c.PrometheusMetrics()
	for _, key := range []string{
		"proxy_uptime_seconds",
		"proxy_requests_total",
		"proxy_errors_total",
		"proxy_error_rate_percent",
		"proxy_bytes_sent_total",
		"proxy_bytes_received_total",
	} {
		if !strings.Contains(out, key) {
			t.Fatalf("prometheus output missing %s", key)
		}
	}
}
