package proxy

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/chilla55/proxy-manager/database"
	"github.com/chilla55/proxy-manager/metrics"
	"github.com/chilla55/proxy-manager/tracing"
	"github.com/chilla55/proxy-manager/webhook"
	"github.com/quic-go/quic-go/http3"
	"github.com/rs/zerolog/log"
)

// SecurityHeaders defines HTTP security headers
type SecurityHeaders struct {
	HSTS              string
	XFrameOptions     string
	XContentType      string
	XSSProtection     string
	CSP               string
	ReferrerPolicy    string
	PermissionsPolicy string
}

// Backend represents a proxy target
type Backend struct {
	URL           *url.URL
	Proxy         *httputil.ReverseProxy
	Healthy       bool
	HealthPath    string
	HealthTimeout time.Duration
	Timeout       time.Duration
	MaxBodySize   int64
	WebSocket     bool
	mu            sync.RWMutex
	// Phase 4 configs
	slowEnabled        bool
	slowWarning        time.Duration
	slowCritical       time.Duration
	slowTimeout        time.Duration
	alertWebhook       bool
	retryEnabled       bool
	retryMax           int
	retryBackoff       string
	retryInitial       time.Duration
	retryMaxDelay      time.Duration
	retryOn            map[string]struct{}
	compressionEnabled bool
	compressionAlgos   []string
	compressionLevel   int
	compressionMinSize int64
	compressionTypes   map[string]struct{}
	websocketEnabled   bool
	websocketMaxConn   int
	websocketMaxDur    time.Duration
	websocketIdle      time.Duration
	websocketPing      time.Duration
	websocketActive    int64
	metrics            *metrics.Collector
	// Circuit breaker (Phase 6)
	cbEnabled          bool
	cbFailureThreshold int
	cbSuccessThreshold int
	cbTimeout          time.Duration
	cbWindow           time.Duration
	cbState            string // "closed", "open", "half-open"
	cbFailures         int
	cbSuccesses        int
	cbOpenedAt         time.Time
	cbLastFailure      time.Time
}

// Route represents a routing rule
type Route struct {
	Domains   []string
	Path      string
	Backend   *Backend
	Headers   map[string]string
	WebSocket bool
	Priority  int // For sorting (longer paths = higher priority)
}

// CertMapping maps domain patterns to certificates
type CertMapping struct {
	Domains []string // ["*.example.com", "example.com"]
	Cert    tls.Certificate
}

// Server is the main reverse proxy server
type Server struct {
	mu              sync.RWMutex
	routes          []*Route
	routeMap        map[string]*Backend // domain+path -> backend
	globalHeaders   SecurityHeaders
	blackholeMetric int64

	httpServer   *http.Server
	httpsServer  *http.Server
	http3Server  *http3.Server
	certificates []CertMapping // Loaded TLS certificates

	db               interface{} // Database connection (interface to avoid import cycle)
	metricsCollector interface{} // Metrics collector (interface to avoid import cycle)
	accessLogger     interface{} // Access logger (interface to avoid import cycle)
	certMonitor      interface{} // Certificate monitor (interface to avoid import cycle)
	healthChecker    interface{} // Health checker (interface to avoid import cycle)
	notifier         interface{} // Webhook notifier (optional)
	debug            bool
}

// Config holds server configuration
type Config struct {
	HTTPAddr         string
	HTTPSAddr        string
	Certificates     []CertMapping
	GlobalHeaders    SecurityHeaders
	BlackholeUnknown bool
	Debug            bool
	DB               interface{} // Database connection
	MetricsCollector interface{} // Metrics collector
	AccessLogger     interface{} // Access logger
	CertMonitor      interface{} // Certificate monitor
	HealthChecker    interface{} // Health checker
	Notifier         interface{} // Webhook notifier
}

// NewServer creates a new proxy server
func NewServer(cfg Config) *Server {
	s := &Server{
		routes:           make([]*Route, 0),
		routeMap:         make(map[string]*Backend),
		globalHeaders:    cfg.GlobalHeaders,
		certificates:     cfg.Certificates,
		db:               cfg.DB,
		metricsCollector: cfg.MetricsCollector,
		accessLogger:     cfg.AccessLogger,
		certMonitor:      cfg.CertMonitor,
		healthChecker:    cfg.HealthChecker,
		notifier:         cfg.Notifier,
		debug:            cfg.Debug,
	}

	return s
}

