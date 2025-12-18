package errorpage

import (
	"bytes"
	"html/template"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewErrorPageManager(t *testing.T) {
	cfg := ErrorPageConfig{Enabled: true, CustomMap: make(map[int]string)}
	m := New(cfg)
	if !m.IsEnabled() {
		t.Error("Manager should be enabled")
	}
}

func TestDisabledErrorPageManager(t *testing.T) {
	cfg := ErrorPageConfig{Enabled: false}
	m := New(cfg)
	if m.IsEnabled() {
		t.Error("Manager should be disabled")
	}
}

func TestSetTemplateHTML(t *testing.T) {
	cfg := ErrorPageConfig{Enabled: true}
	m := New(cfg)

	html := `<h1>{{.StatusCode}} - {{.StatusText}}</h1><p>{{.Message}}</p>`
	err := m.SetTemplateHTML(404, html)
	if err != nil {
		t.Fatalf("Failed to set template: %v", err)
	}

	m.mu.RLock()
	_, exists := m.templates[404]
	m.mu.RUnlock()

	if !exists {
		t.Error("Template should be registered for 404")
	}
}

func TestRenderErrorCustom(t *testing.T) {
	cfg := ErrorPageConfig{Enabled: true}
	m := New(cfg)

	html := `<h1>{{.StatusCode}} - {{.StatusText}}</h1><p>{{.Message}}</p>`
	m.SetTemplateHTML(404, html)

	data := ErrorData{
		StatusCode: 404,
		Message:    "Page not found",
		Route:      "/api/test",
		RequestID:  "req-123",
		Domain:     "example.com",
		Timestamp:  time.Now(),
	}

	w := httptest.NewRecorder()
	err := m.RenderError(w, 404, data)
	if err != nil {
		t.Fatalf("Failed to render error: %v", err)
	}

	if w.Code != 404 {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("404")) {
		t.Error("Response should contain status code")
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("Page not found")) {
		t.Error("Response should contain message")
	}
}

func TestRenderErrorDefault(t *testing.T) {
	cfg := ErrorPageConfig{Enabled: true}
	m := New(cfg)

	data := ErrorData{
		StatusCode: 502,
		Message:    "Bad Gateway",
		Route:      "/api",
		RequestID:  "req-456",
		Domain:     "api.example.com",
		Path:       "/v1/users",
		Timestamp:  time.Now(),
	}

	w := httptest.NewRecorder()
	err := m.RenderError(w, 502, data)
	if err != nil {
		t.Fatalf("Failed to render default error: %v", err)
	}

	if w.Code != 502 {
		t.Errorf("Expected status 502, got %d", w.Code)
	}

	resp := w.Body.String()
	if !bytes.Contains(w.Body.Bytes(), []byte("502")) {
		t.Error("Response should contain status code")
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("api.example.com")) {
		t.Error("Response should contain domain")
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("req-456")) {
		t.Error("Response should contain request ID")
	}

	_ = resp
}

func TestGetStatusCodeText(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{404, "Not Found"},
		{500, "Internal Server Error"},
		{502, "Bad Gateway"},
		{200, "OK"},
	}

	for _, tt := range tests {
		text := GetStatusCodeText(tt.code)
		if text != tt.expected {
			t.Errorf("Code %d: expected %q, got %q", tt.code, tt.expected, text)
		}
	}
}

func TestTemplateVariables(t *testing.T) {
	cfg := ErrorPageConfig{Enabled: true}
	m := New(cfg)

	tmpl, _ := template.New("test").Parse(`
		Code: {{.StatusCode}}
		Text: {{.StatusText}}
		Msg: {{.Message}}
		Route: {{.Route}}
		ID: {{.RequestID}}
		Domain: {{.Domain}}
	`)
	m.SetTemplate(500, tmpl)

	data := ErrorData{
		StatusCode: 500,
		StatusText: "Internal Server Error",
		Message:    "Database connection failed",
		Route:      "/api/data",
		RequestID:  "req-789",
		Domain:     "db.example.com",
	}

	w := httptest.NewRecorder()
	m.RenderError(w, 500, data)

	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("500")) {
		t.Error("Should contain status code")
	}
	if !bytes.Contains([]byte(body), []byte("Internal Server Error")) {
		t.Error("Should contain status text")
	}
	if !bytes.Contains([]byte(body), []byte("Database connection failed")) {
		t.Error("Should contain message")
	}
}
