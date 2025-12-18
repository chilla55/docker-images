package waf

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Config holds WAF configuration
type Config struct {
	Enabled      bool
	Sensitivity  string   // low, medium, high
	BlockMode    bool     // true = block, false = log only
	CheckPath    bool     // Check URL path
	CheckHeaders bool     // Check HTTP headers
	CheckQuery   bool     // Check query parameters
	CheckBody    bool     // Check request body
	MaxBodySize  int64    // Max body size to inspect (bytes)
	Whitelist    []string // Whitelisted IPs/CIDRs
	CustomRules  []Rule   // Custom detection rules
}

// Rule represents a custom WAF rule
type Rule struct {
	Name        string
	Pattern     *regexp.Regexp
	Description string
	Severity    string // low, medium, high, critical
}

// WAF implements Web Application Firewall
type WAF struct {
	config    Config
	db        Database
	rules     []Rule
	whitelist map[string]bool
}

// Database interface for logging blocks
type Database interface {
	LogWAFBlock(ip, route, attackType, payload, userAgent string) error
}

// AttackType constants
const (
	AttackSQLInjection  = "sql_injection"
	AttackXSS           = "xss"
	AttackPathTraversal = "path_traversal"
	AttackCommandInj    = "command_injection"
	AttackLDAPInjection = "ldap_injection"
	AttackXMLInjection  = "xml_injection"
)

// NewWAF creates a new WAF instance
func NewWAF(config Config, db Database) *WAF {
	if config.MaxBodySize == 0 {
		config.MaxBodySize = 1024 * 1024 // 1 MB default
	}

	waf := &WAF{
		config:    config,
		db:        db,
		whitelist: make(map[string]bool),
	}

	// Initialize built-in rules
	waf.initRules()

	// Add custom rules
	waf.rules = append(waf.rules, config.CustomRules...)

	// Initialize whitelist
	for _, ip := range config.Whitelist {
		waf.whitelist[ip] = true
	}

	return waf
}

// initRules initializes built-in detection rules
func (w *WAF) initRules() {
	w.rules = []Rule{
		// SQL Injection patterns
		{
			Name:        "sql_union",
			Pattern:     regexp.MustCompile(`(?i)(union\s+select|union\s+all\s+select)`),
			Description: "SQL UNION injection attempt",
			Severity:    "high",
		},
		{
			Name:        "sql_comment",
			Pattern:     regexp.MustCompile(`(?i)(--|\#|\/\*|\*\/)`),
			Description: "SQL comment injection",
			Severity:    "medium",
		},
		{
			Name:        "sql_keywords",
			Pattern:     regexp.MustCompile(`(?i)(;|\s)(drop|delete|update|insert|alter|create|exec|execute)\s+(table|database|index|view)`),
			Description: "Dangerous SQL keywords",
			Severity:    "critical",
		},
		{
			Name:        "sql_always_true",
			Pattern:     regexp.MustCompile(`(?i)('|\"|;)?\s*(or|and)\s+('|\")?\s*\d+\s*=\s*\d+`),
			Description: "SQL always-true condition (1=1)",
			Severity:    "high",
		},
		{
			Name:        "sql_sleep",
			Pattern:     regexp.MustCompile(`(?i)(sleep|benchmark|waitfor\s+delay)`),
			Description: "SQL time-based injection",
			Severity:    "high",
		},

		// XSS patterns
		{
			Name:        "xss_script_tag",
			Pattern:     regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`),
			Description: "Script tag injection",
			Severity:    "high",
		},
		{
			Name:        "xss_event_handler",
			Pattern:     regexp.MustCompile(`(?i)on(load|error|click|mouse|focus|blur|change|submit)\s*=`),
			Description: "Event handler injection",
			Severity:    "high",
		},
		{
			Name:        "xss_javascript_protocol",
			Pattern:     regexp.MustCompile(`(?i)javascript\s*:`),
			Description: "JavaScript protocol handler",
			Severity:    "medium",
		},
		{
			Name:        "xss_iframe",
			Pattern:     regexp.MustCompile(`(?i)<iframe[^>]*>`),
			Description: "Iframe injection",
			Severity:    "medium",
		},
		{
			Name:        "xss_object_embed",
			Pattern:     regexp.MustCompile(`(?i)<(object|embed|applet)[^>]*>`),
			Description: "Object/Embed tag injection",
			Severity:    "medium",
		},

		// Path Traversal patterns
		{
			Name:        "path_traversal_basic",
			Pattern:     regexp.MustCompile(`\.\.(/|\\)`),
			Description: "Path traversal attempt (../)",
			Severity:    "high",
		},
		{
			Name:        "path_traversal_encoded",
			Pattern:     regexp.MustCompile(`(%2e%2e|%252e%252e|\.\.%2f|\.\.%5c)`),
			Description: "Encoded path traversal",
			Severity:    "high",
		},
		{
			Name:        "path_traversal_absolute",
			Pattern:     regexp.MustCompile(`(?i)(^|[^a-z0-9])(/etc/passwd|/etc/shadow|c:\\windows\\system32)`),
			Description: "Absolute path to sensitive files",
			Severity:    "critical",
		},

		// Command Injection patterns
		{
			Name:        "command_injection",
			Pattern:     regexp.MustCompile(`[;&|]\s*(ls|cat|wget|curl|nc|bash|sh|cmd|powershell)`),
			Description: "Shell command injection",
			Severity:    "critical",
		},
		{
			Name:        "command_backticks",
			Pattern:     regexp.MustCompile("`[^`]+`"),
			Description: "Backtick command execution",
			Severity:    "high",
		},

		// LDAP Injection patterns
		{
			Name:        "ldap_injection",
			Pattern:     regexp.MustCompile(`\*\)|\(\|`),
			Description: "LDAP filter injection",
			Severity:    "high",
		},

		// XML Injection patterns
		{
			Name:        "xml_entity",
			Pattern:     regexp.MustCompile(`(?i)<!entity`),
			Description: "XML entity injection (XXE)",
			Severity:    "critical",
		},
	}
}

// Middleware returns HTTP middleware for WAF protection
func (w *WAF) Middleware(route string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			if !w.config.Enabled {
				next.ServeHTTP(rw, r)
				return
			}

			ip := extractIP(r)

			// Check whitelist
			if w.whitelist[ip] {
				next.ServeHTTP(rw, r)
				return
			}

			// Check for attacks
			if blocked, attackType, payload := w.checkRequest(r, route); blocked {
				// Log to database
				if w.db != nil {
					w.db.LogWAFBlock(ip, route, attackType, payload, r.UserAgent())
				}

				log.Warn().
					Str("ip", ip).
					Str("route", route).
					Str("attack_type", attackType).
					Str("payload", truncate(payload, 100)).
					Msg("WAF blocked attack")

				if w.config.BlockMode {
					http.Error(rw, "Forbidden", http.StatusForbidden)
					return
				} else {
					log.Info().Msg("WAF in log-only mode, allowing request")
				}
			}

			next.ServeHTTP(rw, r)
		})
	}
}

