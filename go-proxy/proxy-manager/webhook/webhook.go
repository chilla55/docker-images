package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Notifier handles webhook notifications to external services
type Notifier struct {
	webhooks      []Webhook
	throttle      map[string]time.Time // event -> last alert time
	throttleMutex sync.RWMutex
	stats         Stats
	statsMutex    sync.RWMutex
	enabled       bool
}

// Webhook represents a webhook configuration
type Webhook struct {
	Name     string   `yaml:"name"`
	URL      string   `yaml:"url"`
	Events   []string `yaml:"events"`   // Which events to send
	Throttle int      `yaml:"throttle"` // Seconds between alerts for same event
	Type     string   `yaml:"type"`     // discord, slack, generic
}

// EventType represents different alert event types
type EventType string

const (
	EventServiceDown       EventType = "service_down"
	EventCertExpiring7d    EventType = "cert_expiring_7d"
	EventCertExpiring14d   EventType = "cert_expiring_14d"
	EventCertExpiring30d   EventType = "cert_expiring_30d"
	EventHighErrorRate     EventType = "high_error_rate"
	EventFailedLoginSpike  EventType = "failed_login_spike"
	EventWAFBlockSpike     EventType = "waf_block_spike"
	EventUnusualCountry    EventType = "unusual_country"
	EventRateLimitExceeded EventType = "rate_limit_exceeded"
	EventSlowRequest       EventType = "slow_request"
)

