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

	// Maintenance pages should return 503 (Service Unavailable)
	if status != 503 {
		t.Errorf("Expected status 503 for maintenance fallback, got %d", status)
	}

	expectedStrings := []string{"Service Temporarily Unavailable", "maintenance"}
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

func TestMaintenancePageVariations(t *testing.T) {
	tests := []struct {
		name           string
		data           PageData
		expectedStatus int
		expectedInHTML []string
		notInHTML      []string
	}{
		{
			name:           "Simple maintenance (no details)",
			data:           PageData{},
			expectedStatus: 503,
			expectedInHTML: []string{"Service Temporarily Unavailable", "maintenance"},
			notInHTML:      []string{"Reason:", "Expected completion:"},
		},
		{
			name:           "Detailed maintenance (with reason and schedule)",
			data:           PageData{Domain: "example.com", Reason: "Database upgrade", ScheduledEnd: "2024-12-25 15:00"},
			expectedStatus: 503,
			expectedInHTML: []string{"Maintenance in Progress", "example.com", "Database upgrade", "2024-12-25 15:00", "badge"},
			notInHTML:      []string{},
		},
		{
			name:           "Service unavailable (domain only)",
			data:           PageData{Domain: "api.example.com"},
			expectedStatus: 503,
			expectedInHTML: []string{"503", "Service Temporarily Unavailable", "api.example.com", "not currently configured"},
			notInHTML:      []string{"Maintenance in Progress", "Reason:", "Expected completion:"},
		},
		{
			name:           "Maintenance with domain but defaults (service unavailable pattern)",
			data:           PageData{Domain: "test.com", Reason: "", ScheduledEnd: ""},
			expectedStatus: 503,
			expectedInHTML: []string{"503", "test.com", "Service Temporarily Unavailable"},
			notInHTML:      []string{"Maintenance in Progress"},
		},
		{
			name:           "Maintenance with only reason (detailed maintenance page)",
			data:           PageData{Reason: "Emergency fix"},
			expectedStatus: 503,
			expectedInHTML: []string{"Maintenance in Progress", "Emergency fix", "Not specified", "badge"},
			notInHTML:      []string{},
		},
		{
			name:           "Maintenance with only scheduled end (detailed maintenance page)",
			data:           PageData{ScheduledEnd: "2024-12-25 10:00"},
			expectedStatus: 503,
			expectedInHTML: []string{"Maintenance in Progress", "2024-12-25 10:00", "Scheduled maintenance", "badge"},
			notInHTML:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, html := GetPage(PageMaintenanceDefault, tt.data)

			if status != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, status)
			}

			for _, expected := range tt.expectedInHTML {
				if !strings.Contains(html, expected) {
					t.Errorf("Expected HTML to contain '%s', but it didn't", expected)
				}
			}

			for _, notExpected := range tt.notInHTML {
				if strings.Contains(html, notExpected) {
					t.Errorf("Expected HTML to NOT contain '%s', but it did", notExpected)
				}
			}
		})
	}
}

func TestDarkLightModeCSS(t *testing.T) {
	_, html := GetPage(PageError404, PageData{Domain: "test.com"})

	// Check for dark mode media query
	if !strings.Contains(html, "@media (prefers-color-scheme: dark)") {
		t.Error("Expected HTML to contain dark mode media query")
	}

	// Check for color-scheme meta
	if !strings.Contains(html, "color-scheme: light dark") {
		t.Error("Expected HTML to contain color-scheme declaration")
	}
}

func TestErrorMessageDefaults(t *testing.T) {
	tests := []struct {
		statusCode      int
		expectedMessage string
	}{
		{400, "malformed syntax"},
		{401, "Authentication is required"},
		{403, "permission"},
		{404, "could not be found"},
		{500, "internal error"},
		{502, "Bad Gateway"},
		{503, "temporarily unavailable"},
		{504, "timely response"},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
			_, html := GetPageByStatusCode(tt.statusCode, PageData{})

			if !strings.Contains(strings.ToLower(html), strings.ToLower(tt.expectedMessage)) {
				t.Errorf("Expected default message for %d to contain '%s'", tt.statusCode, tt.expectedMessage)
			}
		})
	}
}

func TestCustomMessageOverride(t *testing.T) {
	customMsg := "This is a custom error message for testing"
	data := PageData{
		Domain:  "test.com",
		Message: customMsg,
	}

	_, html := GetPage(PageError404, data)

	if !strings.Contains(html, customMsg) {
		t.Error("Expected HTML to contain custom message")
	}

	// Should not contain the default 404 message
	if strings.Contains(html, "could not be found on this server") {
		t.Error("Expected default message to be overridden")
	}
}

func TestEdgeCasesForFullCoverage(t *testing.T) {
	t.Run("StatusCode zero defaults to 500", func(t *testing.T) {
		data := PageData{
			StatusCode: 0,
			Domain:     "test.com",
		}
		status, html := GetPageByStatusCode(0, data)

		if status != 500 {
			t.Errorf("Expected status 500 when statusCode is 0, got %d", status)
		}
		if !strings.Contains(html, "500") {
			t.Error("Expected HTML to contain 500 status code")
		}
	})

	t.Run("Unknown status code with no http.StatusText", func(t *testing.T) {
		// Use a very high status code that Go doesn't recognize
		data := PageData{
			StatusCode: 999999,
			Domain:     "test.com",
		}
		status, html := GetPageByStatusCode(999999, data)

		if status != 999999 {
			t.Errorf("Expected status 999999, got %d", status)
		}
		// When http.StatusText returns empty, it should default to "Error"
		if !strings.Contains(html, "Error") {
			t.Error("Expected HTML to contain 'Error' for unknown status code")
		}
	})

	t.Run("Custom StatusText is preserved", func(t *testing.T) {
		customStatusText := "Custom Status Text"
		data := PageData{
			StatusCode: 404,
			StatusText: customStatusText,
			Domain:     "test.com",
		}
		_, html := GetPageByStatusCode(404, data)

		if !strings.Contains(html, customStatusText) {
			t.Error("Expected HTML to contain custom status text")
		}
	})
}
