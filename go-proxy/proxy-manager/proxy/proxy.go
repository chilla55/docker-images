package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

	db    interface{} // Database connection (interface to avoid import cycle)
	debug bool
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
}

// NewServer creates a new proxy server
func NewServer(cfg Config) *Server {
	s := &Server{
		routes:        make([]*Route, 0),
		routeMap:      make(map[string]*Backend),
		globalHeaders: cfg.GlobalHeaders,
		certificates:  cfg.Certificates,
		db:            cfg.DB,
		debug:         cfg.Debug,
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

	// Check if backend is healthy
	backend.mu.RLock()
	healthy := backend.Healthy
	backend.mu.RUnlock()

	if !healthy {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	// Get route for headers
	route := s.findRoute(r.Host, r.URL.Path)

	// Apply security headers
	s.applyHeaders(w, route)

	// Proxy request
	backend.Proxy.ServeHTTP(w, r)
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
		URL:           target,
		Proxy:         proxy,
		Healthy:       true,
		HealthPath:    "/",
		HealthTimeout: 5 * time.Second,
		Timeout:       30 * time.Second,
		MaxBodySize:   100 * 1024 * 1024, // 100MB
	}

	// Apply options
	if options != nil {
		if v, ok := options["health_check_path"].(string); ok {
			backend.HealthPath = v
		}
		if v, ok := options["timeout"].(time.Duration); ok {
			backend.Timeout = v
		}
	}

	return backend
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