// Start starts all HTTP servers (HTTP, HTTPS, HTTP/3)
func (s *Server) Start(ctx context.Context, httpAddr, httpsAddr string) error {
	// HTTP server (redirects to HTTPS)
	s.httpServer = &http.Server{
		Addr:    httpAddr,
		Handler: http.HandlerFunc(s.redirectToHTTPS),
	}

	// HTTPS server (HTTP/1.1 and HTTP/2)
	s.httpsServer = &http.Server{
		Addr:      httpsAddr,
		Handler:   s,
		TLSConfig: s.tlsConfig(),
	}

	// HTTP/3 server
	s.http3Server = &http3.Server{
		Addr:      httpsAddr,
		Handler:   s,
		TLSConfig: s.tlsConfig(),
	}

	// Start HTTP server
	go func() {
		log.Info().Str("addr", httpAddr).Msg("Starting HTTP server")
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTP server error")
		}
	}()

	// Start HTTPS server
	go func() {
		log.Info().Str("addr", httpsAddr).Msg("Starting HTTPS server (HTTP/2 enabled)")
		if err := s.httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTPS server error")
		}
	}()

	// Start HTTP/3 server
	go func() {
		log.Info().Str("addr", httpsAddr).Msg("Starting HTTP/3 server")
		if err := s.http3Server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("HTTP/3 server error")
		}
	}()

	<-ctx.Done()
	return s.Shutdown(context.Background())
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Find backend for this request
	backend := s.findBackend(r.Host, r.URL.Path)

	if backend == nil {
		// Unknown domain - blackhole
		s.blackhole(w, r)
		return
	}

	// Check if backend is healthy or circuit open
	backend.mu.Lock()
	healthy := backend.Healthy
	cbOpen := false
	if backend.cbEnabled {
		switch backend.cbState {
		case "open":
			if backend.cbTimeout > 0 && time.Since(backend.cbOpenedAt) >= backend.cbTimeout {
				backend.cbState = "half-open"
				backend.cbSuccesses = 0
			} else {
				cbOpen = true
			}
		}
	}
	backend.mu.Unlock()

	if !healthy || cbOpen {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Get route for headers
	route := s.findRoute(r.Host, r.URL.Path)

	// Handle WebSocket upgrade separately
	if isWebSocketRequest(r) {
		if route != nil && route.WebSocket {
			s.handleWebSocket(w, r, route, backend)
		} else {
			http.Error(w, "WebSocket not allowed", http.StatusBadRequest)
		}
		return
	}

	// Apply security headers
	s.applyHeaders(w, route)

	// Proxy request with slow-request tracking
	start := time.Now()
	backend.Proxy.ServeHTTP(w, r)
	elapsed := time.Since(start)
	if backend.slowEnabled {
		if backend.slowCritical > 0 && elapsed >= backend.slowCritical {
			log.Error().Dur("duration", elapsed).Str("host", r.Host).Str("path", r.URL.Path).Msg("Critical slow request")
			s.recordSlowMetric("critical")
			if backend.alertWebhook {
				s.sendSlowAlert(route, r, elapsed, "critical")
			}
		} else if backend.slowWarning > 0 && elapsed >= backend.slowWarning {
			log.Warn().Dur("duration", elapsed).Str("host", r.Host).Str("path", r.URL.Path).Msg("Slow request warning")
			s.recordSlowMetric("warning")
			if backend.alertWebhook {
				s.sendSlowAlert(route, r, elapsed, "warning")
			}
		}
	}
}

// AddRoute adds or updates a route
func (s *Server) AddRoute(domains []string, path, backendURL string, headers map[string]string, websocket bool, options map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Parse backend URL
	target, err := url.Parse(backendURL)
	if err != nil {
		return fmt.Errorf("invalid backend URL: %w", err)
	}

	// Create or find backend
	backend := s.getOrCreateBackend(target, options)

	// Create route
	route := &Route{
		Domains:   domains,
		Path:      path,
		Backend:   backend,
		Headers:   headers,
		WebSocket: websocket,
		Priority:  len(path), // Longer paths = higher priority
	}

	// Add route
	s.routes = append(s.routes, route)
	s.sortRoutes()

	// Update route map for all domains
	for _, domain := range domains {
		key := s.routeKey(domain, path)
		s.routeMap[key] = backend
	}

	if s.debug {
		log.Debug().Strs("domains", domains).Str("path", path).Str("backend", backendURL).Msg("Added route")
	}

	return nil
}

// RemoveRoute removes routes for given domains and path
func (s *Server) RemoveRoute(domains []string, path string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from route map
	for _, domain := range domains {
		key := s.routeKey(domain, path)
		delete(s.routeMap, key)
	}

	// Remove from routes slice
	filtered := make([]*Route, 0, len(s.routes))
	for _, r := range s.routes {
		if !s.routeMatches(r, domains, path) {
			filtered = append(filtered, r)
		}
	}
	s.routes = filtered

	if s.debug {
		log.Debug().Strs("domains", domains).Str("path", path).Msg("Removed route")
	}
}

