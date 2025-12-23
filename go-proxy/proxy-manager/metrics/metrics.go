package metrics

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// Collector collects and exposes Prometheus-compatible metrics
type Collector struct {
	// Request counters
	totalRequests    uint64
	totalErrors      uint64
	requestsByStatus map[int]*uint64
	requestsByRoute  map[string]*RouteMetrics

	// Timing histograms
	requestDurations *Histogram

	// Bandwidth
	totalBytesSent     uint64
	totalBytesReceived uint64

	// Active connections
	activeConnections int64

	// WebSocket tracking
	websocketActive         int64
	websocketConnections    uint64
	websocketBytesToClient  uint64
	websocketBytesToBackend uint64
	websocketDurationSum    uint64 // nanoseconds

	// Retry tracking
	retryAttempts  uint64
	retrySuccesses uint64
	retryFailures  uint64

	// Slow requests
	slowWarnings  uint64
	slowCriticals uint64

	// Rate limiting
	rateLimitViolations uint64

	// WAF
	wafBlocks uint64

	// Start time
	startTime time.Time

	// Mutex for maps
	mu sync.RWMutex
}

// RouteMetrics tracks metrics for a specific route
type RouteMetrics struct {
	Requests      uint64
	Errors        uint64
	BytesSent     uint64
	BytesReceived uint64
	TotalDuration uint64 // nanoseconds
	ResponseTimes *Histogram
}

// Histogram tracks request duration distribution
type Histogram struct {
	buckets map[string]*uint64 // "0.1", "0.5", "1.0", "5.0", "10.0", "+Inf"
	sum     uint64
	count   uint64
	mu      sync.RWMutex
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	c := &Collector{
		requestsByStatus: make(map[int]*uint64),
		requestsByRoute:  make(map[string]*RouteMetrics),
		requestDurations: NewHistogram(),
		startTime:        time.Now(),
	}

	// Initialize common status codes
	for _, status := range []int{200, 201, 204, 301, 302, 304, 400, 401, 403, 404, 429, 500, 502, 503, 504} {
		count := uint64(0)
		c.requestsByStatus[status] = &count
	}

	return c
}

// NewHistogram creates a new histogram
func NewHistogram() *Histogram {
	h := &Histogram{
		buckets: make(map[string]*uint64),
	}

	// Initialize buckets (seconds)
	for _, bucket := range []string{"0.1", "0.5", "1.0", "5.0", "10.0", "+Inf"} {
		count := uint64(0)
		h.buckets[bucket] = &count
	}

	return h
}

// RecordRequest records a completed request
func (c *Collector) RecordRequest(route, method string, status int, duration time.Duration, bytesSent, bytesReceived uint64) {
	// Total counters
	atomic.AddUint64(&c.totalRequests, 1)
	if status >= 400 {
		atomic.AddUint64(&c.totalErrors, 1)
	}

	// Bandwidth
	atomic.AddUint64(&c.totalBytesSent, bytesSent)
	atomic.AddUint64(&c.totalBytesReceived, bytesReceived)

	// Status code counter
	c.mu.RLock()
	if counter, ok := c.requestsByStatus[status]; ok {
		atomic.AddUint64(counter, 1)
	} else {
		c.mu.RUnlock()
		c.mu.Lock()
		count := uint64(1)
		c.requestsByStatus[status] = &count
		c.mu.Unlock()
		c.mu.RLock()
	}
	c.mu.RUnlock()

	// Route metrics
	c.mu.Lock()
	routeKey := route + ":" + method
	rm, ok := c.requestsByRoute[routeKey]
	if !ok {
		rm = &RouteMetrics{
			ResponseTimes: NewHistogram(),
		}
		c.requestsByRoute[routeKey] = rm
	}
	c.mu.Unlock()

	atomic.AddUint64(&rm.Requests, 1)
	if status >= 400 {
		atomic.AddUint64(&rm.Errors, 1)
	}
	atomic.AddUint64(&rm.BytesSent, bytesSent)
	atomic.AddUint64(&rm.BytesReceived, bytesReceived)
	atomic.AddUint64(&rm.TotalDuration, uint64(duration.Nanoseconds()))

	// Record duration in histograms
	c.requestDurations.Observe(duration)
	rm.ResponseTimes.Observe(duration)
}

