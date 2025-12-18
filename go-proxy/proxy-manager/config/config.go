package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// GlobalConfig holds proxy-wide configuration
type GlobalConfig struct {
	Defaults struct {
		Headers map[string]string `yaml:"headers"`
		Options OptionConfig      `yaml:"options"`
	} `yaml:"defaults"`

	Blackhole struct {
		UnknownDomains bool `yaml:"unknown_domains"`
		MetricsOnly    bool `yaml:"metrics_only"`
	} `yaml:"blackhole"`

	TLS struct {
		Certificates []CertConfig `yaml:"certificates"`
	} `yaml:"tls"`
}

// CertConfig represents a TLS certificate configuration
type CertConfig struct {
	Domains  []string `yaml:"domains"`
	CertFile string   `yaml:"cert_file"`
	KeyFile  string   `yaml:"key_file"`
}

// SiteConfig represents a single site configuration file
type SiteConfig struct {
	Enabled bool `yaml:"enabled"`

	Service struct {
		Name            string `yaml:"name"`
		MaintenancePort int    `yaml:"maintenance_port,omitempty"`
	} `yaml:"service"`

	Routes []RouteConfig `yaml:"routes,omitempty"`

	Headers map[string]string `yaml:"headers,omitempty"`

	Options OptionConfig `yaml:"options,omitempty"`
}

// RouteConfig represents a routing rule
type RouteConfig struct {
	Domains   []string          `yaml:"domains"`
	Path      string            `yaml:"path"`
	Backend   string            `yaml:"backend"`
	WebSocket bool              `yaml:"websocket,omitempty"`
	Headers   map[string]string `yaml:"headers,omitempty"`
}

// OptionConfig represents service options
type OptionConfig struct {
	HealthCheckPath     string        `yaml:"health_check_path,omitempty"`
	HealthCheckInterval string        `yaml:"health_check_interval,omitempty"`
	HealthCheckTimeout  string        `yaml:"health_check_timeout,omitempty"`
	Timeout             string        `yaml:"timeout,omitempty"`
	MaxBodySize         string        `yaml:"max_body_size,omitempty"`
	Compression         CompressionConfig `yaml:"compression,omitempty"`
	WebSocket           WebSocketConfig   `yaml:"websocket,omitempty"`
	HTTP2               *bool         `yaml:"http2,omitempty"`
	HTTP3               *bool         `yaml:"http3,omitempty"`
	Timeouts            TimeoutConfig     `yaml:"timeouts,omitempty"`
	Limits              LimitConfig       `yaml:"limits,omitempty"`
	RateLimit           RateLimitConfig   `yaml:"rate_limit,omitempty"`
	WAF                 WAFConfig         `yaml:"waf,omitempty"`
	PII                 PIIConfig         `yaml:"pii,omitempty"`
	Retention           RetentionConfig   `yaml:"retention,omitempty"`
	GeoIP               GeoIPConfig       `yaml:"geoip,omitempty"`
	ConnectionPool      ConnectionPoolConfig `yaml:"connection_pool,omitempty"`
	SlowRequest         SlowRequestConfig    `yaml:"slow_request,omitempty"`
	Retry               RetryConfig          `yaml:"retry,omitempty"`
	CircuitBreaker      CircuitBreakerConfig `yaml:"circuit_breaker,omitempty"`
}

// GeoIPConfig represents GeoIP tracking settings
type GeoIPConfig struct {
	Enabled               *bool    `yaml:"enabled,omitempty"`
	DatabasePath          string   `yaml:"database_path,omitempty"`
	AlertOnUnusualCountry *bool    `yaml:"alert_on_unusual_country,omitempty"`
	ExpectedCountries     []string `yaml:"expected_countries,omitempty"` // ISO 3166-1 alpha-2 codes
	CacheExpiryMinutes    int      `yaml:"cache_expiry_minutes,omitempty"`
}

