package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/chilla55/proxy-manager/accesslog"
	"github.com/chilla55/proxy-manager/analytics"
	"github.com/chilla55/proxy-manager/certmonitor"
	"github.com/chilla55/proxy-manager/config"
	"github.com/chilla55/proxy-manager/dashboard"
	"github.com/chilla55/proxy-manager/database"
	"github.com/chilla55/proxy-manager/health"
	"github.com/chilla55/proxy-manager/metrics"
	"github.com/chilla55/proxy-manager/proxy"
	"github.com/chilla55/proxy-manager/registry"
	"github.com/chilla55/proxy-manager/traffic"
	"github.com/chilla55/proxy-manager/watcher"
	"github.com/chilla55/proxy-manager/webhook"
	"gopkg.in/yaml.v3"
)

var (
	sitesPath        = flag.String("sites-path", getEnv("SITES_PATH", "/etc/proxy/sites-available"), "Path to site YAML configs")
	globalConfig     = flag.String("global-config", getEnv("GLOBAL_CONFIG", "/etc/proxy/global.yaml"), "Path to global config")
	httpAddr         = flag.String("http-addr", getEnv("HTTP_ADDR", ":80"), "HTTP listen address")
	httpsAddr        = flag.String("https-addr", getEnv("HTTPS_ADDR", ":443"), "HTTPS listen address")
	registryPort     = flag.Int("registry-port", getIntEnv("REGISTRY_PORT", 81), "Service registry port")
	healthPort       = flag.Int("health-port", getIntEnv("HEALTH_PORT", 8080), "Health check HTTP port")
	upstreamTimeout  = flag.Duration("upstream-timeout", getDurationEnv("UPSTREAM_CHECK_TIMEOUT", 2*time.Second), "Upstream check timeout")
	shutdownTimeout  = flag.Duration("shutdown-timeout", getDurationEnv("SHUTDOWN_TIMEOUT", 30*time.Second), "Graceful shutdown timeout")
	debug            = flag.Bool("debug", getEnv("DEBUG", "0") == "1", "Enable debug logging")
	dashboardEnabled = flag.Bool("dashboard-enabled", getEnv("DASHBOARD_ENABLED", "1") == "1", "Enable admin dashboard endpoints")
	dbPath           = flag.String("db-path", getEnv("DB_PATH", "/data/proxy.db"), "Path to SQLite database")
)

