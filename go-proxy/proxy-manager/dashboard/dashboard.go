package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// SystemStats holds current system metrics
type SystemStats struct {
	UptimeMs         int64     `json:"uptime_ms"`
	ActiveConnection int64     `json:"active_connections"`
	RequestsPerSec   float64   `json:"requests_per_sec"`
	ErrorRate        float64   `json:"error_rate"`
	TotalRequests    int64     `json:"total_requests"`
	TotalErrors      int64     `json:"total_errors"`
	Timestamp        time.Time `json:"timestamp"`
}

// RouteStatus holds per-route monitoring data
type RouteStatus struct {
	Domain          string        `json:"domain"`
	Path            string        `json:"path"`
	Backend         string        `json:"backend"`
	Status          string        `json:"status"` // healthy, degraded, down
	Requests24h     int64         `json:"requests_24h"`
	AvgResponseTime time.Duration `json:"avg_response_time"`
	ErrorRate       float64       `json:"error_rate"`
	LastError       string        `json:"last_error,omitempty"`
}

// CertStatus holds certificate expiry information
type CertStatus struct {
	Domain    string    `json:"domain"`
	Issuer    string    `json:"issuer"`
	ExpiresAt time.Time `json:"expires_at"`
	DaysLeft  int       `json:"days_left"`
	Status    string    `json:"status"`   // ok, warning, critical
	Severity  int       `json:"severity"` // 0=ok, 1=warning, 2=critical
}

// ErrorLog represents a recent error entry
type ErrorLog struct {
	Timestamp  time.Time `json:"timestamp"`
	StatusCode int       `json:"status_code"`
	Domain     string    `json:"domain"`
	Path       string    `json:"path"`
	Error      string    `json:"error"`
	RequestID  string    `json:"request_id"`
}

// DashboardData holds all dashboard information
type DashboardData struct {
	SystemStats  *SystemStats  `json:"system_stats"`
	Routes       []RouteStatus `json:"routes"`
	Certificates []CertStatus  `json:"certificates"`
	RecentErrors []ErrorLog    `json:"recent_errors"`
	GeneratedAt  time.Time     `json:"generated_at"`
}

// Dashboard manages the web dashboard
type Dashboard struct {
	mu              sync.RWMutex
	startTime       time.Time
	metricsProvider interface{} // metrics.Collector
	certMonitor     interface{} // certmonitor.Monitor
	proxyServer     interface{} // proxy.Server
	database        interface{} // database.Database
	enabled         bool
}

// New creates a new Dashboard instance
func New(metricsProvider, certMonitor, proxyServer, database interface{}, enabled bool) *Dashboard {
	return &Dashboard{
		startTime:       time.Now(),
		metricsProvider: metricsProvider,
		certMonitor:     certMonitor,
		proxyServer:     proxyServer,
		database:        database,
		enabled:         enabled,
	}
}

// Start registers HTTP handlers for the dashboard
func (d *Dashboard) Start(ctx context.Context, mux *http.ServeMux) error {
	if !d.enabled {
		log.Info().Msg("Dashboard disabled")
		return nil
	}

	// Register dashboard endpoints
	mux.HandleFunc("/dashboard", d.handleDashboard)
	mux.HandleFunc("/api/dashboard", d.handleDashboardAPI)
	mux.HandleFunc("/api/dashboard/stats", d.handleStats)
	mux.HandleFunc("/api/dashboard/routes", d.handleRoutes)
	mux.HandleFunc("/api/dashboard/certificates", d.handleCertificates)
	mux.HandleFunc("/api/dashboard/errors", d.handleErrors)
	mux.HandleFunc("/api/dashboard/context", d.handleAIContext)

	log.Info().Msg("Dashboard enabled at /dashboard")
	return nil
}

// handleDashboard serves the HTML dashboard
func (d *Dashboard) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	fmt.Fprint(w, d.getHTML())
}

