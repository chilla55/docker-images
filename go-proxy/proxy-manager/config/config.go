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
	Compression         *bool         `yaml:"compression,omitempty"`
	WebSocket           *bool         `yaml:"websocket,omitempty"`
	HTTP2               *bool         `yaml:"http2,omitempty"`
	HTTP3               *bool         `yaml:"http3,omitempty"`
	Timeouts            TimeoutConfig `yaml:"timeouts,omitempty"`
	Limits              LimitConfig   `yaml:"limits,omitempty"`
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
		MaxRequestBody:  10 * 1024 * 1024, // 10 MB
		MaxResponseBody: 10 * 1024 * 1024, // 10 MB
	}

	if l.MaxRequestBody > 0 {
		defaults.MaxRequestBody = l.MaxRequestBody
	}
	if l.MaxResponseBody > 0 {
		defaults.MaxResponseBody = l.MaxResponseBody
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

	if c.Options.Compression != nil {
		opts["compression"] = *c.Options.Compression
	}

	if c.Options.WebSocket != nil {
		opts["websocket"] = *c.Options.WebSocket
	}

	if c.Options.HTTP2 != nil {
		opts["http2"] = *c.Options.HTTP2
	}

	if c.Options.HTTP3 != nil {
		opts["http3"] = *c.Options.HTTP3
	}

	return opts, nil
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
