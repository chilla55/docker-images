package pii

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/rs/zerolog"
)

// Config holds PII masking configuration
type Config struct {
	Enabled           bool
	MaskIPMethod      string   // "last_octet", "hash", "full"
	MaskIPv6Method    string   // "last_64", "hash", "full"
	StripHeaders      []string // Headers to remove from logs
	MaskQueryParams   []string // Query params to mask
	MaskFormFields    []string // Form fields to mask
	PreserveLocalhost bool     // Don't mask localhost/private IPs
}

// Masker handles PII masking operations
type Masker struct {
	config           Config
	stripHeaders     map[string]bool
	maskQueryParams  map[string]bool
	maskFormFields   map[string]bool
	sensitivePattern *regexp.Regexp
}

// NewMasker creates a new PII masker
func NewMasker(config Config) *Masker {
	// Default sensitive patterns
	if config.MaskIPMethod == "" {
		config.MaskIPMethod = "last_octet"
	}
	if config.MaskIPv6Method == "" {
		config.MaskIPv6Method = "last_64"
	}

	// Default headers to strip
	if len(config.StripHeaders) == 0 {
		config.StripHeaders = []string{
			"Authorization",
			"Cookie",
			"Set-Cookie",
			"X-API-Key",
			"X-Auth-Token",
			"Proxy-Authorization",
		}
	}

	// Default query params to mask
	if len(config.MaskQueryParams) == 0 {
		config.MaskQueryParams = []string{
			"token",
			"api_key",
			"apikey",
			"password",
			"passwd",
			"pwd",
			"secret",
			"key",
			"auth",
			"access_token",
			"refresh_token",
			"session",
			"sessionid",
		}
	}

	// Default form fields to mask
	if len(config.MaskFormFields) == 0 {
		config.MaskFormFields = []string{
			"password",
			"passwd",
			"pwd",
			"credit_card",
			"creditcard",
			"card_number",
			"cvv",
			"ssn",
			"social_security",
		}
	}

	m := &Masker{
		config:          config,
		stripHeaders:    make(map[string]bool),
		maskQueryParams: make(map[string]bool),
		maskFormFields:  make(map[string]bool),
	}

	// Build lookup maps for performance
	for _, h := range config.StripHeaders {
		m.stripHeaders[strings.ToLower(h)] = true
	}
	for _, p := range config.MaskQueryParams {
		m.maskQueryParams[strings.ToLower(p)] = true
	}
	for _, f := range config.MaskFormFields {
		m.maskFormFields[strings.ToLower(f)] = true
	}

	// Pattern for finding sensitive data in strings
	m.sensitivePattern = regexp.MustCompile(`(?i)(password|token|key|secret|auth|credit|card|ssn|social)`)

	return m
}

// MaskIP masks an IP address according to configuration
func (m *Masker) MaskIP(ip string) string {
	if !m.config.Enabled {
		return ip
	}

	// Check if it's localhost or private IP
	if m.config.PreserveLocalhost && isPrivateIP(ip) {
		return ip
	}

	// Detect IPv4 vs IPv6
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "[invalid-ip]"
	}

	// IPv4
	if parsedIP.To4() != nil {
		switch m.config.MaskIPMethod {
		case "last_octet":
			return maskIPv4LastOctet(ip)
		case "hash":
			return hashIP(ip)
		case "full":
			return "[masked]"
		default:
			return maskIPv4LastOctet(ip)
		}
	}

	// IPv6
	switch m.config.MaskIPv6Method {
	case "last_64":
		return maskIPv6Last64(ip)
	case "hash":
		return hashIP(ip)
	case "full":
		return "[masked]"
	default:
		return maskIPv6Last64(ip)
	}
}

// MaskHeaders masks sensitive headers from HTTP headers
func (m *Masker) MaskHeaders(headers http.Header) http.Header {
	if !m.config.Enabled {
		return headers
	}

	masked := make(http.Header)
	for key, values := range headers {
		lowerKey := strings.ToLower(key)

		if m.stripHeaders[lowerKey] {
			masked[key] = []string{"[masked]"}
		} else {
			masked[key] = values
		}
	}

	return masked
}

