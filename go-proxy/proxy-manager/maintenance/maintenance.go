package maintenance

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// MaintenanceState represents a maintenance mode state
type MaintenanceState struct {
	Enabled      bool
	HTMLContent  string
	ScheduledEnd time.Time
	Reason       string
	CreatedAt    time.Time
}

// Manager manages maintenance pages for routes
type Manager struct {
	mu        sync.RWMutex
	states    map[string]*MaintenanceState
	listeners map[string][]func()
}

// New creates a new maintenance page manager
func New() *Manager {
	return &Manager{
		states:    make(map[string]*MaintenanceState),
		listeners: make(map[string][]func()),
	}
}

// SetMaintenanceMode enables or disables maintenance mode for a route
func (m *Manager) SetMaintenanceMode(domain string, enabled bool, htmlContent, reason string, scheduledEnd time.Time) error {
	m.mu.Lock()
	if !enabled {
		delete(m.states, domain)
		m.mu.Unlock()
		log.Info().Str("domain", domain).Msg("Maintenance mode disabled")
		m.notifyListeners(domain)
		return nil
	}

	state := &MaintenanceState{
		Enabled:      enabled,
		HTMLContent:  htmlContent,
		ScheduledEnd: scheduledEnd,
		Reason:       reason,
		CreatedAt:    time.Now(),
	}

	m.states[domain] = state
	m.mu.Unlock()

	log.Info().Str("domain", domain).Str("reason", reason).Time("end", scheduledEnd).Msg("Maintenance mode enabled")

	// Schedule automatic disable if an end time is provided
	if !scheduledEnd.IsZero() && scheduledEnd.After(time.Now()) {
		go m.scheduleDisable(domain, scheduledEnd)
	}

	m.notifyListeners(domain)
	return nil
}

// IsMaintenanceMode returns whether a route is in maintenance mode
func (m *Manager) IsMaintenanceMode(domain string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[domain]
	if !exists {
		return false
	}

	if !state.ScheduledEnd.IsZero() && time.Now().After(state.ScheduledEnd) {
		return false
	}

	return state.Enabled
}

// GetMaintenanceState returns the maintenance state for a route
func (m *Manager) GetMaintenanceState(domain string) *MaintenanceState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.states[domain]
	if !exists {
		return nil
	}

	if !state.ScheduledEnd.IsZero() && time.Now().After(state.ScheduledEnd) {
		return nil
	}

	return state
}

// RenderMaintenancePage renders the maintenance page for a route
func (m *Manager) RenderMaintenancePage(w http.ResponseWriter, domain string) {
	m.mu.RLock()
	state, exists := m.states[domain]
	m.mu.RUnlock()

	if !exists || !state.Enabled {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	if !state.ScheduledEnd.IsZero() && time.Now().After(state.ScheduledEnd) {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	if state.HTMLContent != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, state.HTMLContent)
	} else {
		m.renderDefaultMaintenance(w, domain, state)
	}
}

// renderDefaultMaintenance renders the default maintenance page
func (m *Manager) renderDefaultMaintenance(w http.ResponseWriter, domain string, state *MaintenanceState) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)

	endTime := ""
	if !state.ScheduledEnd.IsZero() {
		endTime = state.ScheduledEnd.Format(time.RFC1123)
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Maintenance Mode</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: #f8fafc; color: #0f172a; margin: 0; padding: 0; }
        .container { max-width: 720px; margin: 60px auto; background: white; padding: 32px; border-radius: 12px; box-shadow: 0 10px 40px rgba(15, 23, 42, 0.12); }
        h1 { font-size: 28px; margin-bottom: 12px; }
        p { margin: 8px 0; line-height: 1.6; }
        .badge { display: inline-block; padding: 6px 12px; background: #2563eb; color: white; border-radius: 9999px; font-size: 12px; text-transform: uppercase; letter-spacing: 0.05em; }
        .meta { margin-top: 20px; padding: 16px; background: #f1f5f9; border-radius: 8px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="badge">Maintenance in Progress</div>
        <h1>%s is temporarily unavailable</h1>
        <p>We're performing maintenance to keep our services running smoothly. Thank you for your patience.</p>
        <div class="meta">
            <p><strong>Reason:</strong> %s</p>
            <p><strong>Expected completion:</strong> %s</p>
        </div>
    </div>
</body>
</html>`, domain, state.Reason, endTime)

	fmt.Fprint(w, html)
}

// scheduleDisable waits until end time and disables maintenance
func (m *Manager) scheduleDisable(domain string, end time.Time) {
	duration := time.Until(end)
	if duration <= 0 {
		return
	}

	time.Sleep(duration)
	// Disable maintenance when the timer elapses
	_ = m.SetMaintenanceMode(domain, false, "", "", time.Time{})
}

// OnStateChange registers a callback for a domain
func (m *Manager) OnStateChange(domain string, callback func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners[domain] = append(m.listeners[domain], callback)
}

// notifyListeners calls all callbacks for a domain
func (m *Manager) notifyListeners(domain string) {
	m.mu.RLock()
	callbacks := append([]func(){}, m.listeners[domain]...)
	m.mu.RUnlock()

	for _, cb := range callbacks {
		cb()
	}
}

// GetAll returns all active maintenance states
func (m *Manager) GetAll() map[string]*MaintenanceState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*MaintenanceState)
	now := time.Now()

	for domain, state := range m.states {
		if !state.ScheduledEnd.IsZero() && now.After(state.ScheduledEnd) {
			continue
		}
		result[domain] = state
	}

	return result
}

// DisableAll disables maintenance mode for all routes
func (m *Manager) DisableAll() {
	m.mu.Lock()
	domains := make([]string, 0, len(m.states))
	for domain := range m.states {
		domains = append(domains, domain)
	}
	m.states = make(map[string]*MaintenanceState)
	m.mu.Unlock()

	for _, domain := range domains {
		m.notifyListeners(domain)
	}

	log.Info().Msg("Maintenance mode disabled for all routes")
}
