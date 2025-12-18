package retention

import (
    "testing"
    "time"
)

type mockDB struct{}
func (m mockDB) CleanupOldData(retentionDays int) error { return nil }
func (m mockDB) CleanupAccessLogs(days int, routePattern string) error { return nil }
func (m mockDB) CleanupSecurityLogs(days int, routePattern string) error { return nil }
func (m mockDB) CleanupAuditLogs(days int) error { return nil }
func (m mockDB) CleanupMetrics(days int, routePattern string) error { return nil }
func (m mockDB) CleanupHealthChecks(days int) error { return nil }

func TestPoliciesAndStats(t *testing.T) {
    m := NewManager(mockDB{}, 2*time.Hour)
    m.AddPublicPolicy("public.example.com")
    m.AddPrivatePolicy("private.example.com")
    m.AddPolicy(Policy{Name: "custom", RoutePattern: "*.x", AccessLogDays: 3, SecurityLogDays: 5, AuditLogDays: 7, MetricsDays: 9, HealthCheckDays: 2})

    policies := m.GetPolicies()
    if len(policies) < 2 {
        t.Fatalf("expected policies to include default and added ones")
    }

    stats := m.GetStats()
    if stats["policies_count"].(int) < 2 { t.Fatalf("expected policies_count >= 2") }
    if stats["cleanup_interval_hours"].(float64) != 2.0 { t.Fatalf("interval hours mismatch") }
}