func main() {
	flag.Parse()

	// Setup structured logging
	setupLogging()

	log.Info().Msg("Starting unified reverse proxy service")
	log.Info().Str("sites_path", *sitesPath).Msg("Configuration")
	log.Info().Str("global_config", *globalConfig).Msg("Configuration")
	log.Info().Int("registry_port", *registryPort).Msg("Configuration")
	log.Info().Str("db_path", *dbPath).Msg("Configuration")

	// Validate configuration before starting
	if err := validateConfiguration(); err != nil {
		log.Fatal().Err(err).Msg("Configuration validation failed")
	}

	// Validate configuration before starting
	if err := validateConfiguration(); err != nil {
		log.Fatal().Err(err).Msg("Configuration validation failed")
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load global configuration
	globalCfg, err := config.LoadGlobalConfig(*globalConfig)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load global config, using defaults")
		globalCfg = getDefaultGlobalConfig()
	}

	// Load TLS certificates
	certificates, err := loadCertificates(globalCfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load TLS certificates")
	}

	if len(certificates) == 0 {
		log.Fatal().Msg("No TLS certificates configured. Please add certificates to global.yaml")
	}

	log.Info().Int("count", len(certificates)).Msg("Loaded TLS certificates")

	// Initialize database
	db, err := database.Open(*dbPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to open database")
	}
	defer db.Close()

	// Initialize Phase 2 monitoring systems
	metricsCollector := metrics.NewCollector()
	accessLogger := accesslog.NewLogger(db, 1000) // 1000-entry ring buffer
	certMonitor := certmonitor.NewMonitor()
	healthChecker := health.NewChecker(db)
	analyticsAggregator := analytics.NewAggregator(1000, 10*time.Second) // 1000 samples, 10s period
	trafficAnalyzer := traffic.NewAnalyzer(1 * time.Hour)                // 1 hour window

	// Initialize webhook notifier from global config (if present)
	notifier := initWebhookNotifier(*globalConfig)

	// Add certificates to certificate monitor
	for i, certMapping := range certificates {
		for _, domain := range certMapping.Domains {
			if err := certMonitor.AddCertificateFromTLS(domain, &certificates[i].Cert); err != nil {
				log.Warn().Err(err).Str("domain", domain).Msg("Failed to add certificate to monitor")
			} else {
				log.Info().Str("domain", domain).Msg("Added certificate to monitoring")
			}
		}
	}

	// Start periodic certificate expiry checks (every 6 hours)
	certMonitor.StartPeriodicCheck(6 * time.Hour)

	// Start alert monitors (Phase 3 Task #19)
	go monitorHealthAlerts(ctx, healthChecker, notifier)
	go monitorCertAlerts(ctx, certMonitor, notifier)
	go monitorErrorRateAlerts(ctx, metricsCollector, notifier)

	// Start daily cleanup job
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Info().Msg("Running daily database cleanup")
				if err := db.CleanupOldData(30); err != nil {
					log.Error().Err(err).Msg("Database cleanup failed")
				}
			}
		}
	}()

	// Initialize proxy server
	proxyServer := proxy.NewServer(proxy.Config{
		HTTPAddr:         *httpAddr,
		HTTPSAddr:        *httpsAddr,
		Certificates:     certificates,
		GlobalHeaders:    buildSecurityHeaders(globalCfg),
		BlackholeUnknown: globalCfg.Blackhole.UnknownDomains,
		Debug:            *debug,
		DB:               db,
		MetricsCollector: metricsCollector,
		AccessLogger:     accessLogger,
		CertMonitor:      certMonitor,
		HealthChecker:    healthChecker,
		Notifier:         notifier,
	})

	// Initialize service registry
	reg := registry.NewRegistry(*registryPort, *upstreamTimeout, proxyServer, *debug)

	// Initialize site watcher
	siteWatcher := watcher.NewSiteWatcher(*sitesPath, proxyServer, *debug)

	// Initialize certificate watcher
	certWatcher := watcher.NewCertWatcher(*globalConfig, proxyServer, *debug)

	// Start health check server (includes dashboard when enabled)
	go startHealthServer(ctx, *healthPort, proxyServer, metricsCollector, accessLogger, certMonitor, healthChecker, analyticsAggregator, trafficAnalyzer, db, *dashboardEnabled)

	// Start site watcher
	go siteWatcher.Start(ctx)

	// Start certificate watcher (monitors for cert renewals)
	go func() {
		if err := certWatcher.Start(ctx); err != nil {
			log.Printf("[proxy-manager] Certificate watcher error: %s", err)
		}
	}()

	// Start service registry
	go reg.Start(ctx)

	// Start proxy servers (HTTP, HTTPS, HTTP/3)
	go func() {
		if err := proxyServer.Start(ctx, *httpAddr, *httpsAddr); err != nil {
			log.Error().Err(err).Msg("Proxy server error")
		}
	}()

	log.Info().Msg("All services started successfully")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	log.Info().Str("signal", sig.String()).Msg("Shutdown signal received")

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), *shutdownTimeout)
	defer shutdownCancel()

	// Notify all connected services
	log.Info().Msg("Notifying connected services...")
	reg.NotifyShutdown()

	// Give services time to receive shutdown notification
	time.Sleep(2 * time.Second)

	// Cancel main context to stop all goroutines
	cancel()

	// Wait for graceful shutdown or timeout
	done := make(chan struct{})
	go func() {
		// Wait for all background goroutines
		time.Sleep(1 * time.Second)
		close(done)
	}()

	select {
	case <-done:
		log.Info().Msg("Shutdown complete")
	case <-shutdownCtx.Done():
		log.Warn().Msg("Shutdown timeout exceeded, forcing exit")
	}
}

