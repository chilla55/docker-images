package ratelimit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Config holds rate limiting configuration
type Config struct {
	Enabled          bool
	RequestsPerMin   int           // Max requests per minute
	RequestsPerHour  int           // Max requests per hour
	BurstSize        int           // Burst allowance
	CleanupInterval  time.Duration // How often to clean old entries
	ViolationTimeout time.Duration // How long to block violators
}

// Limiter manages rate limiting using sliding window algorithm
type Limiter struct {
	mu        sync.RWMutex
	windows   map[string]*window // IP -> window
	config    Config
	db        Database
	enabled   bool
	whitelist map[string]bool // Whitelisted IPs
}

// Database interface for storing violations
type Database interface {
	LogRateLimitViolation(ip, route, reason string, requestCount int) error
}

// window tracks request counts in sliding time windows
type window struct {
	minuteWindow []time.Time
	hourWindow   []time.Time
	violations   int
	blockedUntil time.Time
}

// NewLimiter creates a new rate limiter
func NewLimiter(config Config, db Database) *Limiter {
	if config.CleanupInterval == 0 {
		config.CleanupInterval = 5 * time.Minute
	}
	if config.ViolationTimeout == 0 {
		config.ViolationTimeout = 15 * time.Minute
	}
	if config.BurstSize == 0 {
		config.BurstSize = config.RequestsPerMin / 10 // 10% burst
	}

	return &Limiter{
		windows:   make(map[string]*window),
		config:    config,
		db:        db,
		enabled:   config.Enabled,
		whitelist: make(map[string]bool),
	}
}

// Start begins background cleanup
func (l *Limiter) Start(ctx context.Context) {
	if !l.enabled {
		return
	}

	ticker := time.NewTicker(l.config.CleanupInterval)
	defer ticker.Stop()

	log.Info().Msg("Rate limiter started")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Rate limiter stopped")
			return
		case <-ticker.C:
			l.cleanup()
		}
	}
}

