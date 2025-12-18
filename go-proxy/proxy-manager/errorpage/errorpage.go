package errorpage

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

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

	html := getErrorPageHTML(statusCode, data.StatusText, data.Message, data.Domain, data.Path, data.RequestID, data.Timestamp)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	fmt.Fprint(w, html)
	return nil
}

// getErrorPageHTML returns default error page HTML
func getErrorPageHTML(statusCode int, statusText, message, domain, path, requestID string, timestamp time.Time) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%d Error</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 20px; }
    .error-container { background: white; border-radius: 12px; box-shadow: 0 20px 60px rgba(0,0,0,0.3); padding: 40px; max-width: 600px; text-align: center; }
    .status-code { font-size: 72px; font-weight: 700; color: #667eea; margin-bottom: 10px; }
    .status-text { font-size: 28px; font-weight: 600; color: #333; margin-bottom: 20px; }
    .message { font-size: 16px; color: #666; line-height: 1.6; margin-bottom: 30px; }
    .details { background: #f5f7fa; border: 1px solid #e0e0e0; border-radius: 8px; padding: 15px; text-align: left; margin-bottom: 30px; font-size: 13px; color: #666; }
    .detail-item { display: flex; justify-content: space-between; padding: 5px 0; border-bottom: 1px solid #e0e0e0; }
    .detail-item:last-child { border-bottom: none; }
    .detail-label { font-weight: 600; color: #333; }
    .detail-value { color: #999; font-family: 'Courier New', monospace; word-break: break-all; }
    .actions { margin-top: 30px; }
    .btn { display: inline-block; padding: 10px 20px; margin: 0 10px; border-radius: 6px; text-decoration: none; font-size: 14px; transition: all 0.2s; }
    .btn-primary { background: #667eea; color: white; }
    .btn-primary:hover { background: #5568d3; }
    .btn-secondary { background: #f5f7fa; color: #333; border: 1px solid #e0e0e0; }
    .btn-secondary:hover { background: #e0e0e0; }
  </style>
</head>
<body>
  <div class="error-container">
    <div class="status-code">%d</div>
    <div class="status-text">%s</div>
    <div class="message">%s</div>
    <div class="details">
      <div class="detail-item">
        <span class="detail-label">Domain:</span>
        <span class="detail-value">%s</span>
      </div>
      <div class="detail-item">
        <span class="detail-label">Path:</span>
        <span class="detail-value">%s</span>
      </div>
      <div class="detail-item">
        <span class="detail-label">Request ID:</span>
        <span class="detail-value">%s</span>
      </div>
      <div class="detail-item">
        <span class="detail-label">Timestamp:</span>
        <span class="detail-value">%s</span>
      </div>
    </div>
    <div class="actions">
      <a href="/" class="btn btn-primary">Go Home</a>
      <a href="javascript:history.back()" class="btn btn-secondary">Go Back</a>
    </div>
  </div>
</body>
</html>`,
		statusCode, statusCode, statusText, message, domain, path, requestID, timestamp.Format(time.RFC3339))
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