func startHealthServer(ctx context.Context, port int, proxyServer *proxy.Server, metricsCollector *metrics.Collector, accessLogger *accesslog.Logger, certMonitor *certmonitor.Monitor, healthChecker *health.Checker, analyticsAggregator *analytics.Aggregator, trafficAnalyzer *traffic.Analyzer, dbConn *database.Database, dashboardEnabled bool) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy"))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(metricsCollector.PrometheusMetrics()))
	})

	mux.HandleFunc("/api/logs/recent", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		entries := accessLogger.GetRecentRequests(100)
		fmt.Fprintf(w, "%v", entries)
	})

	mux.HandleFunc("/api/logs/errors", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		entries := accessLogger.GetRecentErrors(50)
		fmt.Fprintf(w, "%v", entries)
	})

	mux.HandleFunc("/api/logs/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stats := accessLogger.GetStats()
		fmt.Fprintf(w, "%v", stats)
	})

	mux.HandleFunc("/api/certs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		certs := certMonitor.GetAllCertificates()
		fmt.Fprintf(w, "%v", certs)
	})

	mux.HandleFunc("/api/certs/expiring", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		level := r.URL.Query().Get("level")
		if level == "" {
			level = certmonitor.LevelWarning
		}
		certs := certMonitor.GetExpiringCertificates(level)
		fmt.Fprintf(w, "%v", certs)
	})

	mux.HandleFunc("/api/certs/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stats := certMonitor.GetStats()
		fmt.Fprintf(w, "%v", stats)
	})

	mux.HandleFunc("/api/health/services", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		statuses := healthChecker.GetAllStatuses()
		fmt.Fprintf(w, "%v", statuses)
	})

	mux.HandleFunc("/api/health/unhealthy", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		unhealthy := healthChecker.GetUnhealthyServices()
		fmt.Fprintf(w, "%v", unhealthy)
	})

	mux.HandleFunc("/api/analytics/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		aggregated := analyticsAggregator.GetAggregatedMetrics()
		fmt.Fprintf(w, "%v", aggregated)
	})

	mux.HandleFunc("/api/traffic/analysis", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		topN := 10
		if n := r.URL.Query().Get("top"); n != "" {
			if parsed, err := fmt.Sscanf(n, "%d", &topN); err == nil && parsed == 1 {
			}
		}
		analysis := trafficAnalyzer.Analyze(topN)
		fmt.Fprintf(w, "%v", analysis)
	})

	mux.HandleFunc("/api/traffic/ip", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		if ip == "" {
			http.Error(w, "Missing ip parameter", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		reputation := trafficAnalyzer.GetIPReputation(ip)
		fmt.Fprintf(w, `{"ip":"%s","reputation_score":%.2f}`, ip, reputation)
	})

	mux.HandleFunc("/api/traffic/anomalies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		analysis := trafficAnalyzer.Analyze(10)
		fmt.Fprintf(w, "%v", analysis.AnomalousPatterns)
	})

	mux.HandleFunc("/api/blackhole", func(w http.ResponseWriter, r *http.Request) {
		blackholeCount := proxyServer.GetBlackholeCount()
		fmt.Fprintf(w, "# HELP blackhole_requests_total Total number of blackholed requests\n")
		fmt.Fprintf(w, "# TYPE blackhole_requests_total counter\n")
		fmt.Fprintf(w, "blackhole_requests_total %d\n", blackholeCount)
	})

	mux.HandleFunc("/api/analytics/top-routes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		period := r.URL.Query().Get("period")
		window := parsePeriod(period)
		limit := 10
		if l := r.URL.Query().Get("limit"); l != "" {
			_, _ = fmt.Sscanf(l, "%d", &limit)
		}

		entries := accessLogger.GetRecentRequests(1000)
		cutoff := time.Now().Add(-window).Unix()
		counts := map[string]int{}
		for _, e := range entries {
			if e.Timestamp < cutoff {
				continue
			}
			counts[e.Domain]++
		}

		type item struct {
			Route string `json:"route"`
			Count int    `json:"count"`
		}
		var list []item
		for route, count := range counts {
			list = append(list, item{Route: route, Count: count})
		}
		for i := 0; i < len(list); i++ {
			for j := i + 1; j < len(list); j++ {
				if list[j].Count > list[i].Count {
					list[i], list[j] = list[j], list[i]
				}
			}
		}
		if limit > 0 && limit < len(list) {
			list = list[:limit]
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"routes": list,
			"period": period,
		})
	})

	mux.HandleFunc("/api/analytics/heatmap", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		period := r.URL.Query().Get("period")
		window := parsePeriod(period)
		entries := accessLogger.GetRecentRequests(1000)
		cutoff := time.Now().Add(-window).Unix()

		buckets := make([]int, 24)
		for _, e := range entries {
			if e.Timestamp < cutoff {
				continue
			}
			hour := time.Unix(e.Timestamp, 0).UTC().Hour()
			buckets[hour]++
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"hours":  buckets,
			"period": period,
		})
	})

	mux.HandleFunc("/api/analytics/bandwidth", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		route := r.URL.Query().Get("route")
		period := r.URL.Query().Get("period")
		window := parsePeriod(period)

		entries := accessLogger.GetRecentRequests(1000)
		cutoff := time.Now().Add(-window).Unix()
		var sent, recv uint64
		for _, e := range entries {
			if e.Timestamp < cutoff {
				continue
			}
			if route == "" || e.Domain == route || e.Path == route {
				sent += e.BytesSent
				recv += e.BytesReceived
			}
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"route":            route,
			"period":           period,
			"bytes_sent_total": sent,
			"bytes_recv_total": recv,
		})
	})

	mux.HandleFunc("/api/ai-context", func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		if format == "" {
			format = "markdown"
		}

		if format != "markdown" {
			http.Error(w, "unsupported format", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")

		stats := metricsCollector.GetStats()
		certStats := certMonitor.GetStats()
		recentErrors := accessLogger.GetRecentErrors(10)

		var buf bytes.Buffer
		fmt.Fprintf(&buf, "# Proxy Status Report - %s\n\n", time.Now().UTC().Format("2006-01-02 15:04:05 MST"))
		fmt.Fprintf(&buf, "## System Overview\n")
		fmt.Fprintf(&buf, "- Uptime: %.0fs\n", stats.Uptime)
		fmt.Fprintf(&buf, "- Active Connections: %d\n", stats.ActiveConnections)
		fmt.Fprintf(&buf, "- Error Rate (%%): %.2f\n\n", stats.ErrorRate)
		fmt.Fprintf(&buf, "## Certificate Status\n")
		fmt.Fprintf(&buf, "- Total: %d | Healthy: %d | Warning: %d | Urgent: %d | Critical: %d | Expired: %d\n\n",
			certStats.TotalCertificates,
			certStats.HealthyCount,
			certStats.WarningCount,
			certStats.UrgentCount,
			certStats.CriticalCount,
			certStats.ExpiredCount,
		)
		fmt.Fprintf(&buf, "## Recent Errors (Last %d)\n", len(recentErrors))
		for i, e := range recentErrors {
			fmt.Fprintf(&buf, "%d. [%s] %d %s - %s %s\n",
				i+1,
				time.Unix(e.Timestamp, 0).UTC().Format("15:04:05"),
				e.Status,
				e.Domain,
				e.Method,
				e.Path,
			)
		}
		fmt.Fprintf(&buf, "\n## Suggested Analysis Questions:\n")
		fmt.Fprintf(&buf, "- Are there patterns in error spikes?\n")
		fmt.Fprintf(&buf, "- Which routes have highest latency?\n")
		fmt.Fprintf(&buf, "- Any certificates nearing expiry that need action?\n")
		w.Write(buf.Bytes())
	})

	dash := dashboard.New(metricsCollector, certMonitor, proxyServer, dbConn, dashboardEnabled)
	if err := dash.Start(ctx, mux); err != nil {
		log.Error().Err(err).Msg("Failed to start dashboard")
	}

	addr := fmt.Sprintf(":%d", port)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		log.Info().Str("addr", addr).Msg("Shutting down health server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Health server shutdown failed")
		}
	}()

	log.Info().Str("addr", addr).Msg("Health check server starting")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error().Err(err).Msg("Health server error")
	}
}