// GetBlackholeCount returns the number of blackholed requests
func (s *Server) GetBlackholeCount() int64 {
	return atomic.LoadInt64(&s.blackholeMetric)
}

// UpdateCertificates hot-reloads certificates without restarting the server
func (s *Server) UpdateCertificates(certificates []CertMapping) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.certificates = certificates
	log.Info().Int("count", len(certificates)).Msg("Certificates updated")
}

// findBackend finds the best matching backend for a request
func (s *Server) findBackend(host, path string) *Backend {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Try exact match first
	key := s.routeKey(host, path)
	if backend, ok := s.routeMap[key]; ok {
		return backend
	}

	// Try longest prefix match
	var bestMatch *Backend
	longestMatch := 0

	for _, route := range s.routes {
		for _, domain := range route.Domains {
			if domain == host && len(route.Path) <= len(path) {
				if path[:len(route.Path)] == route.Path {
					if len(route.Path) > longestMatch {
						longestMatch = len(route.Path)
						bestMatch = route.Backend
					}
				}
			}
		}
	}

	return bestMatch
}

// findRoute finds the route for header application
func (s *Server) findRoute(host, path string) *Route {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var bestMatch *Route
	longestMatch := 0

	for _, route := range s.routes {
		for _, domain := range route.Domains {
			if domain == host && len(route.Path) <= len(path) {
				if path[:len(route.Path)] == route.Path {
					if len(route.Path) > longestMatch {
						longestMatch = len(route.Path)
						bestMatch = route
					}
				}
			}
		}
	}

	return bestMatch
}