// RetentionConfig represents data retention policy settings
type RetentionConfig struct {
	Enabled          *bool  `yaml:"enabled,omitempty"`           // Default: true
	AccessLogDays    int    `yaml:"access_log_days,omitempty"`    // Default: 30
	SecurityLogDays  int    `yaml:"security_log_days,omitempty"`  // Default: 90
	AuditLogDays     int    `yaml:"audit_log_days,omitempty"`     // Default: 365
	MetricsDays      int    `yaml:"metrics_days,omitempty"`       // Default: 90
	HealthCheckDays  int    `yaml:"health_check_days,omitempty"`  // Default: 7
	WebSocketLogDays int    `yaml:"websocket_log_days,omitempty"` // Default: 30
	PolicyType       string `yaml:"policy_type,omitempty"`        // public, private, custom
}

// PIIConfig represents PII masking settings for GDPR compliance
type PIIConfig struct {
	Enabled           *bool    `yaml:"enabled,omitempty"`
	MaskIPMethod      string   `yaml:"mask_ip_method,omitempty"`      // last_octet, hash, full
	MaskIPv6Method    string   `yaml:"mask_ipv6_method,omitempty"`    // last_64, hash, full
	StripHeaders      []string `yaml:"strip_headers,omitempty"`       // Headers to remove
	MaskQueryParams   []string `yaml:"mask_query_params,omitempty"`   // Query params to mask
	PreserveLocalhost *bool    `yaml:"preserve_localhost,omitempty"`  // Don't mask private IPs
}

// WAFConfig represents Web Application Firewall settings
type WAFConfig struct {
	Enabled      *bool    `yaml:"enabled,omitempty"`
	BlockMode    *bool    `yaml:"block_mode,omitempty"`    // true = block, false = log only
	Sensitivity  string   `yaml:"sensitivity,omitempty"`   // low, medium, high
	CheckPath    *bool    `yaml:"check_path,omitempty"`    // Check URL path
	CheckHeaders *bool    `yaml:"check_headers,omitempty"` // Check HTTP headers
	CheckQuery   *bool    `yaml:"check_query,omitempty"`   // Check query parameters
	CheckBody    *bool    `yaml:"check_body,omitempty"`    // Check request body
	MaxBodySize  int64    `yaml:"max_body_size,omitempty"` // Max body size to inspect
	Whitelist    []string `yaml:"whitelist,omitempty"`     // Whitelisted IPs/CIDRs
}

// RateLimitConfig represents rate limiting settings
type RateLimitConfig struct {
	Enabled         *bool  `yaml:"enabled,omitempty"`
	RequestsPerMin  int    `yaml:"requests_per_min,omitempty"`  // Default: 60
	RequestsPerHour int    `yaml:"requests_per_hour,omitempty"` // Default: 1000
	BurstSize       int    `yaml:"burst_size,omitempty"`        // Default: 10% of per-min
	PerIP           *bool  `yaml:"per_ip,omitempty"`            // Default: true
	PerRoute        *bool  `yaml:"per_route,omitempty"`         // Default: false
	Whitelist       []string `yaml:"whitelist,omitempty"`       // Whitelisted IPs/CIDRs
}

// TimeoutConfig represents timeout settings for a route
type TimeoutConfig struct {
	Connect time.Duration `yaml:"connect,omitempty"` // Default: 5s
	Read    time.Duration `yaml:"read,omitempty"`    // Default: 30s
	Write   time.Duration `yaml:"write,omitempty"`   // Default: 30s
	Idle    time.Duration `yaml:"idle,omitempty"`    // Default: 120s
}

// LimitConfig represents size limits for requests/responses
type LimitConfig struct {
	MaxRequestBody  int64 `yaml:"max_request_body,omitempty"`  // Bytes, default: 10MB
	MaxResponseBody int64 `yaml:"max_response_body,omitempty"` // Bytes, default: 10MB
}