// parsePeriod parses strings like "24h", "7d" into a duration.
// Defaults to 24h when input is empty or invalid.
func parsePeriod(s string) time.Duration {
	if s == "" {
		return 24 * time.Hour
	}
	// support 'd' suffix
	if strings.HasSuffix(s, "d") {
		var days int
		if _, err := fmt.Sscanf(s, "%dd", &days); err == nil && days > 0 {
			return time.Duration(days) * 24 * time.Hour
		}
		return 24 * time.Hour
	}
	// fallback to time.ParseDuration for h,m,s
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return 24 * time.Hour
}

// initWebhookNotifier loads webhook configuration from the global YAML
func initWebhookNotifier(globalConfigPath string) *webhook.Notifier {
	// Minimal loader that looks for a top-level 'webhooks' and optional 'enabled'
	type raw struct {
		Webhooks []webhook.Webhook `yaml:"webhooks"`
		Enabled  *bool             `yaml:"webhooks_enabled"`
	}
	data, err := os.ReadFile(globalConfigPath)
	if err != nil {
		// Fallback: disabled notifier
		return webhook.New(webhook.Config{Enabled: false})
	}
	var r raw
	if err := yaml.Unmarshal(data, &r); err != nil {
		return webhook.New(webhook.Config{Enabled: false})
	}
	enabled := true
	if r.Enabled != nil {
		enabled = *r.Enabled
	}
	return webhook.New(webhook.Config{Enabled: enabled, Webhooks: r.Webhooks})
}

