package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection
type DB struct {
	*sql.DB
}

// Open opens a SQLite database connection and initializes schema
func Open(path string) (*DB, error) {
	log.Info().Str("path", path).Msg("Opening database")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(1) // SQLite only supports 1 writer
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	wrapper := &DB{DB: db}

	// Initialize schema
	if err := wrapper.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	log.Info().Msg("Database initialized successfully")
	return wrapper, nil
}

// initSchema creates all required tables
func (db *DB) initSchema() error {
	schema := `
	-- Enable foreign keys
	PRAGMA foreign_keys = ON;
	PRAGMA journal_mode = WAL;

	-- Service Registry
	CREATE TABLE IF NOT EXISTS services (
		service_id INTEGER PRIMARY KEY AUTOINCREMENT,
		service_name TEXT NOT NULL,
		container_id TEXT,
		ip_address TEXT,
		port INTEGER NOT NULL,
		protocol TEXT DEFAULT 'http',
		status TEXT DEFAULT 'unknown',
		last_check INTEGER,
		metadata TEXT,
		UNIQUE(service_name, port)
	);

	-- Route Configurations
	CREATE TABLE IF NOT EXISTS routes (
		route_id INTEGER PRIMARY KEY AUTOINCREMENT,
		domain TEXT NOT NULL,
		path TEXT DEFAULT '/',
		service_id INTEGER,
		backend_url TEXT NOT NULL,
		enabled INTEGER DEFAULT 1,
		config_hash TEXT,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now')),
		FOREIGN KEY(service_id) REFERENCES services(service_id)
	);
	CREATE INDEX IF NOT EXISTS idx_routes_domain ON routes(domain);
	CREATE INDEX IF NOT EXISTS idx_routes_enabled ON routes(enabled);

	-- Time-Series Metrics
	CREATE TABLE IF NOT EXISTS metrics (
		metric_id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		route_id INTEGER,
		service_id INTEGER,
		metric_type TEXT NOT NULL,
		value REAL NOT NULL,
		labels TEXT,
		FOREIGN KEY(route_id) REFERENCES routes(route_id),
		FOREIGN KEY(service_id) REFERENCES services(service_id)
	);
	CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics(timestamp);
	CREATE INDEX IF NOT EXISTS idx_metrics_service ON metrics(service_id);
	CREATE INDEX IF NOT EXISTS idx_metrics_type ON metrics(metric_type);

	-- Request Logs
	CREATE TABLE IF NOT EXISTS request_logs (
		log_id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		route_id INTEGER,
		service_id INTEGER,
		request_id TEXT NOT NULL,
		client_ip TEXT NOT NULL,
		method TEXT NOT NULL,
		path TEXT NOT NULL,
		status_code INTEGER,
		response_time_ms INTEGER,
		bytes_sent INTEGER,
		bytes_received INTEGER,
		user_agent TEXT,
		referer TEXT,
		error_message TEXT,
		FOREIGN KEY(route_id) REFERENCES routes(route_id),
		FOREIGN KEY(service_id) REFERENCES services(service_id)
	);
	CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON request_logs(timestamp);
	CREATE INDEX IF NOT EXISTS idx_logs_status ON request_logs(status_code);
	CREATE INDEX IF NOT EXISTS idx_logs_request_id ON request_logs(request_id);
	CREATE INDEX IF NOT EXISTS idx_logs_client_ip ON request_logs(client_ip);

	-- Health Checks
	CREATE TABLE IF NOT EXISTS health_checks (
		check_id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		service_id INTEGER NOT NULL,
		success INTEGER NOT NULL,
		response_time_ms INTEGER,
		error_message TEXT,
		FOREIGN KEY(service_id) REFERENCES services(service_id)
	);
	CREATE INDEX IF NOT EXISTS idx_health_timestamp ON health_checks(timestamp);
	CREATE INDEX IF NOT EXISTS idx_health_service ON health_checks(service_id);

	-- Certificates
	CREATE TABLE IF NOT EXISTS certificates (
		cert_id INTEGER PRIMARY KEY AUTOINCREMENT,
		domain TEXT NOT NULL UNIQUE,
		cert_path TEXT NOT NULL,
		key_path TEXT NOT NULL,
		issuer TEXT,
		subject TEXT,
		not_before INTEGER,
		not_after INTEGER,
		san_domains TEXT,
		auto_renew INTEGER DEFAULT 1,
		last_check INTEGER,
		created_at INTEGER DEFAULT (strftime('%s', 'now')),
		updated_at INTEGER DEFAULT (strftime('%s', 'now'))
	);
	CREATE INDEX IF NOT EXISTS idx_certs_domain ON certificates(domain);
	CREATE INDEX IF NOT EXISTS idx_certs_expiry ON certificates(not_after);

	-- Rate Limits
	CREATE TABLE IF NOT EXISTS rate_limits (
		ip_address TEXT NOT NULL,
		route_id INTEGER NOT NULL,
		window_start INTEGER NOT NULL,
		request_count INTEGER DEFAULT 1,
		last_request INTEGER NOT NULL,
		PRIMARY KEY (ip_address, route_id, window_start),
		FOREIGN KEY(route_id) REFERENCES routes(route_id)
	);
	CREATE INDEX IF NOT EXISTS idx_ratelimit_window ON rate_limits(window_start);

	-- Rate Limit Violations
	CREATE TABLE IF NOT EXISTS rate_limit_violations (
		violation_id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		ip_address TEXT NOT NULL,
		route_id INTEGER,
		request_count INTEGER,
		limit_value INTEGER,
		action TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_violations_timestamp ON rate_limit_violations(timestamp);
	CREATE INDEX IF NOT EXISTS idx_violations_ip ON rate_limit_violations(ip_address);

	-- WAF Blocks
	CREATE TABLE IF NOT EXISTS waf_blocks (
		block_id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		ip_address TEXT NOT NULL,
		route_id INTEGER,
		attack_type TEXT NOT NULL,
		pattern_matched TEXT,
		request_path TEXT,
		request_method TEXT,
		action TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_waf_timestamp ON waf_blocks(timestamp);
	CREATE INDEX IF NOT EXISTS idx_waf_ip ON waf_blocks(ip_address);
	CREATE INDEX IF NOT EXISTS idx_waf_type ON waf_blocks(attack_type);

	-- Audit Log
	CREATE TABLE IF NOT EXISTS audit_log (
		audit_id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp INTEGER NOT NULL,
		user TEXT,
		action TEXT NOT NULL,
		resource_type TEXT,
		resource_id TEXT,
		old_value TEXT,
		new_value TEXT,
		ip_address TEXT,
		notes TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_log(timestamp);
	CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action);

	-- WebSocket Connections
	CREATE TABLE IF NOT EXISTS websocket_connections (
		conn_id INTEGER PRIMARY KEY AUTOINCREMENT,
		request_id TEXT NOT NULL,
		route_id INTEGER,
		client_ip TEXT NOT NULL,
		connected_at INTEGER NOT NULL,
		disconnected_at INTEGER,
		bytes_sent INTEGER DEFAULT 0,
		bytes_received INTEGER DEFAULT 0,
		messages_sent INTEGER DEFAULT 0,
		messages_received INTEGER DEFAULT 0,
		close_reason TEXT,
		FOREIGN KEY(route_id) REFERENCES routes(route_id)
	);
	CREATE INDEX IF NOT EXISTS idx_ws_connected ON websocket_connections(connected_at);
	CREATE INDEX IF NOT EXISTS idx_ws_active ON websocket_connections(disconnected_at) WHERE disconnected_at IS NULL;
	`

	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to execute schema: %w", err)
	}

	return nil
}