// CompressionConfig represents response compression settings
type CompressionConfig struct {
	Enabled      *bool    `yaml:"enabled,omitempty"`
	Algorithms   []string `yaml:"algorithms,omitempty"`   // brotli, gzip
	Level        int      `yaml:"level,omitempty"`        // algorithm-specific level
	MinSize      int64    `yaml:"min_size,omitempty"`     // bytes
	ContentTypes []string `yaml:"content_types,omitempty"`
}

// UnmarshalYAML allows boolean or map for compression configuration
func (c *CompressionConfig) UnmarshalYAML(value *yaml.Node) error {
	var enabledOnly bool
	if err := value.Decode(&enabledOnly); err == nil {
		c.Enabled = &enabledOnly
		return nil
	}

	type raw CompressionConfig
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	*c = CompressionConfig(r)
	return nil
}

// WebSocketConfig represents WebSocket tuning settings
type WebSocketConfig struct {
	Enabled        *bool         `yaml:"enabled,omitempty"`
	MaxConnections int           `yaml:"max_connections,omitempty"`
	MaxDuration    time.Duration `yaml:"max_duration,omitempty"`
	IdleTimeout    time.Duration `yaml:"idle_timeout,omitempty"`
	PingInterval   time.Duration `yaml:"ping_interval,omitempty"`
}

// UnmarshalYAML allows boolean or map for websocket configuration
func (w *WebSocketConfig) UnmarshalYAML(value *yaml.Node) error {
	var enabledOnly bool
	if err := value.Decode(&enabledOnly); err == nil {
		w.Enabled = &enabledOnly
		return nil
	}

	type raw WebSocketConfig
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	*w = WebSocketConfig(r)
	return nil
}

// ConnectionPoolConfig represents HTTP connection pooling settings
type ConnectionPoolConfig struct {
	MaxIdleConns        int           `yaml:"max_idle_conns,omitempty"`
	MaxIdleConnsPerHost int           `yaml:"max_idle_conns_per_host,omitempty"`
	MaxConnsPerHost     int           `yaml:"max_conns_per_host,omitempty"`
	IdleTimeout         time.Duration `yaml:"idle_timeout,omitempty"`
}

// SlowRequestConfig represents slow request detection thresholds
type SlowRequestConfig struct {
	Enabled      *bool         `yaml:"enabled,omitempty"`
	Warning      time.Duration `yaml:"warning,omitempty"`
	Critical     time.Duration `yaml:"critical,omitempty"`
	Timeout      time.Duration `yaml:"timeout,omitempty"`
	AlertWebhook *bool         `yaml:"alert_webhook,omitempty"`
}

// RetryConfig represents request retry behavior
type RetryConfig struct {
	Enabled      *bool    `yaml:"enabled,omitempty"`
	MaxAttempts  int      `yaml:"max_attempts,omitempty"`
	Backoff      string   `yaml:"backoff,omitempty"`      // exponential, linear
	InitialDelay string   `yaml:"initial_delay,omitempty"` // e.g., 100ms
	MaxDelay     string   `yaml:"max_delay,omitempty"`     // e.g., 2s
	RetryOn      []string `yaml:"retry_on,omitempty"`      // e.g., ["connection_refused","timeout","502","503"]
}

// CircuitBreakerConfig represents circuit breaker settings (Phase 6)
type CircuitBreakerConfig struct {
	Enabled          *bool  `yaml:"enabled,omitempty"`
	FailureThreshold int    `yaml:"failure_threshold,omitempty"`
	SuccessThreshold int    `yaml:"success_threshold,omitempty"`
	Timeout          string `yaml:"timeout,omitempty"` // e.g., 30s
	Window           string `yaml:"window,omitempty"`  // e.g., 60s
}

// GetTimeouts returns timeout configuration with defaults
func (t *TimeoutConfig) GetTimeouts() TimeoutConfig {
	defaults := TimeoutConfig{
		Connect: 5 * time.Second,
		Read:    30 * time.Second,
		Write:   30 * time.Second,
		Idle:    120 * time.Second,
	}

	if t.Connect > 0 {
		defaults.Connect = t.Connect
	}
	if t.Read > 0 {
		defaults.Read = t.Read
	}
	if t.Write > 0 {
		defaults.Write = t.Write
	}
	if t.Idle > 0 {
		defaults.Idle = t.Idle
	}

	return defaults
}