// Middleware returns HTTP middleware for rate limiting
func (l *Limiter) Middleware(route string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !l.enabled {
				next.ServeHTTP(w, r)
				return
			}

			ip := extractIP(r)

			// Check whitelist
			if l.isWhitelisted(ip) {
				next.ServeHTTP(w, r)
				return
			}

			// Check rate limit
			allowed, reason := l.Allow(ip, route)
			if !allowed {
				w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d/min, %d/hour", l.config.RequestsPerMin, l.config.RequestsPerHour))
				w.Header().Set("Retry-After", "900") // 15 minutes
				http.Error(w, "Too Many Requests: "+reason, http.StatusTooManyRequests)

				log.Warn().
					Str("ip", ip).
					Str("route", route).
					Str("reason", reason).
					Msg("Rate limit exceeded")

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// Allow checks if a request from the given IP is allowed
func (l *Limiter) Allow(ip, route string) (bool, string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// Get or create window
	w, exists := l.windows[ip]
	if !exists {
		w = &window{
			minuteWindow: make([]time.Time, 0, l.config.RequestsPerMin),
			hourWindow:   make([]time.Time, 0, l.config.RequestsPerHour),
		}
		l.windows[ip] = w
	}

	// Check if IP is blocked
	if now.Before(w.blockedUntil) {
		remaining := w.blockedUntil.Sub(now)
		return false, fmt.Sprintf("blocked for %v due to repeated violations", remaining.Round(time.Second))
	}

	// Clean expired entries from windows
	w.minuteWindow = filterExpired(w.minuteWindow, now.Add(-time.Minute))
	w.hourWindow = filterExpired(w.hourWindow, now.Add(-time.Hour))

	// Check minute limit
	minuteCount := len(w.minuteWindow)
	if minuteCount >= l.config.RequestsPerMin+l.config.BurstSize {
		w.violations++
		if w.violations >= 3 {
			w.blockedUntil = now.Add(l.config.ViolationTimeout)
			log.Warn().
				Str("ip", ip).
				Str("route", route).
				Int("violations", w.violations).
				Time("blocked_until", w.blockedUntil).
				Msg("IP blocked due to repeated violations")
		}

		// Log violation to database
		if l.db != nil {
			l.db.LogRateLimitViolation(ip, route, "minute_limit_exceeded", minuteCount)
		}

		return false, fmt.Sprintf("exceeded %d requests per minute (current: %d)", l.config.RequestsPerMin, minuteCount)
	}

	// Check hour limit
	hourCount := len(w.hourWindow)
	if hourCount >= l.config.RequestsPerHour {
		w.violations++
		if w.violations >= 3 {
			w.blockedUntil = now.Add(l.config.ViolationTimeout)
		}

		// Log violation to database
		if l.db != nil {
			l.db.LogRateLimitViolation(ip, route, "hour_limit_exceeded", hourCount)
		}

		return false, fmt.Sprintf("exceeded %d requests per hour (current: %d)", l.config.RequestsPerHour, hourCount)
	}

	// Add request to windows
	w.minuteWindow = append(w.minuteWindow, now)
	w.hourWindow = append(w.hourWindow, now)

	// Reset violation counter on successful request
	if w.violations > 0 {
		w.violations--
	}

	return true, ""
}

// AddWhitelist adds an IP or CIDR to whitelist
func (l *Limiter) AddWhitelist(ipOrCIDR string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if it's a CIDR
	if strings.Contains(ipOrCIDR, "/") {
		_, _, err := net.ParseCIDR(ipOrCIDR)
		if err != nil {
			return fmt.Errorf("invalid CIDR: %w", err)
		}
	} else {
		// Validate IP
		if net.ParseIP(ipOrCIDR) == nil {
			return fmt.Errorf("invalid IP address: %s", ipOrCIDR)
		}
	}

	l.whitelist[ipOrCIDR] = true
	log.Info().Str("ip_or_cidr", ipOrCIDR).Msg("Added to rate limit whitelist")
	return nil
}

// isWhitelisted checks if an IP is whitelisted
func (l *Limiter) isWhitelisted(ip string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()

	// Direct IP match
	if l.whitelist[ip] {
		return true
	}

	// Check CIDR ranges
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for cidr := range l.whitelist {
		if !strings.Contains(cidr, "/") {
			continue
		}

		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}

		if network.Contains(parsedIP) {
			return true
		}
	}

	return false
}

// cleanup removes old entries and expired blocks
func (l *Limiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)

	for ip, w := range l.windows {
		// Clean windows
		w.minuteWindow = filterExpired(w.minuteWindow, now.Add(-time.Minute))
		w.hourWindow = filterExpired(w.hourWindow, oneHourAgo)

		// Remove window if no recent activity and not blocked
		if len(w.hourWindow) == 0 && now.After(w.blockedUntil) {
			delete(l.windows, ip)
		}
	}

	log.Debug().Int("active_windows", len(l.windows)).Msg("Rate limiter cleanup completed")
}

// GetStats returns current rate limiting statistics
func (l *Limiter) GetStats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	blockedCount := 0
	for _, w := range l.windows {
		if time.Now().Before(w.blockedUntil) {
			blockedCount++
		}
	}

	return map[string]interface{}{
		"active_windows": len(l.windows),
		"blocked_ips":    blockedCount,
		"whitelist_size": len(l.whitelist),
		"config": map[string]interface{}{
			"requests_per_min":  l.config.RequestsPerMin,
			"requests_per_hour": l.config.RequestsPerHour,
			"burst_size":        l.config.BurstSize,
		},
	}
}

// filterExpired removes timestamps older than the cutoff
func filterExpired(timestamps []time.Time, cutoff time.Time) []time.Time {
	result := make([]time.Time, 0, len(timestamps))
	for _, t := range timestamps {
		if t.After(cutoff) {
			result = append(result, t)
		}
	}
	return result
}

// extractIP extracts the client IP from the request
func extractIP(r *http.Request) string {
	// Check X-Real-IP first (from our proxy)
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}

	// Check X-Forwarded-For
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	// Fall back to RemoteAddr
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}