// Observe adds a duration observation to the histogram
func (h *Histogram) Observe(duration time.Duration) {
	seconds := duration.Seconds()

	h.mu.Lock()
	defer h.mu.Unlock()

	atomic.AddUint64(&h.sum, uint64(duration.Nanoseconds()))
	atomic.AddUint64(&h.count, 1)

	// Update buckets
	for bucket, counter := range h.buckets {
		if bucket == "+Inf" {
			atomic.AddUint64(counter, 1)
			continue
		}

		var threshold float64
		switch bucket {
		case "0.1":
			threshold = 0.1
		case "0.5":
			threshold = 0.5
		case "1.0":
			threshold = 1.0
		case "5.0":
			threshold = 5.0
		case "10.0":
			threshold = 10.0
		}

		if seconds <= threshold {
			atomic.AddUint64(counter, 1)
		}
	}
}

// IncrementActiveConnections increments the active connection counter
func (c *Collector) IncrementActiveConnections() {
	atomic.AddInt64(&c.activeConnections, 1)
}

// DecrementActiveConnections decrements the active connection counter
func (c *Collector) DecrementActiveConnections() {
	atomic.AddInt64(&c.activeConnections, -1)
}

// RecordRateLimitViolation records a rate limit violation
func (c *Collector) RecordRateLimitViolation() {
	atomic.AddUint64(&c.rateLimitViolations, 1)
}

// IncrementWebSocketActive increments active websocket connections and total count
func (c *Collector) IncrementWebSocketActive() {
	atomic.AddInt64(&c.websocketActive, 1)
	atomic.AddUint64(&c.websocketConnections, 1)
}

// DecrementWebSocketActive decrements active websocket connections
func (c *Collector) DecrementWebSocketActive() {
	atomic.AddInt64(&c.websocketActive, -1)
}

// RecordWebSocketTransfer records bytes and duration for a websocket session
func (c *Collector) RecordWebSocketTransfer(bytesToClient, bytesToBackend uint64, duration time.Duration) {
	atomic.AddUint64(&c.websocketBytesToClient, bytesToClient)
	atomic.AddUint64(&c.websocketBytesToBackend, bytesToBackend)
	atomic.AddUint64(&c.websocketDurationSum, uint64(duration.Nanoseconds()))
}

// RecordRetryAttempt increments retry attempts
func (c *Collector) RecordRetryAttempt() {
	atomic.AddUint64(&c.retryAttempts, 1)
}

// RecordRetrySuccess increments retry successes
func (c *Collector) RecordRetrySuccess() {
	atomic.AddUint64(&c.retrySuccesses, 1)
}

// RecordRetryFailure increments retry failures
func (c *Collector) RecordRetryFailure() {
	atomic.AddUint64(&c.retryFailures, 1)
}

// RecordSlowRequest records slow request events
func (c *Collector) RecordSlowRequest(level string) {
	switch level {
	case "warning":
		atomic.AddUint64(&c.slowWarnings, 1)
	case "critical":
		atomic.AddUint64(&c.slowCriticals, 1)
	}
}

// RecordWAFBlock records a WAF block
func (c *Collector) RecordWAFBlock() {
	atomic.AddUint64(&c.wafBlocks, 1)
}