// handleDashboardAPI returns all dashboard data as JSON
func (d *Dashboard) handleDashboardAPI(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	data, err := d.gatherDashboardData(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// handleStats returns system statistics
func (d *Dashboard) handleStats(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := d.getSystemStats()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleRoutes returns route status information
func (d *Dashboard) handleRoutes(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	routes := d.getRouteStatuses()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(routes)
}

// handleCertificates returns certificate status
func (d *Dashboard) handleCertificates(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	certs := d.getCertificateStatuses()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(certs)
}

// handleErrors returns recent error logs
func (d *Dashboard) handleErrors(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	errors := d.getRecentErrors()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(errors)
}

// handleAIContext returns AI-ready context export
func (d *Dashboard) handleAIContext(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	context := d.generateAIContext()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, context)
}

// gatherDashboardData collects all dashboard information
func (d *Dashboard) gatherDashboardData(ctx context.Context) (*DashboardData, error) {
	return &DashboardData{
		SystemStats:  d.getSystemStats(),
		Routes:       d.getRouteStatuses(),
		Certificates: d.getCertificateStatuses(),
		RecentErrors: d.getRecentErrors(),
		GeneratedAt:  time.Now(),
	}, nil
}

// getSystemStats calculates current system statistics
func (d *Dashboard) getSystemStats() *SystemStats {
	return &SystemStats{
		UptimeMs:         time.Since(d.startTime).Milliseconds(),
		ActiveConnection: 0,   // TODO: get from metricsProvider
		RequestsPerSec:   0.0, // TODO: calculate from metrics
		ErrorRate:        0.0, // TODO: calculate from metrics
		TotalRequests:    0,   // TODO: get from metricsProvider
		TotalErrors:      0,   // TODO: get from metricsProvider
		Timestamp:        time.Now(),
	}
}

// getRouteStatuses returns status for all configured routes
func (d *Dashboard) getRouteStatuses() []RouteStatus {
	// TODO: fetch from proxyServer routes
	return []RouteStatus{}
}

// getCertificateStatuses returns certificate expiry information
func (d *Dashboard) getCertificateStatuses() []CertStatus {
	// TODO: fetch from certMonitor
	return []CertStatus{}
}

// getRecentErrors returns the last N errors from the database
func (d *Dashboard) getRecentErrors() []ErrorLog {
	// TODO: fetch from database
	return []ErrorLog{}
}

// generateAIContext creates an AI-ready export of current state
func (d *Dashboard) generateAIContext() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	data, _ := d.gatherDashboardData(context.Background())

	ctx := "# Proxy Manager State Export\n\n"
	ctx += fmt.Sprintf("**Exported at**: %s\n\n", time.Now().Format(time.RFC3339))

	ctx += "## System Statistics\n"
	if data.SystemStats != nil {
		ctx += fmt.Sprintf("- Uptime: %v\n", time.Duration(data.SystemStats.UptimeMs)*time.Millisecond)
		ctx += fmt.Sprintf("- Active Connections: %d\n", data.SystemStats.ActiveConnection)
		ctx += fmt.Sprintf("- Requests/sec: %.2f\n", data.SystemStats.RequestsPerSec)
		ctx += fmt.Sprintf("- Error Rate: %.2f%%\n", data.SystemStats.ErrorRate*100)
	}

	ctx += "\n## Routes\n"
	for _, route := range data.Routes {
		ctx += fmt.Sprintf("- %s%s → %s\n", route.Domain, route.Path, route.Backend)
		ctx += fmt.Sprintf("  - Status: %s\n", route.Status)
		ctx += fmt.Sprintf("  - Requests (24h): %d\n", route.Requests24h)
		ctx += fmt.Sprintf("  - Avg Response Time: %v\n", route.AvgResponseTime)
		ctx += fmt.Sprintf("  - Error Rate: %.2f%%\n", route.ErrorRate*100)
	}

	ctx += "\n## Certificates\n"
	for _, cert := range data.Certificates {
		ctx += fmt.Sprintf("- %s: %d days remaining (expires %s)\n", cert.Domain, cert.DaysLeft, cert.ExpiresAt.Format("2006-01-02"))
	}

	ctx += "\n## Recent Errors\n"
	for _, err := range data.RecentErrors {
		ctx += fmt.Sprintf("- [%s] %d - %s%s - %s (req: %s)\n",
			err.Timestamp.Format("15:04:05"), err.StatusCode, err.Domain, err.Path, err.Error, err.RequestID)
	}

	return ctx
}

// IsEnabled returns whether the dashboard is enabled
func (d *Dashboard) IsEnabled() bool {
	return d.enabled
}
