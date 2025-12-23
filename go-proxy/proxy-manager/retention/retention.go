package retention

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// Policy represents a data retention policy
type Policy struct {
	Name             string
	RoutePattern     string // Route pattern to match (e.g., "*.private.com")
	AccessLogDays    int    // Days to keep access logs
	SecurityLogDays  int    // Days to keep security logs (WAF, rate limit)
	AuditLogDays     int    // Days to keep audit logs
	MetricsDays      int    // Days to keep metrics
	HealthCheckDays  int    // Days to keep health check logs
	WebSocketLogDays int    // Days to keep WebSocket logs
}

// Manager manages data retention policies
type Manager struct {
	policies        []Policy
	db              Database
	cleanupInterval time.Duration
	defaultPolicy   Policy
}

// Database interface for retention operations
type Database interface {
	CleanupOldData(retentionDays int) error
	CleanupAccessLogs(days int, routePattern string) error
	CleanupSecurityLogs(days int, routePattern string) error
	CleanupAuditLogs(days int) error
	CleanupMetrics(days int, routePattern string) error
	CleanupHealthChecks(days int) error
}

// NewManager creates a new retention policy manager
func NewManager(db Database, cleanupInterval time.Duration) *Manager {
	// Default policy (applies if no specific policy matches)
	defaultPolicy := Policy{
		Name:             "default",
		RoutePattern:     "*",
		AccessLogDays:    30,  // 30 days for general access logs
		SecurityLogDays:  90,  // 90 days for security events
		AuditLogDays:     365, // 1 year for audit logs
		MetricsDays:      90,  // 90 days for metrics
		HealthCheckDays:  7,   // 7 days for health checks
		WebSocketLogDays: 30,  // 30 days for WebSocket logs
	}

	if cleanupInterval == 0 {
		cleanupInterval = 24 * time.Hour // Daily by default
	}

	return &Manager{
		policies:        make([]Policy, 0),
		db:              db,
		cleanupInterval: cleanupInterval,
		defaultPolicy:   defaultPolicy,
	}
}

// AddPolicy adds a retention policy
func (m *Manager) AddPolicy(policy Policy) {
	m.policies = append(m.policies, policy)
	log.Info().
		Str("name", policy.Name).
		Str("route_pattern", policy.RoutePattern).
		Int("access_log_days", policy.AccessLogDays).
		Int("security_log_days", policy.SecurityLogDays).
		Int("audit_log_days", policy.AuditLogDays).
		Msg("Added retention policy")
}

// AddPublicPolicy adds a policy for public-facing services
func (m *Manager) AddPublicPolicy(routePattern string) {
	m.AddPolicy(Policy{
		Name:             "public",
		RoutePattern:     routePattern,
		AccessLogDays:    7,  // 7 days for public access logs
		SecurityLogDays:  30, // 30 days for security events
		AuditLogDays:     90, // 90 days for audit
		MetricsDays:      30, // 30 days for metrics
		HealthCheckDays:  7,  // 7 days for health checks
		WebSocketLogDays: 7,  // 7 days for WebSocket
	})
}

// AddPrivatePolicy adds a policy for private/sensitive services
func (m *Manager) AddPrivatePolicy(routePattern string) {
	m.AddPolicy(Policy{
		Name:             "private",
		RoutePattern:     routePattern,
		AccessLogDays:    30,  // 30 days for private access logs
		SecurityLogDays:  90,  // 90 days for security events
		AuditLogDays:     365, // 1 year for audit (compliance)
		MetricsDays:      90,  // 90 days for metrics
		HealthCheckDays:  7,   // 7 days for health checks
		WebSocketLogDays: 30,  // 30 days for WebSocket
	})
}

// Start starts the retention policy manager
func (m *Manager) Start(ctx context.Context) {
	log.Info().
		Dur("interval", m.cleanupInterval).
		Int("policies", len(m.policies)).
		Msg("Starting retention policy manager")

	// Run initial cleanup after a short delay
	time.Sleep(30 * time.Second)
	m.runCleanup()

	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Retention policy manager stopped")
			return
		case <-ticker.C:
			m.runCleanup()
		}
	}
}

// runCleanup runs cleanup based on all policies
func (m *Manager) runCleanup() {
	log.Info().Msg("Running retention policy cleanup")
	startTime := time.Now()

	// Apply each policy
	for _, policy := range m.policies {
		m.applyPolicy(policy)
	}

	// Apply default policy for unmatched routes
	m.applyPolicy(m.defaultPolicy)

	duration := time.Since(startTime)
	log.Info().
		Dur("duration", duration).
		Msg("Retention policy cleanup completed")
}

// applyPolicy applies a single retention policy
func (m *Manager) applyPolicy(policy Policy) {
	log.Debug().
		Str("policy", policy.Name).
		Str("route_pattern", policy.RoutePattern).
		Msg("Applying retention policy")

	// Cleanup access logs
	if policy.AccessLogDays > 0 {
		if err := m.db.CleanupAccessLogs(policy.AccessLogDays, policy.RoutePattern); err != nil {
			log.Error().
				Err(err).
				Str("policy", policy.Name).
				Msg("Failed to cleanup access logs")
		}
	}

	// Cleanup security logs (WAF blocks, rate limit violations)
	if policy.SecurityLogDays > 0 {
		if err := m.db.CleanupSecurityLogs(policy.SecurityLogDays, policy.RoutePattern); err != nil {
			log.Error().
				Err(err).
				Str("policy", policy.Name).
				Msg("Failed to cleanup security logs")
		}
	}

	// Cleanup audit logs (global, not per-route)
	if policy.AuditLogDays > 0 && policy.RoutePattern == "*" {
		if err := m.db.CleanupAuditLogs(policy.AuditLogDays); err != nil {
			log.Error().
				Err(err).
				Str("policy", policy.Name).
				Msg("Failed to cleanup audit logs")
		}
	}

	// Cleanup metrics
	if policy.MetricsDays > 0 {
		if err := m.db.CleanupMetrics(policy.MetricsDays, policy.RoutePattern); err != nil {
			log.Error().
				Err(err).
				Str("policy", policy.Name).
				Msg("Failed to cleanup metrics")
		}
	}

	// Cleanup health checks (global)
	if policy.HealthCheckDays > 0 && policy.RoutePattern == "*" {
		if err := m.db.CleanupHealthChecks(policy.HealthCheckDays); err != nil {
			log.Error().
				Err(err).
				Str("policy", policy.Name).
				Msg("Failed to cleanup health checks")
		}
	}
}

// GetPolicies returns all configured policies
func (m *Manager) GetPolicies() []Policy {
	policies := make([]Policy, 0, len(m.policies)+1)
	policies = append(policies, m.policies...)
	policies = append(policies, m.defaultPolicy)
	return policies
}

// GetStats returns retention policy statistics
func (m *Manager) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"cleanup_interval_hours": m.cleanupInterval.Hours(),
		"policies_count":         len(m.policies),
		"default_policy": map[string]int{
			"access_log_days":    m.defaultPolicy.AccessLogDays,
			"security_log_days":  m.defaultPolicy.SecurityLogDays,
			"audit_log_days":     m.defaultPolicy.AuditLogDays,
			"metrics_days":       m.defaultPolicy.MetricsDays,
			"health_check_days":  m.defaultPolicy.HealthCheckDays,
			"websocket_log_days": m.defaultPolicy.WebSocketLogDays,
		},
	}
}