// GetLimits returns limit configuration with defaults
func (l *LimitConfig) GetLimits() LimitConfig {
	defaults := LimitConfig{
		MaxRequestBody:  10 * 1024 * 1024,  // 10 MB
		MaxResponseBody: 10 * 1024 * 1024,  // 10 MB
	}

	if l.MaxRequestBody > 0 {
		defaults.MaxRequestBody = l.MaxRequestBody
	}
	if l.MaxResponseBody > 0 {
		defaults.MaxResponseBody = l.MaxResponseBody
	}

	return defaults
}

// GetCompression returns compression configuration with defaults
func (c *CompressionConfig) GetCompression() CompressionConfig {
	falseVal := false
	defaults := CompressionConfig{
		Enabled:      &falseVal,
		Algorithms:   []string{"br", "gzip"},
		Level:        5,
		MinSize:      1024,
		ContentTypes: []string{"text/html", "text/css", "application/javascript", "application/json", "image/svg+xml"},
	}

	if c.Enabled != nil {
		defaults.Enabled = c.Enabled
	}
	if len(c.Algorithms) > 0 {
		defaults.Algorithms = c.Algorithms
	}
	if c.Level != 0 {
		defaults.Level = c.Level
	}
	if c.MinSize > 0 {
		defaults.MinSize = c.MinSize
	}
	if len(c.ContentTypes) > 0 {
		defaults.ContentTypes = c.ContentTypes
	}

	return defaults
}

// GetWebSocket returns websocket configuration with defaults
func (w *WebSocketConfig) GetWebSocket() WebSocketConfig {
	falseVal := false
	defaults := WebSocketConfig{
		Enabled:        &falseVal,
		MaxConnections: 0,
		MaxDuration:    24 * time.Hour,
		IdleTimeout:    5 * time.Minute,
		PingInterval:   30 * time.Second,
	}

	if w.Enabled != nil {
		defaults.Enabled = w.Enabled
	}
	if w.MaxConnections > 0 {
		defaults.MaxConnections = w.MaxConnections
	}
	if w.MaxDuration > 0 {
		defaults.MaxDuration = w.MaxDuration
	}
	if w.IdleTimeout > 0 {
		defaults.IdleTimeout = w.IdleTimeout
	}
	if w.PingInterval > 0 {
		defaults.PingInterval = w.PingInterval
	}

	return defaults
}

// GetConnectionPool returns connection pool configuration with defaults
func (p *ConnectionPoolConfig) GetConnectionPool() ConnectionPoolConfig {
	defaults := ConnectionPoolConfig{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     50,
		IdleTimeout:         90 * time.Second,
	}
	if p.MaxIdleConns > 0 {
		defaults.MaxIdleConns = p.MaxIdleConns
	}
	if p.MaxIdleConnsPerHost > 0 {
		defaults.MaxIdleConnsPerHost = p.MaxIdleConnsPerHost
	}
	if p.MaxConnsPerHost > 0 {
		defaults.MaxConnsPerHost = p.MaxConnsPerHost
	}
	if p.IdleTimeout > 0 {
		defaults.IdleTimeout = p.IdleTimeout
	}
	return defaults
}

// GetSlowRequest returns slow request configuration with defaults
func (s *SlowRequestConfig) GetSlowRequest() SlowRequestConfig {
	trueVal := true
	defaults := SlowRequestConfig{
		Enabled:      &trueVal,
		Warning:      5 * time.Second,
		Critical:     10 * time.Second,
		Timeout:      30 * time.Second,
		AlertWebhook: &trueVal,
	}
	if s.Enabled != nil {
		defaults.Enabled = s.Enabled
	}
	if s.Warning > 0 {
		defaults.Warning = s.Warning
	}
	if s.Critical > 0 {
		defaults.Critical = s.Critical
	}
	if s.Timeout > 0 {
		defaults.Timeout = s.Timeout
	}
	if s.AlertWebhook != nil {
		defaults.AlertWebhook = s.AlertWebhook
	}
	return defaults
}