// monitorHealthAlerts sends alerts on service_down transitions
func monitorHealthAlerts(ctx context.Context, checker *health.Checker, notifier *webhook.Notifier) {
	prev := map[string]string{}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			statuses := checker.GetAllStatuses()
			for svc, st := range statuses {
				if prev[svc] != st.Status && st.Status == string(health.StatusDown) {
					_ = notifier.Send(webhook.Alert{
						Event:       webhook.EventServiceDown,
						Title:       "🚨 Service Down Alert",
						Description: fmt.Sprintf("%s is not responding", svc),
						Severity:    "critical",
						Fields: map[string]string{
							"Service":     svc,
							"Last Error":  st.LastError,
							"SuccessRate": fmt.Sprintf("%.1f%%", st.SuccessRate),
						},
						Timestamp: time.Now(),
					})
				}
				prev[svc] = st.Status
			}
		}
	}
}

// monitorCertAlerts sends alerts for certificates expiring soon (7/14/30 days)
func monitorCertAlerts(ctx context.Context, cm *certmonitor.Monitor, notifier *webhook.Notifier) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 7 days
			for _, info := range cm.GetExpiringCertificates(certmonitor.LevelCritical) {
				_ = notifier.Send(webhook.Alert{
					Event:       webhook.EventCertExpiring7d,
					Title:       "⚠️ Certificate Expiring <= 7d",
					Description: fmt.Sprintf("%s expires in %d days", info.Domain, info.DaysRemaining),
					Severity:    "warning",
					Fields: map[string]string{
						"Domain":         info.Domain,
						"Days Remaining": fmt.Sprintf("%d", info.DaysRemaining),
						"Expiry":         info.NotAfter.UTC().Format(time.RFC3339),
					},
					Timestamp: time.Now(),
				})
			}
			// 14 days
			for _, info := range cm.GetExpiringCertificates(certmonitor.LevelUrgent) {
				_ = notifier.Send(webhook.Alert{
					Event:       webhook.EventCertExpiring14d,
					Title:       "⚠️ Certificate Expiring <= 14d",
					Description: fmt.Sprintf("%s expires in %d days", info.Domain, info.DaysRemaining),
					Severity:    "warning",
					Fields: map[string]string{
						"Domain":         info.Domain,
						"Days Remaining": fmt.Sprintf("%d", info.DaysRemaining),
						"Expiry":         info.NotAfter.UTC().Format(time.RFC3339),
					},
					Timestamp: time.Now(),
				})
			}
			// 30 days
			for _, info := range cm.GetExpiringCertificates(certmonitor.LevelWarning) {
				_ = notifier.Send(webhook.Alert{
					Event:       webhook.EventCertExpiring30d,
					Title:       "⚠️ Certificate Expiring <= 30d",
					Description: fmt.Sprintf("%s expires in %d days", info.Domain, info.DaysRemaining),
					Severity:    "info",
					Fields: map[string]string{
						"Domain":         info.Domain,
						"Days Remaining": fmt.Sprintf("%d", info.DaysRemaining),
						"Expiry":         info.NotAfter.UTC().Format(time.RFC3339),
					},
					Timestamp: time.Now(),
				})
			}
		}
	}
}

// monitorErrorRateAlerts sends alerts when error rate spikes above threshold
func monitorErrorRateAlerts(ctx context.Context, mc *metrics.Collector, notifier *webhook.Notifier) {
	const threshold = 5.0 // percent
	prevHigh := false
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats := mc.GetStats()
			high := stats.ErrorRate > threshold
			if high && !prevHigh {
				_ = notifier.Send(webhook.Alert{
					Event:       webhook.EventHighErrorRate,
					Title:       "⚠️ High Error Rate",
					Description: fmt.Sprintf("Error rate is %.2f%% (threshold %.2f%%)", stats.ErrorRate, threshold),
					Severity:    "warning",
					Fields: map[string]string{
						"Total Requests": fmt.Sprintf("%d", stats.TotalRequests),
						"Total Errors":   fmt.Sprintf("%d", stats.TotalErrors),
					},
					Timestamp: time.Now(),
				})
			}
			prevHigh = high
		}
	}
}

