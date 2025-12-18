package health

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/chilla55/proxy-manager/database"
	"github.com/rs/zerolog/log"
)

// Status represents service health status
type Status string

const (
	StatusHealthy  Status = "healthy"  // 90%+ success rate
	StatusDegraded Status = "degraded" // 50-90% success rate
	StatusDown     Status = "down"     // <50% success rate
	StatusUnknown  Status = "unknown"  // No data yet
)

// Checker performs health checks on backend services
type Checker struct {
	services map[string]*ServiceHealth
	db       Database
	mu       sync.RWMutex
}

// Database interface for health check persistence
type Database interface {
	RecordHealthCheck(service, url string, success bool, duration time.Duration, statusCode int, error string) error
	GetHealthCheckHistory(service string, limit int) ([]database.HealthCheckResult, error)
}

// ServiceHealth tracks health for a single service
type ServiceHealth struct {
	Name           string
	URL            string
	Interval       time.Duration
	Timeout        time.Duration
	ExpectedStatus int
	SuccessCount   int
	FailureCount   int
	TotalChecks    int
	LastCheck      time.Time
	LastSuccess    time.Time
	LastFailure    time.Time
	LastError      string
	Status         Status
	ResponseTime   time.Duration
	mu             sync.RWMutex
}

// HealthCheckResult is an alias for database.HealthCheckResult
type HealthCheckResult = database.HealthCheckResult

// NewChecker creates a new health checker
func NewChecker(db Database) *Checker {
	return &Checker{
		services: make(map[string]*ServiceHealth),
		db:       db,
	}
}

// AddService adds a service to monitor
func (c *Checker) AddService(name, url string, interval, timeout time.Duration, expectedStatus int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.services[name] = &ServiceHealth{
		Name:           name,
		URL:            url,
		Interval:       interval,
		Timeout:        timeout,
		ExpectedStatus: expectedStatus,
		Status:         StatusUnknown,
	}

	log.Info().
		Str("service", name).
		Str("url", url).
		Dur("interval", interval).
		Msg("Added service for health monitoring")
}

// Start starts health checking for all services
func (c *Checker) Start(ctx context.Context) {
	c.mu.RLock()
	services := make([]*ServiceHealth, 0, len(c.services))
	for _, svc := range c.services {
		services = append(services, svc)
	}
	c.mu.RUnlock()

	// Start a goroutine for each service
	for _, svc := range services {
		go c.monitorService(ctx, svc)
	}

	log.Info().
		Int("services", len(services)).
		Msg("Health checker started")
}

// monitorService continuously monitors a single service
func (c *Checker) monitorService(ctx context.Context, svc *ServiceHealth) {
	// Perform initial check immediately
	c.check(svc)

	ticker := time.NewTicker(svc.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().
				Str("service", svc.Name).
				Msg("Stopping health checks")
			return
		case <-ticker.C:
			c.check(svc)
		}
	}
}