// GetRetry returns retry configuration with defaults
func (r *RetryConfig) GetRetry() RetryConfig {
	falseVal := false
	defaults := RetryConfig{
		Enabled:      &falseVal,
		MaxAttempts:  3,
		Backoff:      "exponential",
		InitialDelay: "100ms",
		MaxDelay:     "2s",
		RetryOn:      []string{"connection_refused", "timeout", "502", "503"},
	}
	if r.Enabled != nil {
		defaults.Enabled = r.Enabled
	}
	if r.MaxAttempts > 0 {
		defaults.MaxAttempts = r.MaxAttempts
	}
	if r.Backoff != "" {
		defaults.Backoff = r.Backoff
	}
	if r.InitialDelay != "" {
		defaults.InitialDelay = r.InitialDelay
	}
	if r.MaxDelay != "" {
		defaults.MaxDelay = r.MaxDelay
	}
	if len(r.RetryOn) > 0 {
		defaults.RetryOn = r.RetryOn
	}
	return defaults
}

// GetCircuitBreaker returns circuit breaker configuration with defaults
func (c *CircuitBreakerConfig) GetCircuitBreaker() CircuitBreakerConfig {
	falseVal := false
	defaults := CircuitBreakerConfig{
		Enabled:          &falseVal,
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          "30s",
		Window:           "60s",
	}
	if c.Enabled != nil {
		defaults.Enabled = c.Enabled
	}
	if c.FailureThreshold > 0 {
		defaults.FailureThreshold = c.FailureThreshold
	}
	if c.SuccessThreshold > 0 {
		defaults.SuccessThreshold = c.SuccessThreshold
	}
	if c.Timeout != "" {
		defaults.Timeout = c.Timeout
	}
	if c.Window != "" {
		defaults.Window = c.Window
	}
	return defaults
}

// GetRateLimit returns rate limit configuration with defaults
func (r *RateLimitConfig) GetRateLimit() RateLimitConfig {
	trueVal := true
	falseVal := false

	defaults := RateLimitConfig{
		Enabled:         &falseVal,       // Disabled by default
		RequestsPerMin:  60,              // 60 requests per minute
		RequestsPerHour: 1000,            // 1000 requests per hour
		BurstSize:       6,               // 10% of per-min
		PerIP:           &trueVal,        // Per-IP limiting enabled
		PerRoute:        &falseVal,       // Per-route limiting disabled
		Whitelist:       make([]string, 0),
	}

	if r.Enabled != nil {
		defaults.Enabled = r.Enabled
	}
	if r.RequestsPerMin > 0 {
		defaults.RequestsPerMin = r.RequestsPerMin
	}
	if r.RequestsPerHour > 0 {
		defaults.RequestsPerHour = r.RequestsPerHour
	}
	if r.BurstSize > 0 {
		defaults.BurstSize = r.BurstSize
	}
	if r.PerIP != nil {
		defaults.PerIP = r.PerIP
	}
	if r.PerRoute != nil {
		defaults.PerRoute = r.PerRoute
	}
	if len(r.Whitelist) > 0 {
		defaults.Whitelist = r.Whitelist
	}

	return defaults
}

