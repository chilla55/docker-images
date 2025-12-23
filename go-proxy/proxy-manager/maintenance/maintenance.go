package maintenance

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/chilla55/proxy-manager/staticpages"
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

	endTime := ""
	if !state.ScheduledEnd.IsZero() {
		endTime = state.ScheduledEnd.Format(time.RFC1123)
	}

	// Use centralized staticpages package
	pageData := staticpages.PageData{
		Domain:        domain,
		Reason:        state.Reason,
		ScheduledEnd:  endTime,
		CustomContent: state.HTMLContent,
	}

	status, html := staticpages.GetPage(staticpages.PageMaintenanceDefault, pageData)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprint(w, html)
}

// renderDefaultMaintenance is deprecated - kept for backward compatibility
// Use RenderMaintenancePage instead
func (m *Manager) renderDefaultMaintenance(w http.ResponseWriter, domain string, state *MaintenanceState) {
	endTime := ""
	if !state.ScheduledEnd.IsZero() {
		endTime = state.ScheduledEnd.Format(time.RFC1123)
	}

	pageData := staticpages.PageData{
		Domain:       domain,
		Reason:       state.Reason,
		ScheduledEnd: endTime,
	}

	status, html := staticpages.GetPage(staticpages.PageMaintenanceDefault, pageData)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
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