// GetStats returns current statistics
func (c *Collector) GetStats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := Stats{
		Uptime:                  time.Since(c.startTime).Seconds(),
		TotalRequests:           atomic.LoadUint64(&c.totalRequests),
		TotalErrors:             atomic.LoadUint64(&c.totalErrors),
		TotalBytesSent:          atomic.LoadUint64(&c.totalBytesSent),
		TotalBytesReceived:      atomic.LoadUint64(&c.totalBytesReceived),
		ActiveConnections:       atomic.LoadInt64(&c.activeConnections),
		WebSocketActive:         atomic.LoadInt64(&c.websocketActive),
		WebSocketConnections:    atomic.LoadUint64(&c.websocketConnections),
		WebSocketBytesToClient:  atomic.LoadUint64(&c.websocketBytesToClient),
		WebSocketBytesToBackend: atomic.LoadUint64(&c.websocketBytesToBackend),
		RateLimitViolations:     atomic.LoadUint64(&c.rateLimitViolations),
		WAFBlocks:               atomic.LoadUint64(&c.wafBlocks),
		RetryAttempts:           atomic.LoadUint64(&c.retryAttempts),
		RetrySuccesses:          atomic.LoadUint64(&c.retrySuccesses),
		RetryFailures:           atomic.LoadUint64(&c.retryFailures),
		SlowWarnings:            atomic.LoadUint64(&c.slowWarnings),
		SlowCriticals:           atomic.LoadUint64(&c.slowCriticals),
		RequestsByStatus:        make(map[int]uint64),
		RouteMetrics:            make(map[string]RouteStats),
	}

	wsDurSum := atomic.LoadUint64(&c.websocketDurationSum)
	if stats.WebSocketConnections > 0 && wsDurSum > 0 {
		stats.WebSocketAverageDuration = float64(wsDurSum) / float64(stats.WebSocketConnections) / 1e9
	}

	// Copy status code counters
	for status, counter := range c.requestsByStatus {
		stats.RequestsByStatus[status] = atomic.LoadUint64(counter)
	}

	// Copy route metrics
	for route, rm := range c.requestsByRoute {
		avgDuration := float64(0)
		if rm.Requests > 0 {
			avgDuration = float64(atomic.LoadUint64(&rm.TotalDuration)) / float64(atomic.LoadUint64(&rm.Requests)) / 1e9 // Convert to seconds
		}

		stats.RouteMetrics[route] = RouteStats{
			Requests:        atomic.LoadUint64(&rm.Requests),
			Errors:          atomic.LoadUint64(&rm.Errors),
			BytesSent:       atomic.LoadUint64(&rm.BytesSent),
			BytesReceived:   atomic.LoadUint64(&rm.BytesReceived),
			AverageDuration: avgDuration,
		}
	}

	// Calculate error rate
	if stats.TotalRequests > 0 {
		stats.ErrorRate = float64(stats.TotalErrors) / float64(stats.TotalRequests) * 100
	}

	return stats
}

// Stats represents current metrics statistics
type Stats struct {
	Uptime                   float64               `json:"uptime_seconds"`
	TotalRequests            uint64                `json:"total_requests"`
	TotalErrors              uint64                `json:"total_errors"`
	ErrorRate                float64               `json:"error_rate_percent"`
	TotalBytesSent           uint64                `json:"total_bytes_sent"`
	TotalBytesReceived       uint64                `json:"total_bytes_received"`
	ActiveConnections        int64                 `json:"active_connections"`
	WebSocketActive          int64                 `json:"websocket_active"`
	WebSocketConnections     uint64                `json:"websocket_connections"`
	WebSocketBytesToClient   uint64                `json:"websocket_bytes_to_client"`
	WebSocketBytesToBackend  uint64                `json:"websocket_bytes_to_backend"`
	WebSocketAverageDuration float64               `json:"websocket_average_duration_seconds"`
	RateLimitViolations      uint64                `json:"rate_limit_violations"`
	WAFBlocks                uint64                `json:"waf_blocks"`
	RetryAttempts            uint64                `json:"retry_attempts"`
	RetrySuccesses           uint64                `json:"retry_successes"`
	RetryFailures            uint64                `json:"retry_failures"`
	SlowWarnings             uint64                `json:"slow_request_warnings"`
	SlowCriticals            uint64                `json:"slow_request_criticals"`
	RequestsByStatus         map[int]uint64        `json:"requests_by_status"`
	RouteMetrics             map[string]RouteStats `json:"route_metrics"`
}

// RouteStats represents metrics for a specific route
type RouteStats struct {
	Requests        uint64  `json:"requests"`
	Errors          uint64  `json:"errors"`
	BytesSent       uint64  `json:"bytes_sent"`
	BytesReceived   uint64  `json:"bytes_received"`
	AverageDuration float64 `json:"average_duration_seconds"`
}