// MaskQueryParams masks sensitive query parameters
func (m *Masker) MaskQueryParams(query url.Values) url.Values {
	if !m.config.Enabled {
		return query
	}

	masked := make(url.Values)
	for key, values := range query {
		lowerKey := strings.ToLower(key)

		if m.maskQueryParams[lowerKey] {
			masked[key] = []string{"[masked]"}
		} else {
			masked[key] = values
		}
	}

	return masked
}

// MaskURL masks sensitive parts of a URL string
func (m *Masker) MaskURL(urlStr string) string {
	if !m.config.Enabled {
		return urlStr
	}

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}

	// Mask query parameters
	if parsed.RawQuery != "" {
		query := parsed.Query()
		masked := m.MaskQueryParams(query)
		parsed.RawQuery = masked.Encode()
	}

	// Mask user info (username:password in URL)
	if parsed.User != nil {
		parsed.User = url.UserPassword("[masked]", "[masked]")
	}

	return parsed.String()
}

// MaskString masks sensitive patterns in a string
func (m *Masker) MaskString(s string) string {
	if !m.config.Enabled {
		return s
	}

	// Simple pattern-based masking for log messages
	// This catches things like "password=secret123"
	re := regexp.MustCompile(`(?i)(password|token|key|secret|auth)[\s=:]+([^\s&'"]+)`)
	return re.ReplaceAllString(s, "$1=[masked]")
}

// MaskLogEvent masks PII in zerolog events
func (m *Masker) MaskLogEvent(e *zerolog.Event, ip, url, headers string) *zerolog.Event {
	if !m.config.Enabled {
		return e.Str("ip", ip).Str("url", url).Str("headers", headers)
	}

	return e.
		Str("ip", m.MaskIP(ip)).
		Str("url", m.MaskURL(url)).
		Str("headers", m.MaskString(headers))
}

// maskIPv4LastOctet masks the last octet of an IPv4 address
// Example: 203.0.113.45 -> 203.0.113.xxx
func maskIPv4LastOctet(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return "[invalid-ipv4]"
	}

	return strings.Join(parts[:3], ".") + ".xxx"
}

// maskIPv6Last64 masks the last 64 bits of an IPv6 address
// Example: 2001:db8::1 -> 2001:db8::/64
func maskIPv6Last64(ip string) string {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return "[invalid-ipv6]"
	}

	// Get first 8 bytes (64 bits)
	ipBytes := parsedIP.To16()
	if ipBytes == nil {
		return "[invalid-ipv6]"
	}

	// Zero out last 8 bytes
	for i := 8; i < 16; i++ {
		ipBytes[i] = 0
	}

	return net.IP(ipBytes).String() + "/64"
}

// hashIP creates a hashed version of an IP (not implemented for simplicity)
func hashIP(ip string) string {
	// Simple hash - in production, use proper crypto hash
	// For now, just return masked placeholder
	return "[hashed]"
}

// isPrivateIP checks if an IP is private/localhost
func isPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	// Check for localhost
	if parsedIP.IsLoopback() {
		return true
	}

	// Check for private IPv4 ranges
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(parsedIP) {
			return true
		}
	}

	// Check for private IPv6
	if parsedIP.To4() == nil {
		// fc00::/7 (Unique Local Addresses)
		if parsedIP[0] == 0xfc || parsedIP[0] == 0xfd {
			return true
		}
		// fe80::/10 (Link-Local)
		if parsedIP[0] == 0xfe && (parsedIP[1]&0xc0) == 0x80 {
			return true
		}
	}

	return false
}

// ShouldMaskField checks if a field name should be masked
func (m *Masker) ShouldMaskField(fieldName string) bool {
	if !m.config.Enabled {
		return false
	}

	lowerName := strings.ToLower(fieldName)
	return m.maskFormFields[lowerName] || m.sensitivePattern.MatchString(lowerName)
}

// GetStats returns masking statistics
func (m *Masker) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":             m.config.Enabled,
		"mask_ip_method":      m.config.MaskIPMethod,
		"mask_ipv6_method":    m.config.MaskIPv6Method,
		"preserve_localhost":  m.config.PreserveLocalhost,
		"strip_headers_count": len(m.stripHeaders),
		"mask_params_count":   len(m.maskQueryParams),
		"mask_fields_count":   len(m.maskFormFields),
	}
}
