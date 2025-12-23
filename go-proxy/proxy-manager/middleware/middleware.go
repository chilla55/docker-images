package middleware

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// TimeoutConfig holds timeout settings
type TimeoutConfig struct {
	Connect time.Duration
	Read    time.Duration
	Write   time.Duration
	Idle    time.Duration
}

// LimitConfig holds size limit settings
type LimitConfig struct {
	MaxRequestBody  int64
	MaxResponseBody int64
}

// RequestID middleware adds a unique request ID to each request
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add to request context
		ctx := context.WithValue(r.Context(), "request_id", requestID)
		r = r.WithContext(ctx)

		// Add to response headers
		w.Header().Set("X-Request-ID", requestID)

		next.ServeHTTP(w, r)
	})
}

// Timeout middleware enforces request timeout
func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)

			done := make(chan struct{})
			go func() {
				next.ServeHTTP(w, r)
				close(done)
			}()

			select {
			case <-done:
				// Request completed
			case <-ctx.Done():
				// Timeout occurred
				if ctx.Err() == context.DeadlineExceeded {
					http.Error(w, "504 Gateway Timeout", http.StatusGatewayTimeout)
					log.Warn().
						Str("request_id", getRequestID(r)).
						Str("path", r.URL.Path).
						Msg("Request timeout")
				}
			}
		})
	}
}

// LimitRequestBody enforces maximum request body size
func LimitRequestBody(maxSize int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxSize {
				http.Error(w, "413 Payload Too Large", http.StatusRequestEntityTooLarge)
				log.Warn().
					Str("request_id", getRequestID(r)).
					Int64("content_length", r.ContentLength).
					Int64("max_size", maxSize).
					Msg("Request body too large")
				return
			}

			// Wrap body reader with limit
			r.Body = http.MaxBytesReader(w, r.Body, maxSize)

			next.ServeHTTP(w, r)
		})
	}
}

// LimitResponseBody wraps the response writer to limit response size
type limitedResponseWriter struct {
	http.ResponseWriter
	writer   io.Writer
	maxSize  int64
	written  int64
	exceeded bool
}

func (lrw *limitedResponseWriter) Write(b []byte) (int, error) {
	if lrw.exceeded {
		return 0, fmt.Errorf("response size limit exceeded")
	}

	if lrw.written+int64(len(b)) > lrw.maxSize {
		lrw.exceeded = true
		lrw.ResponseWriter.WriteHeader(http.StatusInsufficientStorage)
		return 0, fmt.Errorf("response size limit exceeded")
	}

	n, err := lrw.ResponseWriter.Write(b)
	lrw.written += int64(n)
	return n, err
}

// LimitResponseBodySize enforces maximum response body size
func LimitResponseBodySize(maxSize int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lrw := &limitedResponseWriter{
				ResponseWriter: w,
				maxSize:        maxSize,
			}

			next.ServeHTTP(lrw, r)

			if lrw.exceeded {
				log.Warn().
					Str("request_id", getRequestID(r)).
					Int64("written", lrw.written).
					Int64("max_size", maxSize).
					Msg("Response body too large")
			}
		})
	}
}

// Logger middleware logs all requests
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		log.Info().
			Str("request_id", getRequestID(r)).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Int("status", wrapped.statusCode).
			Dur("duration", duration).
			Int64("bytes", wrapped.bytesWritten).
			Msg("Request completed")
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// SecurityHeaders middleware adds security headers
func SecurityHeaders(headers map[string]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for key, value := range headers {
				w.Header().Set(key, value)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// getRequestID retrieves the request ID from context
func getRequestID(r *http.Request) string {
	if id, ok := r.Context().Value("request_id").(string); ok {
		return id
	}
	return "unknown"
}