// check performs a single health check
func (c *Checker) check(svc *ServiceHealth) {
	startTime := time.Now()

	client := &http.Client{
		Timeout: svc.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	resp, err := client.Get(svc.URL)
	duration := time.Since(startTime)

	success := false
	statusCode := 0
	errorMsg := ""

	if err != nil {
		errorMsg = err.Error()
		log.Debug().
			Str("service", svc.Name).
			Str("url", svc.URL).
			Err(err).
			Msg("Health check failed")
	} else {
		defer resp.Body.Close()
		statusCode = resp.StatusCode

		// Check if status code matches expected
		if svc.ExpectedStatus == 0 || statusCode == svc.ExpectedStatus {
			success = true
		} else {
			errorMsg = fmt.Sprintf("unexpected status code: %d (expected %d)", statusCode, svc.ExpectedStatus)
		}

		log.Debug().
			Str("service", svc.Name).
			Int("status", statusCode).
			Dur("duration", duration).
			Bool("success", success).
			Msg("Health check completed")
	}

	// Update service health
	svc.mu.Lock()
	svc.LastCheck = startTime
	svc.ResponseTime = duration
	svc.TotalChecks++

	if success {
		svc.SuccessCount++
		svc.LastSuccess = startTime
		svc.LastError = ""
	} else {
		svc.FailureCount++
		svc.LastFailure = startTime
		svc.LastError = errorMsg
	}

	// Calculate status based on success rate
	svc.Status = c.calculateStatus(svc.SuccessCount, svc.TotalChecks)
	svc.mu.Unlock()

	// Log status changes
	if svc.Status == StatusDown && success == false {
		log.Warn().
			Str("service", svc.Name).
			Str("status", string(svc.Status)).
			Str("error", errorMsg).
			Msg("Service health degraded")
	} else if svc.Status == StatusHealthy && success == true && svc.FailureCount > 0 {
		log.Info().
			Str("service", svc.Name).
			Msg("Service recovered")
	}

	// Store in database
	if c.db != nil {
		err := c.db.RecordHealthCheck(svc.Name, svc.URL, success, duration, statusCode, errorMsg)
		if err != nil {
			log.Error().
				Err(err).
				Str("service", svc.Name).
				Msg("Failed to record health check")
		}
	}
}

// calculateStatus determines health status based on success rate
func (c *Checker) calculateStatus(successCount, totalChecks int) Status {
	if totalChecks == 0 {
		return StatusUnknown
	}

	successRate := float64(successCount) / float64(totalChecks) * 100

	if successRate >= 90 {
		return StatusHealthy
	} else if successRate >= 50 {
		return StatusDegraded
	}
	return StatusDown
}

// GetStatus returns the current status of a service
func (c *Checker) GetStatus(name string) (*ServiceHealth, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	svc, ok := c.services[name]
	if !ok {
		return nil, fmt.Errorf("service not found: %s", name)
	}

	// Return a copy to avoid race conditions
	svc.mu.RLock()
	defer svc.mu.RUnlock()

	return &ServiceHealth{
		Name:         svc.Name,
		URL:          svc.URL,
		Status:       svc.Status,
		SuccessCount: svc.SuccessCount,
		FailureCount: svc.FailureCount,
		TotalChecks:  svc.TotalChecks,
		LastCheck:    svc.LastCheck,
		LastSuccess:  svc.LastSuccess,
		LastFailure:  svc.LastFailure,
		LastError:    svc.LastError,
		ResponseTime: svc.ResponseTime,
	}, nil
}

// GetAllStatuses returns status for all services
func (c *Checker) GetAllStatuses() map[string]ServiceHealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	statuses := make(map[string]ServiceHealthStatus, len(c.services))

	for name, svc := range c.services {
		svc.mu.RLock()

		successRate := float64(0)
		if svc.TotalChecks > 0 {
			successRate = float64(svc.SuccessCount) / float64(svc.TotalChecks) * 100
		}

		statuses[name] = ServiceHealthStatus{
			Status:         string(svc.Status),
			SuccessRate:    successRate,
			TotalChecks:    svc.TotalChecks,
			SuccessCount:   svc.SuccessCount,
			FailureCount:   svc.FailureCount,
			LastCheck:      svc.LastCheck.Unix(),
			LastSuccess:    svc.LastSuccess.Unix(),
			LastFailure:    svc.LastFailure.Unix(),
			LastError:      svc.LastError,
			ResponseTimeMs: svc.ResponseTime.Milliseconds(),
		}

		svc.mu.RUnlock()
	}

	return statuses
}

// ServiceHealthStatus represents JSON-serializable health status
type ServiceHealthStatus struct {
	Status         string  `json:"status"`
	SuccessRate    float64 `json:"success_rate_percent"`
	TotalChecks    int     `json:"total_checks"`
	SuccessCount   int     `json:"success_count"`
	FailureCount   int     `json:"failure_count"`
	LastCheck      int64   `json:"last_check"`
	LastSuccess    int64   `json:"last_success"`
	LastFailure    int64   `json:"last_failure"`
	LastError      string  `json:"last_error,omitempty"`
	ResponseTimeMs int64   `json:"response_time_ms"`
}

// IsHealthy returns true if all services are healthy
func (c *Checker) IsHealthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, svc := range c.services {
		svc.mu.RLock()
		status := svc.Status
		svc.mu.RUnlock()

		if status == StatusDown {
			return false
		}
	}

	return true
}

// GetUnhealthyServices returns list of unhealthy services
func (c *Checker) GetUnhealthyServices() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var unhealthy []string

	for name, svc := range c.services {
		svc.mu.RLock()
		status := svc.Status
		svc.mu.RUnlock()

		if status == StatusDown || status == StatusDegraded {
			unhealthy = append(unhealthy, name)
		}
	}

	return unhealthy
}