// setupLogging configures zerolog based on environment
func setupLogging() {
	// Set log level from environment
	logLevel := getEnv("LOG_LEVEL", "info")
	switch logLevel {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Set log format from environment
	logFormat := getEnv("LOG_FORMAT", "json")
	if logFormat == "console" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	}

	// Add caller information in debug mode
	if logLevel == "debug" {
		log.Logger = log.With().Caller().Logger()
	}
}

// validateConfiguration checks that all required directories and files exist
func validateConfiguration() error {
	log.Info().Msg("Validating configuration...")

	// Check required directories exist
	dirs := []string{*sitesPath}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return fmt.Errorf("directory does not exist: %s", dir)
		}
	}

	// Check global config file exists
	if _, err := os.Stat(*globalConfig); os.IsNotExist(err) {
		return fmt.Errorf("global config file does not exist: %s", *globalConfig)
	}

	// Ensure database directory exists
	dbDir := filepath.Dir(*dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return fmt.Errorf("failed to create database directory: %w", err)
	}

	// Validate ports are not the same
	if *httpAddr == *httpsAddr {
		return fmt.Errorf("HTTP and HTTPS addresses cannot be the same")
	}

	// Check shutdown timeout is reasonable
	if *shutdownTimeout < 1*time.Second || *shutdownTimeout > 5*time.Minute {
		return fmt.Errorf("shutdown timeout must be between 1s and 5m, got %v", *shutdownTimeout)
	}

	log.Info().Msg("Configuration validation passed")
	return nil
}

func buildSecurityHeaders(cfg *config.GlobalConfig) proxy.SecurityHeaders {
	headers := proxy.SecurityHeaders{}

	if cfg.Defaults.Headers != nil {
		headers.HSTS = cfg.Defaults.Headers["Strict-Transport-Security"]
		headers.XFrameOptions = cfg.Defaults.Headers["X-Frame-Options"]
		headers.XContentType = cfg.Defaults.Headers["X-Content-Type-Options"]
		headers.XSSProtection = cfg.Defaults.Headers["X-XSS-Protection"]
		headers.CSP = cfg.Defaults.Headers["Content-Security-Policy"]
		headers.ReferrerPolicy = cfg.Defaults.Headers["Referrer-Policy"]
		headers.PermissionsPolicy = cfg.Defaults.Headers["Permissions-Policy"]
	}

	return headers
}

// loadCertificates loads TLS certificates from global config
func loadCertificates(cfg *config.GlobalConfig) ([]proxy.CertMapping, error) {
	if len(cfg.TLS.Certificates) == 0 {
		return nil, fmt.Errorf("no certificates defined in global config")
	}

	certificates := make([]proxy.CertMapping, 0, len(cfg.TLS.Certificates))

	for i, certCfg := range cfg.TLS.Certificates {
		// Load certificate and private key
		cert, err := tls.LoadX509KeyPair(certCfg.CertFile, certCfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load certificate %d (%s): %w", i+1, certCfg.CertFile, err)
		}

		if len(certCfg.Domains) == 0 {
			return nil, fmt.Errorf("certificate %d has no domains defined", i+1)
		}

		mapping := proxy.CertMapping{
			Domains: certCfg.Domains,
			Cert:    cert,
		}

		certificates = append(certificates, mapping)
		log.Info().Strs("domains", certCfg.Domains).Msg("Loaded certificate")
	}

	return certificates, nil
}

func getDefaultGlobalConfig() *config.GlobalConfig {
	cfg := &config.GlobalConfig{}

	// Set sensible defaults
	cfg.Defaults.Headers = map[string]string{
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
		"X-Frame-Options":           "DENY",
		"X-Content-Type-Options":    "nosniff",
		"X-XSS-Protection":          "1; mode=block",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
	}

	cfg.Blackhole.UnknownDomains = true
	cfg.Blackhole.MetricsOnly = true

	// No default TLS config - certificates must be explicitly provided
	cfg.TLS.Certificates = []config.CertConfig{}

	return cfg
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intVal int
		if _, err := fmt.Sscanf(value, "%d", &intVal); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
