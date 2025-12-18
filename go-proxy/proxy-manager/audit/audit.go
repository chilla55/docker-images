package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// Action types for audit logging
const (
	ActionConfigReload    = "config_reload"
	ActionRouteAdd        = "route_add"
	ActionRouteRemove     = "route_remove"
	ActionRouteUpdate     = "route_update"
	ActionCertUpdate      = "certificate_update"
	ActionServiceRegister = "service_register"
	ActionServiceRemove   = "service_remove"
	ActionRateLimitChange = "rate_limit_change"
	ActionWAFRuleChange   = "waf_rule_change"
	ActionStartup         = "startup"
	ActionShutdown        = "shutdown"
)

// Logger handles audit logging
type Logger struct {
	db      Database
	enabled bool
}

// Database interface for audit logging
type Database interface {
	LogAudit(user, action, resourceType, resourceID, oldValue, newValue, ipAddress, metadata string) error
	GetAuditLogs(limit int, action, resourceType string, since time.Time) ([]AuditEntry, error)
}

// AuditEntry represents an audit log entry
type AuditEntry struct {
	ID           int64
	Timestamp    int64
	User         string
	Action       string
	ResourceType string
	ResourceID   string
	OldValue     string
	NewValue     string
	IPAddress    string
	Metadata     string
}

// NewLogger creates a new audit logger
func NewLogger(db Database, enabled bool) *Logger {
	return &Logger{
		db:      db,
		enabled: enabled,
	}
}

// Log logs an audit event
func (l *Logger) Log(action, resourceType, resourceID, oldValue, newValue, ipAddress, metadata string) error {
	if !l.enabled {
		return nil
	}

	user := "system" // Default user, could be extended to support actual users

	if err := l.db.LogAudit(user, action, resourceType, resourceID, oldValue, newValue, ipAddress, metadata); err != nil {
		log.Error().
			Err(err).
			Str("action", action).
			Str("resource_type", resourceType).
			Str("resource_id", resourceID).
			Msg("Failed to log audit entry")
		return err
	}

	log.Info().
		Str("action", action).
		Str("resource_type", resourceType).
		Str("resource_id", resourceID).
		Msg("Audit log recorded")

	return nil
}

// LogConfigReload logs a configuration reload event
func (l *Logger) LogConfigReload(configPath, reason string) error {
	metadata := map[string]string{
		"reason": reason,
	}
	metadataJSON, _ := json.Marshal(metadata)

	return l.Log(
		ActionConfigReload,
		"config",
		configPath,
		"",
		"",
		"",
		string(metadataJSON),
	)
}

// LogRouteChange logs a route add/remove/update event
func (l *Logger) LogRouteChange(action string, domains []string, path, backend string) error {
	domainsStr := fmt.Sprintf("%v", domains)
	routeInfo := fmt.Sprintf("%s -> %s", path, backend)

	return l.Log(
		action,
		"route",
		domainsStr,
		"",
		routeInfo,
		"",
		"",
	)
}

// LogCertUpdate logs a certificate update event
func (l *Logger) LogCertUpdate(domains []string) error {
	domainsStr := fmt.Sprintf("%v", domains)

	return l.Log(
		ActionCertUpdate,
		"certificate",
		domainsStr,
		"",
		"",
		"",
		"",
	)
}

// LogServiceChange logs a service registration/removal event
func (l *Logger) LogServiceChange(action, serviceName, hostname string, port int) error {
	serviceID := fmt.Sprintf("%s:%s:%d", serviceName, hostname, port)

	return l.Log(
		action,
		"service",
		serviceID,
		"",
		"",
		"",
		"",
	)
}

// LogStartup logs application startup
func (l *Logger) LogStartup(version string) error {
	metadata := map[string]string{
		"version": version,
	}
	metadataJSON, _ := json.Marshal(metadata)

	return l.Log(
		ActionStartup,
		"application",
		"proxy-manager",
		"",
		"",
		"",
		string(metadataJSON),
	)
}

// LogShutdown logs application shutdown
func (l *Logger) LogShutdown(reason string) error {
	metadata := map[string]string{
		"reason": reason,
	}
	metadataJSON, _ := json.Marshal(metadata)

	return l.Log(
		ActionShutdown,
		"application",
		"proxy-manager",
		"",
		"",
		"",
		string(metadataJSON),
	)
}

// APIHandler returns an HTTP handler for querying audit logs
func (l *Logger) APIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse query parameters
		query := r.URL.Query()
		limitStr := query.Get("limit")
		action := query.Get("action")
		resourceType := query.Get("resource_type")
		sinceStr := query.Get("since")

		// Default limit
		limit := 100
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
				if limit > 1000 {
					limit = 1000 // Max 1000 entries
				}
			}
		}

		// Parse since timestamp
		var since time.Time
		if sinceStr != "" {
			if ts, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
				since = time.Unix(ts, 0)
			}
		}

		// Get audit logs
		entries, err := l.db.GetAuditLogs(limit, action, resourceType, since)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get audit logs")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Return as JSON
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"count":   len(entries),
			"entries": entries,
		}); err != nil {
			log.Error().Err(err).Msg("Failed to encode audit logs")
		}
	}
}

// GetStats returns audit logging statistics
func (l *Logger) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled": l.enabled,
	}
}

// Start starts background tasks (currently none needed)
func (l *Logger) Start(ctx context.Context) {
	log.Info().Bool("enabled", l.enabled).Msg("Audit logger initialized")
}
