package errorpage

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

	"github.com/chilla55/proxy-manager/staticpages"
	"github.com/rs/zerolog/log"
)

// ErrorPageConfig holds error page configuration
type ErrorPageConfig struct {
	Enabled   bool
	PageDir   string
	CustomMap map[int]string
}

// ErrorData contains template variables for error pages
type ErrorData struct {
	StatusCode int
	StatusText string
	Message    string
	Route      string
	RequestID  string
	Timestamp  time.Time
	Domain     string
	Path       string
}

// Manager manages custom error pages
type Manager struct {
	mu        sync.RWMutex
	enabled   bool
	templates map[int]*template.Template
	customMap map[int]string
}

// New creates a new error page manager
func New(cfg ErrorPageConfig) *Manager {
	m := &Manager{
		enabled:   cfg.Enabled,
		templates: make(map[int]*template.Template),
		customMap: cfg.CustomMap,
	}

	if !cfg.Enabled {
		log.Info().Msg("Error pages disabled")
		return m
	}

	log.Info().Msg("Error page manager initialized")
	return m
}

// SetTemplate registers a template for a status code
func (m *Manager) SetTemplate(statusCode int, tmpl *template.Template) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.templates[statusCode] = tmpl
}

// SetTemplateHTML registers an HTML string as template for a status code
func (m *Manager) SetTemplateHTML(statusCode int, html string) error {
	tmpl, err := template.New(fmt.Sprintf("error-%d", statusCode)).Parse(html)
	if err != nil {
		return err
	}
	m.SetTemplate(statusCode, tmpl)
	return nil
}

// RenderError renders a custom error page for the given status code
func (m *Manager) RenderError(w http.ResponseWriter, statusCode int, data ErrorData) error {
	m.mu.RLock()
	tmpl, exists := m.templates[statusCode]
	m.mu.RUnlock()

	if !exists {
		return m.renderDefault(w, statusCode, data)
	}

	data.StatusText = http.StatusText(statusCode)
	if data.StatusText == "" {
		data.StatusText = "Error"
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Error().Err(err).Int("status", statusCode).Msg("Failed to render error template")
		return m.renderDefault(w, statusCode, data)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	w.Write(buf.Bytes())
	return nil
}

// renderDefault renders a simple default error page
func (m *Manager) renderDefault(w http.ResponseWriter, statusCode int, data ErrorData) error {
	data.StatusText = http.StatusText(statusCode)
	if data.StatusText == "" {
		data.StatusText = "Error"
	}

	// Use centralized staticpages package
	pageData := staticpages.PageData{
		StatusCode: statusCode,
		StatusText: data.StatusText,
		Message:    data.Message,
		Domain:     data.Domain,
		Path:       data.Path,
		RequestID:  data.RequestID,
		Timestamp:  data.Timestamp,
	}

	status, html := staticpages.GetPageByStatusCode(statusCode, pageData)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprint(w, html)
	return nil
}

// IsEnabled returns whether custom error pages are enabled
func (m *Manager) IsEnabled() bool {
	return m.enabled
}

// GetStatusCodeText returns the HTTP status text for a code
func GetStatusCodeText(statusCode int) string {
	text := http.StatusText(statusCode)
	if text == "" {
		return "Unknown Error"
	}
	return text
}