// checkRequest checks a request for malicious patterns
func (w *WAF) checkRequest(r *http.Request, route string) (blocked bool, attackType string, payload string) {
	// Check URL path
	if w.config.CheckPath {
		if matched, aType, p := w.checkString(r.URL.Path); matched {
			return true, aType, p
		}
	}

	// Check query parameters
	if w.config.CheckQuery {
		for key, values := range r.URL.Query() {
			for _, value := range values {
				if matched, aType, p := w.checkString(value); matched {
					return true, aType, fmt.Sprintf("%s=%s", key, p)
				}
			}
		}
	}

	// Check headers
	if w.config.CheckHeaders {
		for key, values := range r.Header {
			// Skip common safe headers
			if isSafeHeader(key) {
				continue
			}
			for _, value := range values {
				if matched, aType, p := w.checkString(value); matched {
					return true, aType, fmt.Sprintf("%s: %s", key, p)
				}
			}
		}
	}

	// Check request body (for POST/PUT)
	if w.config.CheckBody && (r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH") {
		if r.ContentLength > 0 && r.ContentLength <= w.config.MaxBodySize {
			body := make([]byte, r.ContentLength)
			n, err := r.Body.Read(body)
			if err == nil && n > 0 {
				if matched, aType, p := w.checkString(string(body[:n])); matched {
					return true, aType, p
				}
			}
		}
	}

	return false, "", ""
}

// checkString checks a string against all WAF rules
func (w *WAF) checkString(input string) (matched bool, attackType string, payload string) {
	// Decode URL encoding to catch encoded attacks
	decoded, err := url.QueryUnescape(input)
	if err != nil {
		decoded = input
	}

	// Check both original and decoded
	for _, str := range []string{input, decoded} {
		for _, rule := range w.rules {
			if rule.Pattern.MatchString(str) {
				// Determine attack type based on rule name
				aType := w.categorizeRule(rule.Name)

				// Extract matched portion
				match := rule.Pattern.FindString(str)

				return true, aType, match
			}
		}
	}

	return false, "", ""
}

// categorizeRule determines attack type from rule name
func (w *WAF) categorizeRule(ruleName string) string {
	if strings.HasPrefix(ruleName, "sql_") {
		return AttackSQLInjection
	}
	if strings.HasPrefix(ruleName, "xss_") {
		return AttackXSS
	}
	if strings.HasPrefix(ruleName, "path_") {
		return AttackPathTraversal
	}
	if strings.HasPrefix(ruleName, "command_") {
		return AttackCommandInj
	}
	if strings.HasPrefix(ruleName, "ldap_") {
		return AttackLDAPInjection
	}
	if strings.HasPrefix(ruleName, "xml_") {
		return AttackXMLInjection
	}
	return "unknown"
}

// GetStats returns WAF statistics
func (w *WAF) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":        w.config.Enabled,
		"block_mode":     w.config.BlockMode,
		"sensitivity":    w.config.Sensitivity,
		"rules_count":    len(w.rules),
		"whitelist_size": len(w.whitelist),
		"checks": map[string]bool{
			"path":    w.config.CheckPath,
			"headers": w.config.CheckHeaders,
			"query":   w.config.CheckQuery,
			"body":    w.config.CheckBody,
		},
	}
}

// isSafeHeader checks if a header is safe to skip
func isSafeHeader(header string) bool {
	safeHeaders := map[string]bool{
		"accept":            true,
		"accept-encoding":   true,
		"accept-language":   true,
		"cache-control":     true,
		"connection":        true,
		"content-length":    true,
		"content-type":      true,
		"host":              true,
		"user-agent":        true,
		"x-forwarded-for":   true,
		"x-forwarded-host":  true,
		"x-forwarded-proto": true,
		"x-real-ip":         true,
		"x-request-id":      true,
	}
	return safeHeaders[strings.ToLower(header)]
}

// extractIP extracts the client IP from the request
func extractIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// truncate truncates a string to maxLen
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Start starts background tasks (currently none needed)
func (w *WAF) Start(ctx context.Context) {
	// Future: Could add background tasks for:
	// - Updating threat intelligence feeds
	// - Analyzing attack patterns
	// - Auto-tuning sensitivity
	log.Info().Bool("enabled", w.config.Enabled).Msg("WAF initialized")
}
