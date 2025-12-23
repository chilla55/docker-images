package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNew_Disabled(t *testing.T) {
	config := Config{
		Enabled: false,
	}

	notifier := New(config)
	if notifier.IsEnabled() {
		t.Error("Notifier should be disabled")
	}

	// Operations should be safe on disabled notifier
	err := notifier.Send(Alert{
		Event: EventServiceDown,
		Title: "Test",
	})
	if err != nil {
		t.Errorf("Send on disabled notifier should not error: %v", err)
	}

	stats := notifier.GetStats()
	if stats.AlertsSent != 0 {
		t.Error("Disabled notifier should have zero stats")
	}
}

func TestNew_Enabled(t *testing.T) {
	config := Config{
		Enabled: true,
		Webhooks: []Webhook{
			{
				Name:   "test-webhook",
				URL:    "http://example.com/webhook",
				Events: []string{string(EventServiceDown)},
			},
		},
	}

	notifier := New(config)
	if !notifier.IsEnabled() {
		t.Error("Notifier should be enabled")
	}

	if len(notifier.webhooks) != 1 {
		t.Errorf("Expected 1 webhook, got %d", len(notifier.webhooks))
	}
}

func TestSend_SuccessfulDelivery(t *testing.T) {
	// Mock webhook server
	received := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Enabled: true,
		Webhooks: []Webhook{
			{
				Name:     "test-webhook",
				URL:      server.URL,
				Events:   []string{string(EventServiceDown)},
				Throttle: 0,
			},
		},
	}

	notifier := New(config)

	alert := Alert{
		Event:       EventServiceDown,
		Title:       "Service Down",
		Description: "Test service is down",
		Severity:    "critical",
		Fields: map[string]string{
			"Service": "test-service",
		},
		Timestamp: time.Now(),
	}

	err := notifier.Send(alert)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if !received {
		t.Error("Webhook server did not receive alert")
	}

	stats := notifier.GetStats()
	if stats.AlertsSent != 1 {
		t.Errorf("Expected 1 alert sent, got %d", stats.AlertsSent)
	}
	if stats.AlertsFailed != 0 {
		t.Errorf("Expected 0 alerts failed, got %d", stats.AlertsFailed)
	}
}

func TestSend_FailedDelivery(t *testing.T) {
	// Mock webhook server that always fails
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := Config{
		Enabled: true,
		Webhooks: []Webhook{
			{
				Name:     "test-webhook",
				URL:      server.URL,
				Events:   []string{string(EventServiceDown)},
				Throttle: 0,
			},
		},
	}

	notifier := New(config)

	alert := Alert{
		Event:    EventServiceDown,
		Title:    "Test",
		Severity: "error",
	}

	err := notifier.Send(alert)
	if err == nil {
		t.Error("Expected error for failed delivery")
	}

	stats := notifier.GetStats()
	if stats.AlertsFailed != 1 {
		t.Errorf("Expected 1 alert failed, got %d", stats.AlertsFailed)
	}
}

func TestSend_Throttling(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Enabled: true,
		Webhooks: []Webhook{
			{
				Name:     "test-webhook",
				URL:      server.URL,
				Events:   []string{string(EventServiceDown)},
				Throttle: 5, // 5 seconds
			},
		},
	}

	notifier := New(config)

	alert := Alert{
		Event:    EventServiceDown,
		Title:    "Test",
		Severity: "error",
	}

	// First alert should succeed
	err := notifier.Send(alert)
	if err != nil {
		t.Fatalf("First send failed: %v", err)
	}

	// Second alert should be throttled
	err = notifier.Send(alert)
	if err != nil {
		t.Fatalf("Second send failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 webhook call, got %d", callCount)
	}

	stats := notifier.GetStats()
	if stats.AlertsThrottled != 1 {
		t.Errorf("Expected 1 throttled alert, got %d", stats.AlertsThrottled)
	}
}