// getOrCreateBackend gets or creates a backend
func (s *Server) getOrCreateBackend(target *url.URL, options map[string]interface{}) *Backend {
	// Check if backend already exists
	for _, route := range s.routes {
		if route.Backend.URL.String() == target.String() {
			return route.Backend
		}
	}

	// Create new backend
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize transport
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	proxy.Transport = transport

	// Customize director
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		// Add X-Forwarded headers
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Real-IP", req.RemoteAddr)
	}

	backend := &Backend{
		URL:                target,
		Proxy:              proxy,
		Healthy:            true,
		HealthPath:         "/",
		HealthTimeout:      5 * time.Second,
		Timeout:            30 * time.Second,
		MaxBodySize:        100 * 1024 * 1024, // 100MB
		compressionAlgos:   []string{"br", "gzip"},
		compressionLevel:   5,
		compressionMinSize: 1024,
		compressionTypes: map[string]struct{}{
			"text/html":              {},
			"text/css":               {},
			"application/javascript": {},
			"application/json":       {},
			"image/svg+xml":          {},
		},
		websocketMaxDur:    24 * time.Hour,
		websocketIdle:      5 * time.Minute,
		websocketPing:      30 * time.Second,
		cbEnabled:          false,
		cbFailureThreshold: 5,
		cbSuccessThreshold: 2,
		cbTimeout:          30 * time.Second,
		cbWindow:           60 * time.Second,
		cbState:            "closed",
	}

	if mc, ok := s.metricsCollector.(*metrics.Collector); ok {
		backend.metrics = mc
	}

	// Apply options
	if options != nil {
		if v, ok := options["health_check_path"].(string); ok {
			backend.HealthPath = v
		}
		if v, ok := options["timeout"].(time.Duration); ok {
			backend.Timeout = v
			transport.ResponseHeaderTimeout = v
		}
		// Connection pool settings
		if pm, ok := options["pool"].(map[string]interface{}); ok {
			if v, ok := pm["max_idle_conns"].(int); ok && v > 0 {
				transport.MaxIdleConns = v
			}
			if v, ok := pm["max_idle_conns_per_host"].(int); ok && v > 0 {
				transport.MaxIdleConnsPerHost = v
			}
			if v, ok := pm["max_conns_per_host"].(int); ok && v > 0 {
				transport.MaxConnsPerHost = v
			}
			if v, ok := pm["idle_timeout"].(time.Duration); ok && v > 0 {
				transport.IdleConnTimeout = v
			}
		}
		// Slow request detection
		if sm, ok := options["slow_request"].(map[string]interface{}); ok {
			if v, ok := sm["enabled"].(bool); ok {
				backend.slowEnabled = v
			}
			if v, ok := sm["warning"].(time.Duration); ok {
				backend.slowWarning = v
			}
			if v, ok := sm["critical"].(time.Duration); ok {
				backend.slowCritical = v
			}
			if v, ok := sm["timeout"].(time.Duration); ok {
				backend.slowTimeout = v
			}
			if v, ok := sm["alert_webhook"].(bool); ok {
				backend.alertWebhook = v
			}
			if backend.slowTimeout > 0 {
				transport.ResponseHeaderTimeout = backend.slowTimeout
			}
		}
		// Retry logic
		if rm, ok := options["retry"].(map[string]interface{}); ok {
			if v, ok := rm["enabled"].(bool); ok {
				backend.retryEnabled = v
			}
			if v, ok := rm["max_attempts"].(int); ok && v > 0 {
				backend.retryMax = v
			}
			if v, ok := rm["backoff"].(string); ok {
				backend.retryBackoff = v
			}
			if v, ok := rm["initial_delay"].(time.Duration); ok {
				backend.retryInitial = v
			}
			if v, ok := rm["max_delay"].(time.Duration); ok {
				backend.retryMaxDelay = v
			}
			if arr, ok := rm["retry_on"].([]string); ok {
				backend.retryOn = make(map[string]struct{}, len(arr))
				for _, s := range arr {
					backend.retryOn[s] = struct{}{}
				}
			}
			if backend.retryEnabled && backend.retryMax > 0 {
				proxy.Transport = newRetryTransport(transport, backend)
			}
		}
		// Compression
		if cm, ok := options["compression"].(map[string]interface{}); ok {
			if v, ok := cm["enabled"].(bool); ok {
				backend.compressionEnabled = v
			}
			if v, ok := cm["level"].(int); ok {
				backend.compressionLevel = v
			}
			if v, ok := cm["min_size"].(int64); ok {
				backend.compressionMinSize = v
			}
			if v, ok := cm["min_size"].(int); ok {
				backend.compressionMinSize = int64(v)
			}
			if arr, ok := cm["algorithms"].([]string); ok && len(arr) > 0 {
				backend.compressionAlgos = normalizeAlgorithms(arr)
			}
			if arr, ok := cm["content_types"].([]string); ok && len(arr) > 0 {
				backend.compressionTypes = make(map[string]struct{}, len(arr))
				for _, ct := range arr {
					backend.compressionTypes[strings.ToLower(ct)] = struct{}{}
				}
			}
		}
		// WebSocket tuning
		if wm, ok := options["websocket"].(map[string]interface{}); ok {
			if v, ok := wm["enabled"].(bool); ok {
				backend.websocketEnabled = v
			}
			if v, ok := wm["max_connections"].(int); ok {
				backend.websocketMaxConn = v
			}
			if v, ok := wm["max_duration"].(time.Duration); ok && v > 0 {
				backend.websocketMaxDur = v
			}
			if v, ok := wm["idle_timeout"].(time.Duration); ok && v > 0 {
				backend.websocketIdle = v
			}
			if v, ok := wm["ping_interval"].(time.Duration); ok && v > 0 {
				backend.websocketPing = v
			}
		}
		// Circuit breaker
		if cbm, ok := options["circuit_breaker"].(map[string]interface{}); ok {
			if v, ok := cbm["enabled"].(bool); ok {
				backend.cbEnabled = v
			}
			if v, ok := cbm["failure_threshold"].(int); ok && v > 0 {
				backend.cbFailureThreshold = v
			}
			if v, ok := cbm["success_threshold"].(int); ok && v > 0 {
				backend.cbSuccessThreshold = v
			}
			if v, ok := cbm["timeout"].(time.Duration); ok && v > 0 {
				backend.cbTimeout = v
			}
			if v, ok := cbm["window"].(time.Duration); ok && v > 0 {
				backend.cbWindow = v
			}
		}
	}

	// Attach response modifiers and error handler
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		// Record circuit breaker failure on transport errors
		backend.cbRecordFailure()
		log.Error().Err(err).Str("host", req.Host).Str("path", req.URL.Path).Msg("Upstream transport error")
		rw.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(rw, "Bad Gateway")
	}
	proxy.ModifyResponse = backend.buildModifyResponse()

	return backend
}

// buildModifyResponse composes response modifiers (compression + circuit breaker updates)
func (b *Backend) buildModifyResponse() func(*http.Response) error {
	compress := b.compressionHandler()
	return func(res *http.Response) error {
		// Update circuit breaker state based on status
		if res != nil {
			code := res.StatusCode
			if code >= 500 {
				b.cbRecordFailure()
			} else if code >= 200 {
				b.cbRecordSuccess()
			}
		}
		// Apply compression if eligible
		return compress(res)
	}
}