// PrometheusMetrics returns metrics in Prometheus exposition format
func (c *Collector) PrometheusMetrics() string {
	stats := c.GetStats()
	var out string

	// uptime
	out += "# HELP proxy_uptime_seconds Proxy uptime in seconds\n"
	out += "# TYPE proxy_uptime_seconds gauge\n"
	out += formatMetric("proxy_uptime_seconds", stats.Uptime)

	// total requests
	out += "# HELP proxy_requests_total Total number of HTTP requests\n"
	out += "# TYPE proxy_requests_total counter\n"
	out += formatMetric("proxy_requests_total", stats.TotalRequests)

	// total errors
	out += "# HELP proxy_errors_total Total number of HTTP errors\n"
	out += "# TYPE proxy_errors_total counter\n"
	out += formatMetric("proxy_errors_total", stats.TotalErrors)

	// error rate
	out += "# HELP proxy_error_rate_percent Current error rate percentage\n"
	out += "# TYPE proxy_error_rate_percent gauge\n"
	out += formatMetric("proxy_error_rate_percent", stats.ErrorRate)

	// bandwidth
	out += "# HELP proxy_bytes_sent_total Total bytes sent to clients\n"
	out += "# TYPE proxy_bytes_sent_total counter\n"
	out += formatMetric("proxy_bytes_sent_total", stats.TotalBytesSent)

	out += "# HELP proxy_bytes_received_total Total bytes received from clients\n"
	out += "# TYPE proxy_bytes_received_total counter\n"
	out += formatMetric("proxy_bytes_received_total", stats.TotalBytesReceived)

	// active connections
	out += "# HELP proxy_active_connections Current number of active connections\n"
	out += "# TYPE proxy_active_connections gauge\n"
	out += formatMetric("proxy_active_connections", stats.ActiveConnections)

	out += "# HELP proxy_websocket_active Current active WebSocket connections\n"
	out += "# TYPE proxy_websocket_active gauge\n"
	out += formatMetric("proxy_websocket_active", stats.WebSocketActive)

	out += "# HELP proxy_websocket_connections_total Total WebSocket connections established\n"
	out += "# TYPE proxy_websocket_connections_total counter\n"
	out += formatMetric("proxy_websocket_connections_total", stats.WebSocketConnections)

	out += "# HELP proxy_websocket_bytes_to_client_total Total WebSocket bytes sent to clients\n"
	out += "# TYPE proxy_websocket_bytes_to_client_total counter\n"
	out += formatMetric("proxy_websocket_bytes_to_client_total", stats.WebSocketBytesToClient)

	out += "# HELP proxy_websocket_bytes_to_backend_total Total WebSocket bytes sent to backends\n"
	out += "# TYPE proxy_websocket_bytes_to_backend_total counter\n"
	out += formatMetric("proxy_websocket_bytes_to_backend_total", stats.WebSocketBytesToBackend)

	out += "# HELP proxy_websocket_average_duration_seconds Average WebSocket session duration in seconds\n"
	out += "# TYPE proxy_websocket_average_duration_seconds gauge\n"
	out += formatMetric("proxy_websocket_average_duration_seconds", stats.WebSocketAverageDuration)

	// rate limiting
	out += "# HELP proxy_rate_limit_violations_total Total rate limit violations\n"
	out += "# TYPE proxy_rate_limit_violations_total counter\n"
	out += formatMetric("proxy_rate_limit_violations_total", stats.RateLimitViolations)

	// WAF
	out += "# HELP proxy_waf_blocks_total Total WAF blocks\n"
	out += "# TYPE proxy_waf_blocks_total counter\n"
	out += formatMetric("proxy_waf_blocks_total", stats.WAFBlocks)

	out += "# HELP proxy_retry_attempts_total Total retry attempts\n"
	out += "# TYPE proxy_retry_attempts_total counter\n"
	out += formatMetric("proxy_retry_attempts_total", stats.RetryAttempts)

	out += "# HELP proxy_retry_successes_total Successful retries\n"
	out += "# TYPE proxy_retry_successes_total counter\n"
	out += formatMetric("proxy_retry_successes_total", stats.RetrySuccesses)

	out += "# HELP proxy_retry_failures_total Failed retries\n"
	out += "# TYPE proxy_retry_failures_total counter\n"
	out += formatMetric("proxy_retry_failures_total", stats.RetryFailures)

	out += "# HELP proxy_slow_request_warnings_total Slow request warnings (>= warning threshold)\n"
	out += "# TYPE proxy_slow_request_warnings_total counter\n"
	out += formatMetric("proxy_slow_request_warnings_total", stats.SlowWarnings)

	out += "# HELP proxy_slow_request_criticals_total Slow request criticals (>= critical threshold)\n"
	out += "# TYPE proxy_slow_request_criticals_total counter\n"
	out += formatMetric("proxy_slow_request_criticals_total", stats.SlowCriticals)

	// requests by status code
	out += "# HELP proxy_requests_by_status_total Total requests by HTTP status code\n"
	out += "# TYPE proxy_requests_by_status_total counter\n"
	for status, count := range stats.RequestsByStatus {
		out += formatMetricWithLabel("proxy_requests_by_status_total", count, "status", status)
	}

	// route metrics
	out += "# HELP proxy_route_requests_total Total requests per route\n"
	out += "# TYPE proxy_route_requests_total counter\n"
	for route, rm := range stats.RouteMetrics {
		out += formatMetricWithLabel("proxy_route_requests_total", rm.Requests, "route", route)
	}

	out += "# HELP proxy_route_errors_total Total errors per route\n"
	out += "# TYPE proxy_route_errors_total counter\n"
	for route, rm := range stats.RouteMetrics {
		out += formatMetricWithLabel("proxy_route_errors_total", rm.Errors, "route", route)
	}

	out += "# HELP proxy_route_duration_average_seconds Average request duration per route\n"
	out += "# TYPE proxy_route_duration_average_seconds gauge\n"
	for route, rm := range stats.RouteMetrics {
		out += formatMetricWithLabel("proxy_route_duration_average_seconds", rm.AverageDuration, "route", route)
	}

	return out
}

