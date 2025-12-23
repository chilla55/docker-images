package staticpages

import (
	"strings"
	"testing"
	"time"
)

func TestGetPage(t *testing.T) {
	tests := []struct {
		name           string
		pageType       PageType
		data           PageData
		expectedStatus int
		expectedInHTML []string
	}{
		{
			name:           "404 Not Found",
			pageType:       PageError404,
			data:           PageData{Domain: "example.com", Path: "/test"},
			expectedStatus: 404,
			expectedInHTML: []string{"404", "Not Found", "example.com", "/test"},
		},
		{
			name:           "500 Internal Server Error",
			pageType:       PageError500,
			data:           PageData{Domain: "api.example.com", Path: "/api/v1"},
			expectedStatus: 500,
			expectedInHTML: []string{"500", "Internal Server Error", "api.example.com"},
		},
		{
			name:           "503 Service Unavailable",
			pageType:       PageError503,
			data:           PageData{Domain: "service.com", Path: "/"},
			expectedStatus: 503,
			expectedInHTML: []string{"503", "Service Unavailable"},
		},
		{
			name:           "Maintenance Default",
			pageType:       PageMaintenanceDefault,
			data:           PageData{Domain: "site.com", Reason: "Scheduled upgrade", ScheduledEnd: "2024-01-01 12:00:00"},
			expectedStatus: 503,
			expectedInHTML: []string{"Maintenance", "site.com", "Scheduled upgrade"},
		},
		{
			name:           "Maintenance with Custom HTML",
			pageType:       PageMaintenanceDefault,
			data:           PageData{CustomContent: "<html><body><h1>Custom Maintenance</h1></body></html>"},
			expectedStatus: 503,
			expectedInHTML: []string{"Custom Maintenance"},
		},
		{
			name:           "Generic Error Fallback",
			pageType:       "unknown_page_type",
			data:           PageData{StatusCode: 418, Domain: "teapot.com"},
			expectedStatus: 418,
			expectedInHTML: []string{"418", "teapot.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, html := GetPage(tt.pageType, tt.data)

			if status != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, status)
			}

			for _, expected := range tt.expectedInHTML {
				if !strings.Contains(html, expected) {
					t.Errorf("Expected HTML to contain '%s'", expected)
				}
			}
		})
	}
}

func TestGetPageByStatusCode(t *testing.T) {
	tests := []struct {
		statusCode     int
		expectedStatus int
		expectedInHTML []string
	}{
		{400, 400, []string{"400", "Bad Request"}},
		{401, 401, []string{"401", "Unauthorized"}},
		{403, 403, []string{"403", "Forbidden"}},
		{404, 404, []string{"404", "Not Found"}},
		{500, 500, []string{"500", "Internal Server Error"}},
		{502, 502, []string{"502", "Bad Gateway"}},
		{503, 503, []string{"503", "Service Unavailable"}},
		{504, 504, []string{"504", "Gateway Timeout"}},
		{999, 999, []string{"999"}}, // Unknown status code
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
			data := PageData{Domain: "test.com", Path: "/test"}
			status, html := GetPageByStatusCode(tt.statusCode, data)

			if status != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, status)
			}

			for _, expected := range tt.expectedInHTML {
				if !strings.Contains(html, expected) {
					t.Errorf("Expected HTML to contain '%s'", expected)
				}
			}
		})
	}
}

func TestPageDataDefaults(t *testing.T) {
	data := PageData{
		Domain: "test.com",
		Path:   "/api",
	}

	_, html := GetPage(PageError404, data)

	// Should contain generated request ID
	if !strings.Contains(html, "req-") {
		t.Error("Expected HTML to contain generated request ID")
	}

	// Should contain timestamp
	if !strings.Contains(html, time.Now().Format("2006")) {
		t.Error("Expected HTML to contain timestamp with current year")
	}
}

func TestMaintenanceFallback(t *testing.T) {
	status, html := GetPage(PageMaintenanceFallback, PageData{})

	if status != 200 {
		t.Errorf("Expected status 200 for maintenance fallback, got %d", status)
	}

	expectedStrings := []string{"Maintenance", "Temporarily Unavailable"}
	for _, expected := range expectedStrings {
		if !strings.Contains(html, expected) {
			t.Errorf("Expected HTML to contain '%s'", expected)
		}
	}
}

func TestServiceUnavailable(t *testing.T) {
	data := PageData{Domain: "unavailable.example.com"}
	status, html := GetPage(PageServiceUnavailable, data)

	if status != 503 {
		t.Errorf("Expected status 503, got %d", status)
	}

	if !strings.Contains(html, "unavailable.example.com") {
		t.Error("Expected HTML to contain domain")
	}

	if !strings.Contains(html, "503") {
		t.Error("Expected HTML to contain status code 503")
	}
}

func BenchmarkGetPage(b *testing.B) {
	data := PageData{
		Domain:    "benchmark.com",
		Path:      "/test",
		RequestID: "bench-123",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GetPage(PageError404, data)
	}
}

func BenchmarkGetPageByStatusCode(b *testing.B) {
	data := PageData{
		Domain:    "benchmark.com",
		Path:      "/test",
		RequestID: "bench-123",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GetPageByStatusCode(500, data)
	}
}