// Alert represents an alert to be sent
type Alert struct {
	Event       EventType         `json:"event"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Severity    string            `json:"severity"` // info, warning, error, critical
	Fields      map[string]string `json:"fields"`
	Timestamp   time.Time         `json:"timestamp"`
}

// Stats tracks webhook statistics
type Stats struct {
	AlertsSent      int64            `json:"alerts_sent"`
	AlertsFailed    int64            `json:"alerts_failed"`
	AlertsThrottled int64            `json:"alerts_throttled"`
	ByEvent         map[string]int64 `json:"by_event"`
	ByWebhook       map[string]int64 `json:"by_webhook"`
}

// Config represents webhook configuration
type Config struct {
	Enabled  bool      `yaml:"enabled"`
	Webhooks []Webhook `yaml:"webhooks"`
}

// DiscordEmbed represents a Discord webhook embed
type DiscordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Fields      []DiscordField `json:"fields"`
	Timestamp   string         `json:"timestamp"`
}

// DiscordField represents a field in a Discord embed
type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// DiscordPayload represents a Discord webhook payload
type DiscordPayload struct {
	Embeds []DiscordEmbed `json:"embeds"`
}

// New creates a new webhook notifier
func New(config Config) *Notifier {
	if !config.Enabled {
		log.Info().Msg("Webhook notifications disabled")
		return &Notifier{enabled: false}
	}

	notifier := &Notifier{
		webhooks: config.Webhooks,
		throttle: make(map[string]time.Time),
		enabled:  true,
		stats: Stats{
			ByEvent:   make(map[string]int64),
			ByWebhook: make(map[string]int64),
		},
	}

	log.Info().
		Int("webhooks", len(config.Webhooks)).
		Msg("Webhook notifier initialized")

	return notifier
}

// Send sends an alert to all configured webhooks
func (n *Notifier) Send(alert Alert) error {
	if !n.enabled {
		return nil
	}

	// Check throttling
	throttleKey := string(alert.Event)
	n.throttleMutex.RLock()
	lastAlert, exists := n.throttle[throttleKey]
	n.throttleMutex.RUnlock()

	var errors []error
	sent := false

	for _, webhook := range n.webhooks {
		// Check if this webhook handles this event
		if !n.webhookHandlesEvent(webhook, alert.Event) {
			continue
		}

		// Check throttle
		if exists && time.Since(lastAlert) < time.Duration(webhook.Throttle)*time.Second {
			n.statsMutex.Lock()
			n.stats.AlertsThrottled++
			n.statsMutex.Unlock()
			log.Debug().
				Str("event", string(alert.Event)).
				Str("webhook", webhook.Name).
				Msg("Alert throttled")
			continue
		}

		// Send alert
		if err := n.sendToWebhook(webhook, alert); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", webhook.Name, err))
			n.statsMutex.Lock()
			n.stats.AlertsFailed++
			n.statsMutex.Unlock()
		} else {
			sent = true
			n.statsMutex.Lock()
			n.stats.AlertsSent++
			n.stats.ByEvent[string(alert.Event)]++
			n.stats.ByWebhook[webhook.Name]++
			n.statsMutex.Unlock()
		}
	}

	// Update throttle timestamp if at least one alert was sent
	if sent {
		n.throttleMutex.Lock()
		n.throttle[throttleKey] = time.Now()
		n.throttleMutex.Unlock()
	}

	if len(errors) > 0 {
		return fmt.Errorf("webhook errors: %v", errors)
	}

	return nil
}

// webhookHandlesEvent checks if a webhook should handle a given event
func (n *Notifier) webhookHandlesEvent(webhook Webhook, event EventType) bool {
	for _, e := range webhook.Events {
		if e == string(event) {
			return true
		}
	}
	return false
}

// sendToWebhook sends an alert to a specific webhook
func (n *Notifier) sendToWebhook(webhook Webhook, alert Alert) error {
	var payload interface{}

	switch webhook.Type {
	case "discord":
		payload = n.buildDiscordPayload(alert)
	case "slack":
		payload = n.buildSlackPayload(alert)
	default:
		payload = alert // Generic JSON
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", webhook.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	log.Info().
		Str("webhook", webhook.Name).
		Str("event", string(alert.Event)).
		Str("title", alert.Title).
		Msg("Alert sent")

	return nil
}

// buildDiscordPayload builds a Discord-compatible payload
func (n *Notifier) buildDiscordPayload(alert Alert) DiscordPayload {
	color := n.getSeverityColor(alert.Severity)

	fields := make([]DiscordField, 0, len(alert.Fields))
	for name, value := range alert.Fields {
		fields = append(fields, DiscordField{
			Name:   name,
			Value:  value,
			Inline: true,
		})
	}

	embed := DiscordEmbed{
		Title:       alert.Title,
		Description: alert.Description,
		Color:       color,
		Fields:      fields,
		Timestamp:   alert.Timestamp.Format(time.RFC3339),
	}

	return DiscordPayload{
		Embeds: []DiscordEmbed{embed},
	}
}

// buildSlackPayload builds a Slack-compatible payload
func (n *Notifier) buildSlackPayload(alert Alert) interface{} {
	color := n.getSeverityColorHex(alert.Severity)

	fields := make([]map[string]interface{}, 0, len(alert.Fields))
	for name, value := range alert.Fields {
		fields = append(fields, map[string]interface{}{
			"title": name,
			"value": value,
			"short": true,
		})
	}

	return map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"title":  alert.Title,
				"text":   alert.Description,
				"color":  color,
				"fields": fields,
				"ts":     alert.Timestamp.Unix(),
			},
		},
	}
}

// getSeverityColor returns Discord color code for severity
func (n *Notifier) getSeverityColor(severity string) int {
	switch severity {
	case "critical":
		return 15158332 // Red
	case "error":
		return 15158332 // Red
	case "warning":
		return 16776960 // Yellow
	case "info":
		return 3447003 // Blue
	default:
		return 9807270 // Gray
	}
}

// getSeverityColorHex returns hex color for severity (for Slack)
func (n *Notifier) getSeverityColorHex(severity string) string {
	switch severity {
	case "critical":
		return "#e74c3c" // Red
	case "error":
		return "#e74c3c" // Red
	case "warning":
		return "#f39c12" // Yellow
	case "info":
		return "#3498db" // Blue
	default:
		return "#95a5a6" // Gray
	}
}

// GetStats returns current webhook statistics
func (n *Notifier) GetStats() Stats {
	if !n.enabled {
		return Stats{
			ByEvent:   make(map[string]int64),
			ByWebhook: make(map[string]int64),
		}
	}

	n.statsMutex.RLock()
	defer n.statsMutex.RUnlock()

	stats := Stats{
		AlertsSent:      n.stats.AlertsSent,
		AlertsFailed:    n.stats.AlertsFailed,
		AlertsThrottled: n.stats.AlertsThrottled,
		ByEvent:         make(map[string]int64),
		ByWebhook:       make(map[string]int64),
	}

	for event, count := range n.stats.ByEvent {
		stats.ByEvent[event] = count
	}
	for webhook, count := range n.stats.ByWebhook {
		stats.ByWebhook[webhook] = count
	}

	return stats
}

// IsEnabled returns whether webhook notifications are enabled
func (n *Notifier) IsEnabled() bool {
	return n.enabled
}

// ClearThrottle clears throttle state for a specific event
func (n *Notifier) ClearThrottle(event EventType) {
	if !n.enabled {
		return
	}

	n.throttleMutex.Lock()
	delete(n.throttle, string(event))
	n.throttleMutex.Unlock()

	log.Info().Str("event", string(event)).Msg("Throttle cleared for event")
}

// ClearAllThrottles clears all throttle state
func (n *Notifier) ClearAllThrottles() {
	if !n.enabled {
		return
	}

	n.throttleMutex.Lock()
	n.throttle = make(map[string]time.Time)
	n.throttleMutex.Unlock()

	log.Info().Msg("All throttles cleared")
}