// Helper functions for formatting Prometheus metrics
func formatMetric(name string, value interface{}) string {
	return name + " " + toString(value) + "\n"
}

func formatMetricWithLabel(name string, value interface{}, labelName string, labelValue interface{}) string {
	return name + "{" + labelName + "=\"" + toString(labelValue) + "\"} " + toString(value) + "\n"
}

func toString(value interface{}) string {
	switch v := value.(type) {
	case int:
		return formatInt(int64(v))
	case int64:
		return formatInt(v)
	case uint64:
		return formatUint(v)
	case float64:
		return formatFloat(v)
	case string:
		return v
	default:
		return ""
	}
}

func formatInt(value int64) string {
	return formatFloat(float64(value))
}

func formatUint(value uint64) string {
	return formatFloat(float64(value))
}

func formatFloat(value float64) string {
	// Prometheus requires specific formatting
	if value == 0 {
		return "0"
	}
	return formatFloatString(value)
}

func formatFloatString(value float64) string {
	s := ""
	if value < 0 {
		s = "-"
		value = -value
	}

	// Simple float to string conversion for Prometheus
	if value == float64(int64(value)) {
		s += formatIntString(int64(value))
	} else {
		s += formatFloatPrecision(value, 6)
	}

	return s
}

func formatIntString(value int64) string {
	if value == 0 {
		return "0"
	}

	var result []byte
	neg := value < 0
	if neg {
		value = -value
	}

	for value > 0 {
		result = append([]byte{byte('0' + value%10)}, result...)
		value /= 10
	}

	if neg {
		result = append([]byte{'-'}, result...)
	}

	return string(result)
}

func formatFloatPrecision(value float64, precision int) string {
	// Simple float formatting without using fmt
	intPart := int64(value)
	fracPart := value - float64(intPart)

	result := formatIntString(intPart) + "."

	for i := 0; i < precision; i++ {
		fracPart *= 10
		digit := int(fracPart)
		result += string(byte('0' + digit))
		fracPart -= float64(digit)
	}

	return result
}

// LogStats logs current statistics
func (c *Collector) LogStats() {
	stats := c.GetStats()

	log.Info().
		Float64("uptime_hours", stats.Uptime/3600).
		Uint64("total_requests", stats.TotalRequests).
		Uint64("total_errors", stats.TotalErrors).
		Float64("error_rate", stats.ErrorRate).
		Int64("active_connections", stats.ActiveConnections).
		Msg("Metrics summary")
}
