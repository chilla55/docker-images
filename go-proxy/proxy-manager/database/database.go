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