// GetWAF returns WAF configuration with defaults
func (w *WAFConfig) GetWAF() WAFConfig {
	trueVal := true
	falseVal := false

	defaults := WAFConfig{
		Enabled:      &falseVal,      // Disabled by default
		BlockMode:    &trueVal,       // Block mode by default
		Sensitivity:  "medium",       // Medium sensitivity
		CheckPath:    &trueVal,       // Check path
		CheckHeaders: &trueVal,       // Check headers
		CheckQuery:   &trueVal,       // Check query params
		CheckBody:    &trueVal,       // Check body
		MaxBodySize:  1024 * 1024,    // 1 MB
		Whitelist:    make([]string, 0),
	}

	if w.Enabled != nil {
		defaults.Enabled = w.Enabled
	}
	if w.BlockMode != nil {
		defaults.BlockMode = w.BlockMode
	}
	if w.Sensitivity != "" {
		defaults.Sensitivity = w.Sensitivity
	}
	if w.CheckPath != nil {
		defaults.CheckPath = w.CheckPath
	}
	if w.CheckHeaders != nil {
		defaults.CheckHeaders = w.CheckHeaders
	}
	if w.CheckQuery != nil {
		defaults.CheckQuery = w.CheckQuery
	}
	if w.CheckBody != nil {
		defaults.CheckBody = w.CheckBody
	}
	if w.MaxBodySize > 0 {
		defaults.MaxBodySize = w.MaxBodySize
	}
	if len(w.Whitelist) > 0 {
		defaults.Whitelist = w.Whitelist
	}

	return defaults
}

// GetPII returns PII masking configuration with defaults
func (p *PIIConfig) GetPII() PIIConfig {
	trueVal := true
	falseVal := false

	defaults := PIIConfig{
		Enabled:           &falseVal,       // Disabled by default
		MaskIPMethod:      "last_octet",    // Mask last octet of IPv4
		MaskIPv6Method:    "last_64",       // Mask last 64 bits of IPv6
		StripHeaders:      []string{},      // Use package defaults
		MaskQueryParams:   []string{},      // Use package defaults
		PreserveLocalhost: &trueVal,        // Don't mask private IPs
	}

	if p.Enabled != nil {
		defaults.Enabled = p.Enabled
	}
	if p.MaskIPMethod != "" {
		defaults.MaskIPMethod = p.MaskIPMethod
	}
	if p.MaskIPv6Method != "" {
		defaults.MaskIPv6Method = p.MaskIPv6Method
	}
	if len(p.StripHeaders) > 0 {
		defaults.StripHeaders = p.StripHeaders
	}
	if len(p.MaskQueryParams) > 0 {
		defaults.MaskQueryParams = p.MaskQueryParams
	}
	if p.PreserveLocalhost != nil {
		defaults.PreserveLocalhost = p.PreserveLocalhost
	}

	return defaults
}

// GetGeoIP returns GeoIP configuration with defaults
func (g *GeoIPConfig) GetGeoIP() GeoIPConfig {
	falseVal := false

	defaults := GeoIPConfig{
		Enabled:               &falseVal,      // Disabled by default
		DatabasePath:          "/data/GeoLite2-City.mmdb",
		AlertOnUnusualCountry: &falseVal,      // No alerts by default
		ExpectedCountries:     []string{},     // All countries allowed
		CacheExpiryMinutes:    30,             // 30 minute cache
	}

	if g.Enabled != nil {
		defaults.Enabled = g.Enabled
	}
	if g.DatabasePath != "" {
		defaults.DatabasePath = g.DatabasePath
	}
	if g.AlertOnUnusualCountry != nil {
		defaults.AlertOnUnusualCountry = g.AlertOnUnusualCountry
	}
	if len(g.ExpectedCountries) > 0 {
		defaults.ExpectedCountries = g.ExpectedCountries
	}
	if g.CacheExpiryMinutes > 0 {
		defaults.CacheExpiryMinutes = g.CacheExpiryMinutes
	}

	return defaults
}

// LoadGlobalConfig loads global configuration from YAML file
func LoadGlobalConfig(path string) (*GlobalConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg GlobalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &cfg, nil
}

// LoadSiteConfig loads a site configuration from YAML file
func LoadSiteConfig(path string) (*SiteConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg SiteConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &cfg, nil
}

