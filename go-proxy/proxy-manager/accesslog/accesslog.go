package accesslog

import (
	"container/ring"
	"sync"
	"time"

	"github.com/chilla55/proxy-manager/database"
	"github.com/rs/zerolog/log"
)

// Logger handles request/error logging with ring buffer and database persistence
type Logger struct {
	db         Database
	ringBuffer *ring.Ring
	ringMutex  sync.RWMutex
	bufferSize int
	enabled    bool
}

// Database interface for access log persistence
type Database interface {
	LogAccessRequest(entry database.AccessLogEntry) error
	GetRecentRequests(limit int) ([]database.AccessLogEntry, error)
	GetRequestsByRoute(route string, limit int) ([]database.AccessLogEntry, error)
	GetErrorRequests(limit int) ([]database.AccessLogEntry, error)
}

// AccessLogEntry is an alias for database.AccessLogEntry
type AccessLogEntry = database.AccessLogEntry

// NewLogger creates a new access logger
func NewLogger(db Database, bufferSize int) *Logger {
	if bufferSize <= 0 {
		bufferSize = 1000 // Default: last 1000 requests
	}

	return &Logger{
		db:         db,
		ringBuffer: ring.New(bufferSize),
		bufferSize: bufferSize,
		enabled:    true,
	}
}

// LogRequest logs an HTTP request to both ring buffer and database
func (l *Logger) LogRequest(entry AccessLogEntry) {
	if !l.enabled {
		return
	}

	// Set timestamp if not set
	if entry.Timestamp == 0 {
		entry.Timestamp = time.Now().Unix()
	}

	// Store in ring buffer (in-memory)
	l.ringMutex.Lock()
	l.ringBuffer.Value = entry
	l.ringBuffer = l.ringBuffer.Next()
	l.ringMutex.Unlock()

	// Store in database (async, non-blocking)
	go func() {
		if err := l.db.LogAccessRequest(entry); err != nil {
			log.Error().
				Err(err).
				Str("domain", entry.Domain).
				Str("path", entry.Path).
				Msg("Failed to log access request to database")
		}
	}()

	// Log errors to stderr for immediate visibility
	if entry.Status >= 400 {
		logLevel := log.Warn()
		if entry.Status >= 500 {
			logLevel = log.Error()
		}

		logLevel.
			Str("domain", entry.Domain).
			Str("method", entry.Method).
			Str("path", entry.Path).
			Int("status", entry.Status).
			Int64("response_ms", entry.ResponseTimeMs).
			Str("client_ip", entry.ClientIP).
			Str("error", entry.Error).
			Msg("Request error")
	}
}

// GetRecentRequests returns the last N requests from ring buffer
func (l *Logger) GetRecentRequests(limit int) []AccessLogEntry {
	if limit <= 0 || limit > l.bufferSize {
		limit = l.bufferSize
	}

	l.ringMutex.RLock()
	defer l.ringMutex.RUnlock()

	var entries []AccessLogEntry
	count := 0

	// Walk the ring buffer backwards
	l.ringBuffer.Do(func(value interface{}) {
		if count >= limit {
			return
		}
		if value != nil {
			if entry, ok := value.(AccessLogEntry); ok {
				entries = append(entries, entry)
				count++
			}
		}
	})

	// Reverse to get chronological order (newest first)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	return entries
}

// GetRecentErrors returns recent error requests from ring buffer
func (l *Logger) GetRecentErrors(limit int) []AccessLogEntry {
	l.ringMutex.RLock()
	defer l.ringMutex.RUnlock()

	var errors []AccessLogEntry
	count := 0

	l.ringBuffer.Do(func(value interface{}) {
		if count >= limit {
			return
		}
		if value != nil {
			if entry, ok := value.(AccessLogEntry); ok && entry.Status >= 400 {
				errors = append(errors, entry)
				count++
			}
		}
	})

	// Reverse to get chronological order (newest first)
	for i, j := 0, len(errors)-1; i < j; i, j = i+1, j-1 {
		errors[i], errors[j] = errors[j], errors[i]
	}

	return errors
}

// GetStats returns access log statistics
func (l *Logger) GetStats() LogStats {
	l.ringMutex.RLock()
	defer l.ringMutex.RUnlock()

	stats := LogStats{
		BufferSize: l.bufferSize,
	}

	var totalResponseTime int64
	statusCounts := make(map[int]int)
	methodCounts := make(map[string]int)

	l.ringBuffer.Do(func(value interface{}) {
		if value != nil {
			if entry, ok := value.(AccessLogEntry); ok {
				stats.TotalEntries++
				totalResponseTime += entry.ResponseTimeMs

				if entry.Status >= 400 {
					stats.ErrorCount++
				}

				statusCounts[entry.Status]++
				methodCounts[entry.Method]++
			}
		}
	})

	if stats.TotalEntries > 0 {
		stats.AverageResponseTimeMs = float64(totalResponseTime) / float64(stats.TotalEntries)
		stats.ErrorRate = float64(stats.ErrorCount) / float64(stats.TotalEntries) * 100
	}

	stats.StatusCounts = statusCounts
	stats.MethodCounts = methodCounts

	return stats
}

// LogStats represents access log buffer statistics
type LogStats struct {
	BufferSize            int            `json:"buffer_size"`
	TotalEntries          int            `json:"total_entries"`
	ErrorCount            int            `json:"error_count"`
	ErrorRate             float64        `json:"error_rate_percent"`
	AverageResponseTimeMs float64        `json:"average_response_time_ms"`
	StatusCounts          map[int]int    `json:"status_counts"`
	MethodCounts          map[string]int `json:"method_counts"`
}

// Enable enables access logging
func (l *Logger) Enable() {
	l.enabled = true
	log.Info().Msg("Access logging enabled")
}

// Disable disables access logging
func (l *Logger) Disable() {
	l.enabled = false
	log.Info().Msg("Access logging disabled")
}

// IsEnabled returns whether access logging is enabled
func (l *Logger) IsEnabled() bool {
	return l.enabled
}

// Clear clears the ring buffer
func (l *Logger) Clear() {
	l.ringMutex.Lock()
	defer l.ringMutex.Unlock()

	l.ringBuffer = ring.New(l.bufferSize)
	log.Info().Msg("Access log ring buffer cleared")
}