// LogRequest logs an HTTP request
func (db *DB) LogRequest(req *RequestLog) error {
	query := `
		INSERT INTO request_logs (
			timestamp, route_id, service_id, request_id, client_ip,
			method, path, status_code, response_time_ms,
			bytes_sent, bytes_received, user_agent, referer, error_message
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(query,
		req.Timestamp, req.RouteID, req.ServiceID, req.RequestID, req.ClientIP,
		req.Method, req.Path, req.StatusCode, req.ResponseTimeMs,
		req.BytesSent, req.BytesReceived, req.UserAgent, req.Referer, req.ErrorMessage,
	)

	return err
}

// RecordMetric records a metric value
func (db *DB) RecordMetric(metric *Metric) error {
	query := `
		INSERT INTO metrics (timestamp, route_id, service_id, metric_type, value, labels)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(query,
		metric.Timestamp, metric.RouteID, metric.ServiceID, metric.Type, metric.Value, metric.Labels,
	)

	return err
}

// CleanupOldData removes data older than retention period
func (db *DB) CleanupOldData(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()

	queries := []string{
		fmt.Sprintf("DELETE FROM request_logs WHERE timestamp < %d", cutoff),
		fmt.Sprintf("DELETE FROM metrics WHERE timestamp < %d", cutoff),
		fmt.Sprintf("DELETE FROM health_checks WHERE timestamp < %d", cutoff),
		fmt.Sprintf("DELETE FROM rate_limits WHERE window_start < %d", cutoff),
		fmt.Sprintf("DELETE FROM rate_limit_violations WHERE timestamp < %d", cutoff),
		fmt.Sprintf("DELETE FROM waf_blocks WHERE timestamp < %d", cutoff),
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			log.Warn().Err(err).Str("query", query).Msg("Failed to cleanup old data")
		}
	}

	// Vacuum to reclaim space
	if _, err := db.Exec("VACUUM"); err != nil {
		log.Warn().Err(err).Msg("Failed to vacuum database")
	}

	return nil
}