// cbRecordFailure records a failure and possibly opens the breaker
func (b *Backend) cbRecordFailure() {
	if !b.cbEnabled {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if b.cbWindow > 0 && !b.cbLastFailure.IsZero() && now.Sub(b.cbLastFailure) > b.cbWindow {
		b.cbFailures = 0
	}
	b.cbLastFailure = now
	if b.cbState == "half-open" {
		// Any failure in half-open re-opens
		b.cbOpenNow(now)
		return
	}
	b.cbFailures++
	if b.cbState == "closed" && b.cbFailures >= b.cbFailureThreshold {
		b.cbOpenNow(now)
	}
}

// cbRecordSuccess records a success and may close the breaker in half-open
func (b *Backend) cbRecordSuccess() {
	if !b.cbEnabled {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cbState == "half-open" {
		b.cbSuccesses++
		if b.cbSuccesses >= b.cbSuccessThreshold {
			b.cbState = "closed"
			b.cbFailures = 0
			b.cbSuccesses = 0
			b.cbLastFailure = time.Time{}
		}
	} else if b.cbState == "closed" {
		// On steady success, decay failures
		if b.cbFailures > 0 {
			b.cbFailures = 0
		}
	}
}

func (b *Backend) cbOpenNow(now time.Time) {
	b.cbState = "open"
	b.cbOpenedAt = now
	b.cbSuccesses = 0
}

// retryTransport wraps a base RoundTripper with retry logic
type retryTransport struct {
	base    http.RoundTripper
	backend *Backend
}

func newRetryTransport(base http.RoundTripper, be *Backend) http.RoundTripper {
	return &retryTransport{base: base, backend: be}
}

func (rt *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	attempts := 0
	delay := rt.backend.retryInitial
	if delay <= 0 {
		delay = 100 * time.Millisecond
	}
	maxDelay := rt.backend.retryMaxDelay
	if maxDelay <= 0 {
		maxDelay = 2 * time.Second
	}

	for {
		if rt.backend.metrics != nil {
			rt.backend.metrics.RecordRetryAttempt()
		}

		resp, err := rt.base.RoundTrip(req)

		// Determine if we should retry
		should := false
		if err != nil {
			// Network errors
			if _, ok := err.(net.Error); ok {
				should = rt.shouldRetryReason("timeout")
			}
			if strings.Contains(strings.ToLower(err.Error()), "connection refused") {
				should = should || rt.shouldRetryReason("connection_refused")
			}
		} else if resp != nil {
			// Retry on specific status codes
			code := resp.StatusCode
			if code == 502 && rt.shouldRetryReason("502") {
				should = true
			}
			if code == 503 && rt.shouldRetryReason("503") {
				should = true
			}
			if code == 504 && rt.shouldRetryReason("504") {
				should = true
			}
			if should {
				// Ensure body is closed before retry
				resp.Body.Close()
			}
		}

		// Stop if no retry or max attempts reached
		if !should || !rt.backend.retryEnabled || attempts >= rt.backend.retryMax-1 {
			if rt.backend.metrics != nil {
				if err == nil {
					rt.backend.metrics.RecordRetrySuccess()
				} else {
					rt.backend.metrics.RecordRetryFailure()
				}
			}
			return resp, err
		}

		attempts++
		time.Sleep(delay)
		// Backoff
		if rt.backend.retryBackoff == "exponential" {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
		// For linear, delay remains constant
	}
}

func (rt *retryTransport) shouldRetryReason(reason string) bool {
	if rt.backend.retryOn == nil {
		return false
	}
	_, ok := rt.backend.retryOn[reason]
	return ok
}

// compressionHandler builds a response modifier that compresses responses when eligible
func (b *Backend) compressionHandler() func(*http.Response) error {
	return func(res *http.Response) error {
		algo, ok := b.shouldCompress(res)
		if !ok {
			return nil
		}
		return b.applyCompression(res, algo)
	}
}

func (b *Backend) shouldCompress(res *http.Response) (string, bool) {
	if !b.compressionEnabled || res == nil || res.Request == nil {
		return "", false
	}

	// Skip on WebSocket or already encoded responses
	if isWebSocketRequest(res.Request) {
		return "", false
	}
	if res.Header.Get("Content-Encoding") != "" {
		return "", false
	}
	if res.Request.Method == http.MethodHead {
		return "", false
	}
	if res.StatusCode < 200 || res.StatusCode == http.StatusNoContent || res.StatusCode == http.StatusNotModified {
		return "", false
	}

	ct := strings.ToLower(res.Header.Get("Content-Type"))
	if ct != "" && !b.contentTypeAllowed(ct) {
		return "", false
	}

	if res.ContentLength >= 0 && b.compressionMinSize > 0 && res.ContentLength < b.compressionMinSize {
		return "", false
	}

	algo := selectAlgorithm(res.Request.Header.Get("Accept-Encoding"), b.compressionAlgos)
	if algo == "" {
		return "", false
	}

	return algo, true
}

func (b *Backend) applyCompression(res *http.Response, algo string) error {
	// Remove length because it will change
	res.Header.Del("Content-Length")
	res.ContentLength = -1
	res.Header.Set("Content-Encoding", algo)
	res.Header.Add("Vary", "Accept-Encoding")

	originalBody := res.Body
	pr, pw := io.Pipe()
	res.Body = pr

	go func() {
		defer originalBody.Close()
		var writer io.WriteCloser
		switch algo {
		case "br":
			level := b.compressionLevelFor(algo)
			writer = brotli.NewWriterLevel(pw, level)
		case "gzip":
			level := b.compressionLevelFor(algo)
			gz, err := gzip.NewWriterLevel(pw, level)
			if err != nil {
				pw.CloseWithError(err)
				return
			}
			writer = gz
		default:
			pw.Close()
			return
		}

		_, err := io.Copy(writer, originalBody)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		writer.Close()
		pw.Close()
	}()

	return nil
}

func (b *Backend) compressionLevelFor(algo string) int {
	level := b.compressionLevel
	switch algo {
	case "gzip":
		if level == 0 {
			return gzip.DefaultCompression
		}
		if level < gzip.HuffmanOnly {
			return gzip.BestSpeed
		}
		if level > gzip.BestCompression {
			return gzip.BestCompression
		}
	case "br":
		if level < 0 {
			return 0
		}
		if level > 11 {
			return 11
		}
	}
	return level
}

func normalizeAlgorithms(list []string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, v := range list {
		v = strings.TrimSpace(strings.ToLower(v))
		if v == "" {
			continue
		}
		if v == "brotli" {
			v = "br"
		}
		if v == "gzip" || v == "br" {
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				out = append(out, v)
			}
		}
	}
	return out
}

func selectAlgorithm(acceptEncoding string, algos []string) string {
	accept := strings.ToLower(acceptEncoding)
	for _, algo := range algos {
		if algo == "br" && strings.Contains(accept, "br") {
			return "br"
		}
		if algo == "gzip" && strings.Contains(accept, "gzip") {
			return "gzip"
		}
	}
	return ""
}

func (b *Backend) contentTypeAllowed(ct string) bool {
	if len(b.compressionTypes) == 0 {
		return true
	}
	for allowed := range b.compressionTypes {
		if strings.HasPrefix(ct, allowed) {
			return true
		}
	}
	return false
}

func isWebSocketRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if strings.ToLower(r.Header.Get("Upgrade")) != "websocket" {
		return false
	}
	return strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// handleWebSocket proxies WebSocket connections with metrics and DB logging
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request, route *Route, backend *Backend) {
	routePath := ""
	if route != nil {
		routePath = route.Path
	}

	if backend.websocketMaxConn > 0 && atomic.LoadInt64(&backend.websocketActive) >= int64(backend.websocketMaxConn) {
		http.Error(w, "WebSocket capacity reached", http.StatusServiceUnavailable)
		return
	}

	backendConn, err := s.dialBackend(backend)
	if err != nil {
		http.Error(w, "Upstream connection failed", http.StatusBadGateway)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "WebSocket not supported", http.StatusInternalServerError)
		backendConn.Close()
		return
	}

	clientConn, _, err := hj.Hijack()
	if err != nil {
		http.Error(w, "Hijack failed", http.StatusInternalServerError)
		backendConn.Close()
		return
	}

	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = tracing.GenerateRequestID()
	}

	outbound := r.Clone(r.Context())
	outbound.URL.Scheme = backend.URL.Scheme
	outbound.URL.Host = backend.URL.Host
	outbound.Host = backend.URL.Host
	outbound.RequestURI = r.URL.RequestURI()
	outbound.Header.Set("Connection", "Upgrade")
	outbound.Header.Set("Upgrade", "websocket")
	outbound.Header.Set("X-Request-ID", requestID)
	outbound.Header.Set("X-Forwarded-For", r.RemoteAddr)

	if err := outbound.Write(backendConn); err != nil {
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n"))
		clientConn.Close()
		backendConn.Close()
		return
	}

	backendReader := bufio.NewReader(backendConn)
	resp, err := http.ReadResponse(backendReader, outbound)
	if err != nil {
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n"))
		clientConn.Close()
		backendConn.Close()
		return
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		resp.Write(clientConn)
		clientConn.Close()
		backendConn.Close()
		return
	}

	resp.Header.Del("Content-Length")
	resp.Header.Del("Transfer-Encoding")
	if err := resp.Write(clientConn); err != nil {
		clientConn.Close()
		backendConn.Close()
		return
	}

	if s.debug {
		log.Debug().Str("host", r.Host).Str("path", routePath).Msg("WebSocket upgrade established")
	}

	atomic.AddInt64(&backend.websocketActive, 1)
	if backend.metrics != nil {
		backend.metrics.IncrementWebSocketActive()
	}

	start := time.Now()
	if backend.websocketMaxDur > 0 {
		deadline := time.Now().Add(backend.websocketMaxDur)
		clientConn.SetDeadline(deadline)
		backendConn.SetDeadline(deadline)
	}

	var dbConnID int64
	if db, ok := s.db.(*database.DB); ok {
		dbConnID, _ = db.InsertWebSocketConnection(&database.WebSocketConnection{
			RequestID:   requestID,
			ClientIP:    r.RemoteAddr,
			ConnectedAt: start.Unix(),
		})
	}

	var toClient, toBackend uint64
	var lastActivity int64
	atomic.StoreInt64(&lastActivity, time.Now().UnixNano())
	done := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		io.Copy(&countingWriter{w: backendConn, counter: &toBackend, activity: &lastActivity}, clientConn)
	}()
	go func() {
		defer wg.Done()
		io.Copy(&countingWriter{w: clientConn, counter: &toClient, activity: &lastActivity}, backendConn)
	}()

	if backend.websocketIdle > 0 {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					if time.Since(time.Unix(0, atomic.LoadInt64(&lastActivity))) > backend.websocketIdle {
						clientConn.Close()
						backendConn.Close()
						return
					}
				}
			}
		}()
	}

	wg.Wait()
	close(done)

	duration := time.Since(start)
	if backend.metrics != nil {
		backend.metrics.RecordWebSocketTransfer(toClient, toBackend, duration)
		backend.metrics.DecrementWebSocketActive()
	}
	atomic.AddInt64(&backend.websocketActive, -1)

	if db, ok := s.db.(*database.DB); ok && dbConnID != 0 {
		_ = db.CloseWebSocketConnection(dbConnID, time.Now().Unix(), toClient, toBackend, 0, 0, "")
	}

	clientConn.Close()
	backendConn.Close()
}

