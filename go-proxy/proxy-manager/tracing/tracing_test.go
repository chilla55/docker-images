package tracing

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateRequestID(t *testing.T) {
	id := GenerateRequestID()
	if id == "" {
		t.Fatal("Generated request ID should not be empty")
	}
}

func TestSetAndGetRequestID(t *testing.T) {
	ctx := SetRequestID(nil, "abc123")
	if GetRequestID(ctx) != "abc123" {
		t.Error("GetRequestID should return the set ID")
	}
}

func TestInjectRequestID(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	id, req2 := InjectRequestID(rec, req)
	if id == "" {
		t.Fatal("InjectRequestID should return an ID")
	}
	if req2 == nil {
		t.Fatal("InjectRequestID should return a request")
	}

	// Check response header
	if rec.Header().Get("X-Request-ID") != id {
		t.Error("Response header should contain request ID")
	}
}

func TestExtractAndAddRequestIDToHeader(t *testing.T) {
	header := http.Header{}

	AddRequestIDToHeader(header, "abc123")
	if ExtractRequestIDFromHeader(header) != "abc123" {
		t.Error("Header should contain added request ID")
	}
}