// RequestLog represents a logged HTTP request
type RequestLog struct {
	Timestamp      int64
	RouteID        *int64
	ServiceID      *int64
	RequestID      string
	ClientIP       string
	Method         string
	Path           string
	StatusCode     int
	ResponseTimeMs int64
	BytesSent      int64
	BytesReceived  int64
	UserAgent      string
	Referer        string
	ErrorMessage   string
}

// Metric represents a collected metric
type Metric struct {
	Timestamp int64
	RouteID   *int64
	ServiceID *int64
	Type      string
	Value     float64
	Labels    string
}

// LogRateLimitViolation logs a rate limit violation to the database
func (db *DB) LogRateLimitViolation(ip, route, reason string, requestCount int) error {
	query := `
		INSERT INTO rate_limit_violations (
			timestamp, ip_address, route, reason, 
			request_count, metadata
		) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(
		query,
		time.Now().Unix(),
		ip,
		route,
		reason,
		requestCount,
		"", // metadata (JSON for additional info)
	)

	if err != nil {
		log.Error().
			Err(err).
			Str("ip", ip).
			Str("route", route).
			Str("reason", reason).
			Msg("Failed to log rate limit violation")
		return err
	}

	return nil
}

// LogWAFBlock logs a WAF block event to the database
func (db *DB) LogWAFBlock(ip, route, attackType, payload, userAgent string) error {
	query := `
		INSERT INTO waf_blocks (
			timestamp, ip_address, route, attack_type,
			payload, user_agent, blocked, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(
		query,
		time.Now().Unix(),
		ip,
		route,
		attackType,
		payload,
		userAgent,
		1,  // blocked = true
		"", // metadata (JSON for additional info)
	)

	if err != nil {
		log.Error().
			Err(err).
			Str("ip", ip).
			Str("route", route).
			Str("attack_type", attackType).
			Msg("Failed to log WAF block")
		return err
	}

	return nil
}

// LogAudit logs an audit event to the database
func (db *DB) LogAudit(user, action, resourceType, resourceID, oldValue, newValue, ipAddress, metadata string) error {
	query := `
		INSERT INTO audit_log (
			timestamp, user, action, resource_type,
			resource_id, old_value, new_value, ip_address, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(
		query,
		time.Now().Unix(),
		user,
		action,
		resourceType,
		resourceID,
		oldValue,
		newValue,
		ipAddress,
		metadata,
	)

	if err != nil {
		log.Error().
			Err(err).
			Str("action", action).
			Str("resource_type", resourceType).
			Msg("Failed to log audit entry")
		return err
	}

	return nil
}

// AuditEntry represents an audit log entry
type AuditEntry struct {
	ID           int64  `json:"id"`
	Timestamp    int64  `json:"timestamp"`
	User         string `json:"user"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	OldValue     string `json:"old_value,omitempty"`
	NewValue     string `json:"new_value,omitempty"`
	IPAddress    string `json:"ip_address,omitempty"`
	Metadata     string `json:"metadata,omitempty"`
}

// GetAuditLogs retrieves audit logs with optional filters
func (db *DB) GetAuditLogs(limit int, action, resourceType string, since time.Time) ([]AuditEntry, error) {
	query := `
		SELECT id, timestamp, user, action, resource_type,
		       resource_id, old_value, new_value, ip_address, metadata
		FROM audit_log
		WHERE 1=1
	`
	args := []interface{}{}

	if action != "" {
		query += " AND action = ?"
		args = append(args, action)
	}

	if resourceType != "" {
		query += " AND resource_type = ?"
		args = append(args, resourceType)
	}

	if !since.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, since.Unix())
	}

	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var entry AuditEntry
		err := rows.Scan(
			&entry.ID,
			&entry.Timestamp,
			&entry.User,
			&entry.Action,
			&entry.ResourceType,
			&entry.ResourceID,
			&entry.OldValue,
			&entry.NewValue,
			&entry.IPAddress,
			&entry.Metadata,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to scan audit entry")
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// CleanupAccessLogs removes old access logs based on retention policy
func (db *DB) CleanupAccessLogs(days int, routePattern string) error {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()

	var query string
	var args []interface{}

	if routePattern == "*" {
		query = "DELETE FROM access_log WHERE timestamp < ?"
		args = []interface{}{cutoff}
	} else {
		query = "DELETE FROM access_log WHERE timestamp < ? AND route LIKE ?"
		args = []interface{}{cutoff, routePattern}
	}

	result, err := db.Exec(query, args...)
	if err != nil {
		log.Error().
			Err(err).
			Int("days", days).
			Str("route_pattern", routePattern).
			Msg("Failed to cleanup access logs")
		return err
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		log.Info().
			Int64("rows_deleted", rows).
			Int("days", days).
			Str("route_pattern", routePattern).
			Msg("Cleaned up access logs")
	}

	return nil
}

// CleanupSecurityLogs removes old security logs (WAF blocks, rate limit violations)
func (db *DB) CleanupSecurityLogs(days int, routePattern string) error {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()

	var totalRows int64

	// Cleanup WAF blocks
	var wafQuery string
	var wafArgs []interface{}

	if routePattern == "*" {
		wafQuery = "DELETE FROM waf_blocks WHERE timestamp < ?"
		wafArgs = []interface{}{cutoff}
	} else {
		wafQuery = "DELETE FROM waf_blocks WHERE timestamp < ? AND route LIKE ?"
		wafArgs = []interface{}{cutoff, routePattern}
	}

	wafResult, err := db.Exec(wafQuery, wafArgs...)
	if err != nil {
		log.Error().
			Err(err).
			Int("days", days).
			Str("route_pattern", routePattern).
			Msg("Failed to cleanup WAF blocks")
		return err
	}

	wafRows, _ := wafResult.RowsAffected()
	totalRows += wafRows

	// Cleanup rate limit violations
	var rlQuery string
	var rlArgs []interface{}

	if routePattern == "*" {
		rlQuery = "DELETE FROM rate_limit_violations WHERE timestamp < ?"
		rlArgs = []interface{}{cutoff}
	} else {
		rlQuery = "DELETE FROM rate_limit_violations WHERE timestamp < ? AND route LIKE ?"
		rlArgs = []interface{}{cutoff, routePattern}
	}

	rlResult, err := db.Exec(rlQuery, rlArgs...)
	if err != nil {
		log.Error().
			Err(err).
			Int("days", days).
			Str("route_pattern", routePattern).
			Msg("Failed to cleanup rate limit violations")
		return err
	}

	rlRows, _ := rlResult.RowsAffected()
	totalRows += rlRows

	if totalRows > 0 {
		log.Info().
			Int64("rows_deleted", totalRows).
			Int64("waf_blocks", wafRows).
			Int64("rate_limit_violations", rlRows).
			Int("days", days).
			Str("route_pattern", routePattern).
			Msg("Cleaned up security logs")
	}

	return nil
}

// CleanupAuditLogs removes old audit logs
func (db *DB) CleanupAuditLogs(days int) error {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()

	result, err := db.Exec("DELETE FROM audit_log WHERE timestamp < ?", cutoff)
	if err != nil {
		log.Error().
			Err(err).
			Int("days", days).
			Msg("Failed to cleanup audit logs")
		return err
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		log.Info().
			Int64("rows_deleted", rows).
			Int("days", days).
			Msg("Cleaned up audit logs")
	}

	return nil
}

// CleanupMetrics removes old metrics data
func (db *DB) CleanupMetrics(days int, routePattern string) error {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()

	var query string
	var args []interface{}

	if routePattern == "*" {
		query = "DELETE FROM metrics WHERE timestamp < ?"
		args = []interface{}{cutoff}
	} else {
		query = "DELETE FROM metrics WHERE timestamp < ? AND route LIKE ?"
		args = []interface{}{cutoff, routePattern}
	}

	result, err := db.Exec(query, args...)
	if err != nil {
		log.Error().
			Err(err).
			Int("days", days).
			Str("route_pattern", routePattern).
			Msg("Failed to cleanup metrics")
		return err
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		log.Info().
			Int64("rows_deleted", rows).
			Int("days", days).
			Str("route_pattern", routePattern).
			Msg("Cleaned up metrics")
	}

	return nil
}

// CleanupHealthChecks removes old health check logs
func (db *DB) CleanupHealthChecks(days int) error {
	cutoff := time.Now().AddDate(0, 0, -days).Unix()

	result, err := db.Exec("DELETE FROM health_checks WHERE timestamp < ?", cutoff)
	if err != nil {
		log.Error().
			Err(err).
			Int("days", days).
			Msg("Failed to cleanup health checks")
		return err
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		log.Info().
			Int64("rows_deleted", rows).
			Int("days", days).
			Msg("Cleaned up health checks")
	}

	return nil
}

// RecordHealthCheck records a health check result
func (db *DB) RecordHealthCheck(service, url string, success bool, duration time.Duration, statusCode int, errorMsg string) error {
	query := `
		INSERT INTO health_checks (
			timestamp, service_name, url, success, response_time_ms, status_code, error
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.Exec(
		query,
		time.Now().Unix(),
		service,
		url,
		success,
		duration.Milliseconds(),
		statusCode,
		errorMsg,
	)

	if err != nil {
		log.Error().
			Err(err).
			Str("service", service).
			Msg("Failed to record health check")
		return err
	}

	return nil
}

// GetHealthCheckHistory retrieves health check history for a service
func (db *DB) GetHealthCheckHistory(service string, limit int) ([]HealthCheckResult, error) {
	query := `
		SELECT timestamp, service_name, url, success, response_time_ms, status_code, error
		FROM health_checks
		WHERE service_name = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`

	rows, err := db.Query(query, service, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []HealthCheckResult
	for rows.Next() {
		var result HealthCheckResult
		var errorMsg sql.NullString

		err := rows.Scan(
			&result.Timestamp,
			&result.Service,
			&result.URL,
			&result.Success,
			&result.Duration,
			&result.StatusCode,
			&errorMsg,
		)
		if err != nil {
			log.Error().Err(err).Msg("Failed to scan health check result")
			continue
		}

		if errorMsg.Valid {
			result.Error = errorMsg.String
		}

		results = append(results, result)
	}

	return results, nil
}

// HealthCheckResult represents a health check result from database
type HealthCheckResult struct {
	Timestamp  int64  `json:"timestamp"`
	Service    string `json:"service"`
	URL        string `json:"url"`
	Success    bool   `json:"success"`
	Duration   int64  `json:"duration_ms"`
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}