// countingWriter wraps a writer and tracks bytes plus last-activity time
type countingWriter struct {
	w        io.Writer
	counter  *uint64
	activity *int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	if n > 0 {
		if cw.counter != nil {
			atomic.AddUint64(cw.counter, uint64(n))
		}
		if cw.activity != nil {
			atomic.StoreInt64(cw.activity, time.Now().UnixNano())
		}
	}
	return n, err
}

func (s *Server) dialBackend(backend *Backend) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   backend.Timeout,
		KeepAlive: 30 * time.Second,
	}
	if backend.URL.Scheme == "https" {
		return tls.DialWithDialer(dialer, "tcp", backend.URL.Host, &tls.Config{ServerName: backend.URL.Hostname()})
	}
	return dialer.Dial("tcp", backend.URL.Host)
}

func (s *Server) recordSlowMetric(level string) {
	if mc, ok := s.metricsCollector.(*metrics.Collector); ok {
		mc.RecordSlowRequest(level)
	}
}

func (s *Server) sendSlowAlert(route *Route, r *http.Request, duration time.Duration, severity string) {
	notifier, ok := s.notifier.(*webhook.Notifier)
	if !ok || notifier == nil {
		return
	}

	fields := map[string]string{
		"host":     r.Host,
		"path":     r.URL.Path,
		"method":   r.Method,
		"duration": duration.String(),
	}
	if route != nil {
		fields["route"] = route.Path
	}

	alert := webhook.Alert{
		Event:       webhook.EventSlowRequest,
		Title:       "Slow request detected",
		Description: fmt.Sprintf("%s %s took %s", r.Method, r.URL.Path, duration),
		Severity:    severity,
		Fields:      fields,
		Timestamp:   time.Now(),
	}

	if err := notifier.Send(alert); err != nil {
		log.Warn().Err(err).Msg("Failed to send slow request alert")
	}
}