func TestSend_EventFiltering(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Enabled: true,
		Webhooks: []Webhook{
			{
				Name:     "test-webhook",
				URL:      server.URL,
				Events:   []string{string(EventServiceDown)}, // Only service_down
				Throttle: 0,
			},
		},
	}

	notifier := New(config)

	// Send event that webhook handles
	err := notifier.Send(Alert{
		Event:    EventServiceDown,
		Title:    "Test",
		Severity: "error",
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Send event that webhook doesn't handle
	err = notifier.Send(Alert{
		Event:    EventHighErrorRate,
		Title:    "Test",
		Severity: "warning",
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Expected 1 webhook call, got %d", callCount)
	}
}

func TestDiscordPayload(t *testing.T) {
	var receivedPayload DiscordPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&receivedPayload); err != nil {
			t.Errorf("Failed to decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := Config{
		Enabled: true,
		Webhooks: []Webhook{
			{
				Name:     "discord-webhook",
				URL:      server.URL,
				Events:   []string{string(EventServiceDown)},
				Type:     "discord",
				Throttle: 0,
			},
		},
	}

	notifier := New(config)

	alert := Alert{
		Event:       EventServiceDown,
		Title:       "🚨 Service Down Alert",
		Description: "Test service is not responding",
		Severity:    "critical",
		Fields: map[string]string{
			"Service":   "test-service",
			"Last Seen": "2 minutes ago",
		},
		Timestamp: time.Now(),
	}

	err := notifier.Send(alert)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if len(receivedPayload.Embeds) != 1 {
		t.Fatalf("Expected 1 embed, got %d", len(receivedPayload.Embeds))
	}

	embed := receivedPayload.Embeds[0]
	if embed.Title != alert.Title {
		t.Errorf("Title mismatch: got %s, want %s", embed.Title, alert.Title)
	}
	if embed.Description != alert.Description {
		t.Errorf("Description mismatch: got %s, want %s", embed.Description, alert.Description)
	}
	if embed.Color != 15158332 { // Red for critical
		t.Errorf("Color mismatch: got %d, want 15158332", embed.Color)
	}
	if len(embed.Fields) != 2 {
		t.Errorf("Expected 2 fields, got %d", len(embed.Fields))
	}
}

func TestSeverityColors(t *testing.T) {
	notifier := &Notifier{}

	tests := []struct {
		severity string
		color    int
	}{
		{"critical", 15158332},
		{"error", 15158332},
		{"warning", 16776960},
		{"info", 3447003},
		{"unknown", 9807270},
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			color := notifier.getSeverityColor(tt.severity)
			if color != tt.color {
				t.Errorf("Expected color %d for %s, got %d", tt.color, tt.severity, color)
			}
		})
	}
}

func TestGetStats_Copy(t *testing.T) {
	notifier := &Notifier{
		enabled: true,
		stats: Stats{
			AlertsSent: 100,
			ByEvent:    map[string]int64{"test": 50},
			ByWebhook:  map[string]int64{"webhook1": 50},
		},
	}

	stats1 := notifier.GetStats()
	stats1.ByEvent["new"] = 25 // Modify the copy

	stats2 := notifier.GetStats()
	if _, exists := stats2.ByEvent["new"]; exists {
		t.Error("Modification to stats copy should not affect original")
	}
}

func TestClearThrottle(t *testing.T) {
	notifier := &Notifier{
		enabled:  true,
		throttle: make(map[string]time.Time),
	}

	// Add throttle entry
	notifier.throttle[string(EventServiceDown)] = time.Now()

	if len(notifier.throttle) != 1 {
		t.Fatal("Throttle should have 1 entry")
	}

	notifier.ClearThrottle(EventServiceDown)

	if len(notifier.throttle) != 0 {
		t.Error("Throttle should be empty after clear")
	}
}

func TestClearAllThrottles(t *testing.T) {
	notifier := &Notifier{
		enabled:  true,
		throttle: make(map[string]time.Time),
	}

	// Add multiple throttle entries
	notifier.throttle[string(EventServiceDown)] = time.Now()
	notifier.throttle[string(EventHighErrorRate)] = time.Now()

	if len(notifier.throttle) != 2 {
		t.Fatal("Throttle should have 2 entries")
	}

	notifier.ClearAllThrottles()

	if len(notifier.throttle) != 0 {
		t.Error("Throttle should be empty after clear all")
	}
}

func TestClearThrottle_Disabled(t *testing.T) {
	notifier := &Notifier{enabled: false}
	notifier.ClearThrottle(EventServiceDown) // Should not panic
	notifier.ClearAllThrottles()             // Should not panic
}

func TestMultipleWebhooks(t *testing.T) {
	callCount1 := 0
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount1++
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	callCount2 := 0
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount2++
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	config := Config{
		Enabled: true,
		Webhooks: []Webhook{
			{
				Name:     "webhook1",
				URL:      server1.URL,
				Events:   []string{string(EventServiceDown)},
				Throttle: 0,
			},
			{
				Name:     "webhook2",
				URL:      server2.URL,
				Events:   []string{string(EventServiceDown)},
				Throttle: 0,
			},
		},
	}

	notifier := New(config)

	alert := Alert{
		Event:    EventServiceDown,
		Title:    "Test",
		Severity: "error",
	}

	err := notifier.Send(alert)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if callCount1 != 1 {
		t.Errorf("Expected 1 call to webhook1, got %d", callCount1)
	}
	if callCount2 != 1 {
		t.Errorf("Expected 1 call to webhook2, got %d", callCount2)
	}
}

func TestEventTypes(t *testing.T) {
	events := []EventType{
		EventServiceDown,
		EventCertExpiring7d,
		EventCertExpiring14d,
		EventCertExpiring30d,
		EventHighErrorRate,
		EventFailedLoginSpike,
		EventWAFBlockSpike,
		EventUnusualCountry,
		EventRateLimitExceeded,
	}

	for _, event := range events {
		if string(event) == "" {
			t.Errorf("Event type %v has empty string value", event)
		}
	}
}

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"Enabled", true},
		{"Disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &Notifier{enabled: tt.enabled}
			if notifier.IsEnabled() != tt.enabled {
				t.Errorf("IsEnabled() = %v, want %v", notifier.IsEnabled(), tt.enabled)
			}
		})
	}
}