// Validate validates the site configuration
func (c *SiteConfig) Validate() error {
	if c.Service.Name == "" {
		return fmt.Errorf("service.name is required")
	}

	if len(c.Routes) == 0 {
		return fmt.Errorf("at least one route is required")
	}

	for i, route := range c.Routes {
		if len(route.Domains) == 0 {
			return fmt.Errorf("route %d: domains is required", i)
		}
		if route.Path == "" {
			return fmt.Errorf("route %d: path is required", i)
		}
		if route.Backend == "" {
			return fmt.Errorf("route %d: backend is required", i)
		}
	}

	return nil
}

// GetOptions returns parsed options with defaults
func (c *SiteConfig) GetOptions() (map[string]interface{}, error) {
	if c == nil {
		return nil, fmt.Errorf("SiteConfig is nil")
	}
	opts := make(map[string]interface{})

	if c.Options.HealthCheckPath != "" {
		opts["health_check_path"] = c.Options.HealthCheckPath
	}

	if c.Options.HealthCheckInterval != "" {
		dur, err := time.ParseDuration(c.Options.HealthCheckInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid health_check_interval: %w", err)
		}
		opts["health_check_interval"] = dur
	}

	if c.Options.HealthCheckTimeout != "" {
		dur, err := time.ParseDuration(c.Options.HealthCheckTimeout)
		if err != nil {
			return nil, fmt.Errorf("invalid health_check_timeout: %w", err)
		}
		opts["health_check_timeout"] = dur
	}

	if c.Options.Timeout != "" {
		dur, err := time.ParseDuration(c.Options.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
		opts["timeout"] = dur
	}

	if c.Options.MaxBodySize != "" {
		size, err := parseSize(c.Options.MaxBodySize)
		if err != nil {
			return nil, fmt.Errorf("invalid max_body_size: %w", err)
		}
		opts["max_body_size"] = size
	}

	// Compression settings
	comp := c.Options.Compression.GetCompression()
	opts["compression"] = map[string]interface{}{
		"enabled":       boolValue(comp.Enabled),
		"algorithms":    comp.Algorithms,
		"level":         comp.Level,
		"min_size":      comp.MinSize,
		"content_types": comp.ContentTypes,
	}

	// WebSocket settings
	ws := c.Options.WebSocket.GetWebSocket()
	opts["websocket"] = map[string]interface{}{
		"enabled":         boolValue(ws.Enabled),
		"max_connections": ws.MaxConnections,
		"max_duration":    ws.MaxDuration,
		"idle_timeout":    ws.IdleTimeout,
		"ping_interval":   ws.PingInterval,
	}

	if c.Options.HTTP2 != nil {
		opts["http2"] = *c.Options.HTTP2
	}

	if c.Options.HTTP3 != nil {
		opts["http3"] = *c.Options.HTTP3
	}

	// Connection pool settings
	pool := c.Options.ConnectionPool.GetConnectionPool()
	opts["pool"] = map[string]interface{}{
		"max_idle_conns":        pool.MaxIdleConns,
		"max_idle_conns_per_host": pool.MaxIdleConnsPerHost,
		"max_conns_per_host":    pool.MaxConnsPerHost,
		"idle_timeout":          pool.IdleTimeout,
	}

	// Slow request detection
	slow := c.Options.SlowRequest.GetSlowRequest()
	opts["slow_request"] = map[string]interface{}{
		"enabled":       boolValue(slow.Enabled),
		"warning":       slow.Warning,
		"critical":      slow.Critical,
		"timeout":       slow.Timeout,
		"alert_webhook": boolValue(slow.AlertWebhook),
	}

	// Retry logic
	retry := c.Options.Retry.GetRetry()
	initialDelay, err := time.ParseDuration(retry.InitialDelay)
	if err != nil {
		return nil, fmt.Errorf("invalid retry.initial_delay: %w", err)
	}
	maxDelay, err := time.ParseDuration(retry.MaxDelay)
	if err != nil {
		return nil, fmt.Errorf("invalid retry.max_delay: %w", err)
	}
	opts["retry"] = map[string]interface{}{
		"enabled":       boolValue(retry.Enabled),
		"max_attempts":  retry.MaxAttempts,
		"backoff":       retry.Backoff,
		"initial_delay": initialDelay,
		"max_delay":     maxDelay,
		"retry_on":      retry.RetryOn,
	}

	// Circuit breaker
	cb := c.Options.CircuitBreaker.GetCircuitBreaker()
	cbTimeout, err := time.ParseDuration(cb.Timeout)
	if err != nil {
		return nil, fmt.Errorf("invalid circuit_breaker.timeout: %w", err)
	}
	cbWindow, err := time.ParseDuration(cb.Window)
	if err != nil {
		return nil, fmt.Errorf("invalid circuit_breaker.window: %w", err)
	}
	opts["circuit_breaker"] = map[string]interface{}{
		"enabled":           boolValue(cb.Enabled),
		"failure_threshold": cb.FailureThreshold,
		"success_threshold": cb.SuccessThreshold,
		"timeout":           cbTimeout,
		"window":            cbWindow,
	}

	return opts, nil
}

func boolValue(p *bool) bool { if p == nil { return false }; return *p }

// GetRetention returns retention configuration with defaults
func (c *SiteConfig) GetRetention() RetentionConfig {
	if c == nil {
		enabled := false
		return RetentionConfig{Enabled: &enabled}
	}
	enabled := true
	if c.Options.Retention.Enabled != nil {
		enabled = *c.Options.Retention.Enabled
	}
	
	if !enabled {
		return RetentionConfig{Enabled: &enabled}
	}
	
	// Default retention settings
	accessLogDays := 30
	securityLogDays := 90
	auditLogDays := 365
	metricsDays := 90
	healthCheckDays := 7
	websocketLogDays := 30
	policyType := "default"
	
	// Override with policy type presets
	if c.Options.Retention.PolicyType == "public" {
		accessLogDays = 7
		securityLogDays = 30
		auditLogDays = 90
		metricsDays = 30
		healthCheckDays = 7
		websocketLogDays = 7
		policyType = "public"
	} else if c.Options.Retention.PolicyType == "private" {
		accessLogDays = 30
		securityLogDays = 90
		auditLogDays = 365
		metricsDays = 90
		healthCheckDays = 7
		websocketLogDays = 30
		policyType = "private"
	}
	
	// Override with specific values if provided
	if c.Options.Retention.AccessLogDays > 0 {
		accessLogDays = c.Options.Retention.AccessLogDays
	}
	if c.Options.Retention.SecurityLogDays > 0 {
		securityLogDays = c.Options.Retention.SecurityLogDays
	}
	if c.Options.Retention.AuditLogDays > 0 {
		auditLogDays = c.Options.Retention.AuditLogDays
	}
	if c.Options.Retention.MetricsDays > 0 {
		metricsDays = c.Options.Retention.MetricsDays
	}
	if c.Options.Retention.HealthCheckDays > 0 {
		healthCheckDays = c.Options.Retention.HealthCheckDays
	}
	if c.Options.Retention.WebSocketLogDays > 0 {
		websocketLogDays = c.Options.Retention.WebSocketLogDays
	}
	
	return RetentionConfig{
		Enabled:          &enabled,
		AccessLogDays:    accessLogDays,
		SecurityLogDays:  securityLogDays,
		AuditLogDays:     auditLogDays,
		MetricsDays:      metricsDays,
		HealthCheckDays:  healthCheckDays,
		WebSocketLogDays: websocketLogDays,
		PolicyType:       policyType,
	}
}

// parseSize parses size strings like "10M", "1G", "512K"
func parseSize(s string) (int64, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("empty size string")
	}

	multiplier := int64(1)
	unit := s[len(s)-1]

	switch unit {
	case 'K', 'k':
		multiplier = 1024
		s = s[:len(s)-1]
	case 'M', 'm':
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	case 'G', 'g':
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	}

	var size int64
	if _, err := fmt.Sscanf(s, "%d", &size); err != nil {
		return 0, fmt.Errorf("invalid size format: %w", err)
	}

	return size * multiplier, nil
}