// applyHeaders applies security headers to response
func (s *Server) applyHeaders(w http.ResponseWriter, route *Route) {
	headers := w.Header()

	// Apply global headers first
	if s.globalHeaders.HSTS != "" {
		headers.Set("Strict-Transport-Security", s.globalHeaders.HSTS)
	}
	if s.globalHeaders.XFrameOptions != "" {
		headers.Set("X-Frame-Options", s.globalHeaders.XFrameOptions)
	}
	if s.globalHeaders.XContentType != "" {
		headers.Set("X-Content-Type-Options", s.globalHeaders.XContentType)
	}
	if s.globalHeaders.XSSProtection != "" {
		headers.Set("X-XSS-Protection", s.globalHeaders.XSSProtection)
	}
	if s.globalHeaders.CSP != "" {
		headers.Set("Content-Security-Policy", s.globalHeaders.CSP)
	}
	if s.globalHeaders.ReferrerPolicy != "" {
		headers.Set("Referrer-Policy", s.globalHeaders.ReferrerPolicy)
	}
	if s.globalHeaders.PermissionsPolicy != "" {
		headers.Set("Permissions-Policy", s.globalHeaders.PermissionsPolicy)
	}

	// Apply route-specific headers (override globals)
	if route != nil && route.Headers != nil {
		for k, v := range route.Headers {
			headers.Set(k, v)
		}
	}
}

