package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleDashboard(t *testing.T) {
	d := New(nil, nil, nil, nil, true)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	d.handleDashboard(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected HTML content type, got %s", contentType)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("HTML body should not be empty")
	}
}

func TestHandleDashboardAPI(t *testing.T) {
	d := New(nil, nil, nil, nil, true)

	req := httptest.NewRequest("GET", "/api/dashboard", nil)
	w := httptest.NewRecorder()

	d.handleDashboardAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var data DashboardData
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	if data.SystemStats == nil {
		t.Error("SystemStats should not be nil")
	}

	if data.Routes == nil {
		t.Error("Routes should not be nil")
	}
}

func TestHandleStats(t *testing.T) {
	d := New(nil, nil, nil, nil, true)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()

	d.handleStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var stats SystemStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}

	if stats.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestHandleRoutes(t *testing.T) {
	d := New(nil, nil, nil, nil, true)

	req := httptest.NewRequest("GET", "/api/routes", nil)
	w := httptest.NewRecorder()

	d.handleRoutes(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var routes []RouteStatus
	if err := json.NewDecoder(w.Body).Decode(&routes); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}
}

func TestHandleCertificates(t *testing.T) {
	d := New(nil, nil, nil, nil, true)

	req := httptest.NewRequest("GET", "/api/certificates", nil)
	w := httptest.NewRecorder()

	d.handleCertificates(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var certs []CertStatus
	if err := json.NewDecoder(w.Body).Decode(&certs); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}
}

func TestHandleErrors(t *testing.T) {
	d := New(nil, nil, nil, nil, true)

	req := httptest.NewRequest("GET", "/api/errors", nil)
	w := httptest.NewRecorder()

	d.handleErrors(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var errors []GroupedError
	if err := json.NewDecoder(w.Body).Decode(&errors); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}
}

func TestHandleMaintenanceStats(t *testing.T) {
	d := New(nil, nil, nil, nil, true)

	req := httptest.NewRequest("GET", "/api/maintenance", nil)
	w := httptest.NewRecorder()

	d.handleMaintenanceStats(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var stats interface{}
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}
}

func TestHandleDebug(t *testing.T) {
	d := New(nil, nil, nil, nil, true)

	req := httptest.NewRequest("GET", "/api/debug", nil)
	w := httptest.NewRecorder()

	d.handleDebug(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var debug interface{}
	if err := json.NewDecoder(w.Body).Decode(&debug); err != nil {
		t.Errorf("Failed to decode response: %v", err)
	}
}

func TestHandleAIContext(t *testing.T) {
	d := New(nil, nil, nil, nil, true)

	req := httptest.NewRequest("GET", "/api/ai-context", nil)
	w := httptest.NewRecorder()

	d.handleAIContext(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("Expected text/plain content type")
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("AI context should not be empty")
	}
}

func TestGetRouteStatuses(t *testing.T) {
	d := New(nil, nil, nil, nil, true)
	routes := d.getRouteStatuses()

	if routes == nil {
		t.Error("getRouteStatuses should return empty slice, not nil")
	}
}

func TestGetHTMLReturnsContent(t *testing.T) {
	d := New(nil, nil, nil, nil, true)
	html := d.getHTML()

	if len(html) == 0 {
		t.Error("getHTML should return non-empty string")
	}

	if !contains(html, "<!DOCTYPE html>") {
		t.Error("HTML should have DOCTYPE")
	}
}

func TestStartWithEnabledDashboard(t *testing.T) {
	d := New(nil, nil, nil, nil, true)

	mux := http.NewServeMux()
	err := d.Start(nil, mux)

	if err != nil {
		t.Errorf("Start should not return error: %v", err)
	}
}

func TestStartWithDisabledDashboard(t *testing.T) {
	d := New(nil, nil, nil, nil, false)

	mux := http.NewServeMux()
	err := d.Start(nil, mux)

	if err != nil {
		t.Errorf("Start should not return error even when disabled: %v", err)
	}
}

func TestGetDebugInfo(t *testing.T) {
	d := New(nil, nil, nil, nil, true)
	debug := d.getDebugInfo()

	if debug == nil {
		t.Error("getDebugInfo should not return nil")
	}
}

func TestGetMaintenanceStats(t *testing.T) {
	d := New(nil, nil, nil, nil, true)
	stats := d.getMaintenanceStats()

	if stats == nil {
		t.Error("getMaintenanceStats should not return nil")
	}
}

func TestGetRecentErrors(t *testing.T) {
	d := New(nil, nil, nil, nil, true)
	errors := d.getRecentErrors()

	if errors == nil {
		t.Error("getRecentErrors should return empty slice, not nil")
	}
}

func TestGetCertificateStatuses(t *testing.T) {
	d := New(nil, nil, nil, nil, true)
	certs := d.getCertificateStatuses()

	if certs == nil {
		t.Error("getCertificateStatuses should return empty slice, not nil")
	}
}
