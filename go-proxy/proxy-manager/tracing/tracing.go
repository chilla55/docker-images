package tracing

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// ContextKey is a custom type for context keys to avoid collisions
type ContextKey string

const (
	// RequestIDKey is the context key for request IDs
	RequestIDKey ContextKey = "request_id"
)

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// GetRequestIDFromRequest retrieves the request ID from HTTP request
func GetRequestIDFromRequest(r *http.Request) string {
	return GetRequestID(r.Context())
}

// SetRequestID sets the request ID in context
func SetRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GenerateRequestID generates a new UUID for request tracing
func GenerateRequestID() string {
	return uuid.New().String()
}

// InjectRequestID adds request ID to HTTP request and response
func InjectRequestID(w http.ResponseWriter, r *http.Request) (string, *http.Request) {
	// Check if request already has an ID (from upstream proxy)
	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = GenerateRequestID()
	}

	// Add to request context
	ctx := SetRequestID(r.Context(), requestID)
	r = r.WithContext(ctx)

	// Add to response headers
	w.Header().Set("X-Request-ID", requestID)

	return requestID, r
}

// ExtractRequestIDFromHeader extracts request ID from HTTP headers
func ExtractRequestIDFromHeader(header http.Header) string {
	return header.Get("X-Request-ID")
}

// AddRequestIDToHeader adds request ID to HTTP headers
func AddRequestIDToHeader(header http.Header, requestID string) {
	header.Set("X-Request-ID", requestID)
}