// blackhole handles unknown domains
func (s *Server) blackhole(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&s.blackholeMetric, 1)

	// Just close the connection without response
	if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
		conn.Close()
	}
}

// redirectToHTTPS redirects HTTP to HTTPS
func (s *Server) redirectToHTTPS(w http.ResponseWriter, r *http.Request) {
	target := "https://" + r.Host + r.URL.RequestURI()
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// tlsConfig returns TLS configuration
func (s *Server) tlsConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: s.getCertificate,
		NextProtos:     []string{"h3", "h2", "http/1.1"}, // HTTP/3, HTTP/2, HTTP/1.1
		MinVersion:     tls.VersionTLS12,
	}
}

// getCertificate returns the appropriate certificate for a domain (supports wildcards)
func (s *Server) getCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := strings.ToLower(hello.ServerName)

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Try exact match first
	for _, mapping := range s.certificates {
		for _, pattern := range mapping.Domains {
			if strings.ToLower(pattern) == domain {
				return &mapping.Cert, nil
			}
		}
	}

	// Try wildcard match
	for _, mapping := range s.certificates {
		for _, pattern := range mapping.Domains {
			if s.matchWildcard(pattern, domain) {
				return &mapping.Cert, nil
			}
		}
	}

	// Fallback to first certificate if available
	if len(s.certificates) > 0 {
		log.Warn().Str("domain", domain).Msg("No matching certificate, using fallback")
		return &s.certificates[0].Cert, nil
	}

	return nil, fmt.Errorf("no certificate available for %s", domain)
}

// matchWildcard checks if domain matches wildcard pattern
func (s *Server) matchWildcard(pattern, domain string) bool {
	pattern = strings.ToLower(pattern)
	domain = strings.ToLower(domain)

	// Not a wildcard pattern
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}

	// Extract base domain from pattern
	baseDomain := pattern[2:] // Remove "*."

	// Domain must end with base domain
	if !strings.HasSuffix(domain, baseDomain) {
		return false
	}

	// Ensure there's at least one subdomain character
	prefix := domain[:len(domain)-len(baseDomain)]
	if len(prefix) == 0 {
		return false
	}

	// Check that we're matching subdomain (not partial match)
	if prefix[len(prefix)-1] != '.' {
		return false
	}

	// Wildcard should only match one level
	// e.g., *.example.com matches sub.example.com but not deep.sub.example.com
	subdomain := prefix[:len(prefix)-1]
	if strings.Contains(subdomain, ".") {
		return false
	}

	return true
}

// hostPolicy is no longer needed but kept for compatibility
func (s *Server) hostPolicy(ctx context.Context, host string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check if any route has this domain
	for _, route := range s.routes {
		for _, domain := range route.Domains {
			if domain == host {
				return nil
			}
		}
	}

	return fmt.Errorf("domain not allowed: %s", host)
}

// sortRoutes sorts routes by priority (longer paths first)
func (s *Server) sortRoutes() {
	// Simple bubble sort (routes list is small)
	for i := 0; i < len(s.routes); i++ {
		for j := i + 1; j < len(s.routes); j++ {
			if s.routes[j].Priority > s.routes[i].Priority {
				s.routes[i], s.routes[j] = s.routes[j], s.routes[i]
			}
		}
	}
}

// routeKey generates a key for route map
func (s *Server) routeKey(domain, path string) string {
	return domain + path
}

// routeMatches checks if route matches domains and path
func (s *Server) routeMatches(route *Route, domains []string, path string) bool {
	if route.Path != path {
		return false
	}
	for _, d1 := range route.Domains {
		for _, d2 := range domains {
			if d1 == d2 {
				return true
			}
		}
	}
	return false
}

// Shutdown gracefully shuts down all servers
func (s *Server) Shutdown(ctx context.Context) error {
	log.Info().Msg("Shutting down servers...")

	var err error
	if s.httpServer != nil {
		if e := s.httpServer.Shutdown(ctx); e != nil {
			err = e
		}
	}
	if s.httpsServer != nil {
		if e := s.httpsServer.Shutdown(ctx); e != nil {
			err = e
		}
	}
	if s.http3Server != nil {
		if e := s.http3Server.Close(); e != nil {
			err = e
		}
	}

	return err
}
