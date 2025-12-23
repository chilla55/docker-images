package staticpages

import (
	"fmt"
	"net/http"
	"time"
)

// PageType represents the type of static page to render
type PageType string

const (
	// Error pages
	PageError400 PageType = "error_400"
	PageError401 PageType = "error_401"
	PageError403 PageType = "error_403"
	PageError404 PageType = "error_404"
	PageError500 PageType = "error_500"
	PageError502 PageType = "error_502"
	PageError503 PageType = "error_503"
	PageError504 PageType = "error_504"

	// Maintenance pages
	PageMaintenanceDefault  PageType = "maintenance_default"
	PageMaintenanceFallback PageType = "maintenance_fallback"

	// Service pages
	PageServiceUnavailable PageType = "service_unavailable"

	// Generic fallback
	PageGenericError PageType = "generic_error"
)

// PageData contains the context data for rendering pages
type PageData struct {
	StatusCode    int
	StatusText    string
	Message       string
	Domain        string
	Path          string
	RequestID     string
	Timestamp     time.Time
	Reason        string // For maintenance pages
	ScheduledEnd  string // For maintenance pages
	CustomContent string // For custom HTML content
}

// Default error messages for common HTTP status codes
var defaultMessages = map[int]string{
	400: "The request could not be understood by the server due to malformed syntax.",
	401: "Authentication is required to access this resource.",
	403: "You don't have permission to access this resource.",
	404: "The requested resource could not be found on this server.",
	500: "The server encountered an internal error and was unable to complete your request.",
	502: "The server received an invalid response from an upstream server.",
	503: "The service is temporarily unavailable. Please try again later.",
	504: "The server did not receive a timely response from an upstream server.",
}

// Map PageType to status codes for errors
var pageTypeToStatus = map[PageType]int{
	PageError400: 400, PageError401: 401, PageError403: 403, PageError404: 404,
	PageError500: 500, PageError502: 502, PageError503: 503, PageError504: 504,
}

// GetPage returns the HTTP status code and HTML content for the specified page type
func GetPage(pageType PageType, data PageData) (int, string) {
	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}
	if data.RequestID == "" {
		data.RequestID = fmt.Sprintf("req-%d", time.Now().UnixNano())
	}

	// Check if it's an error page
	if statusCode, ok := pageTypeToStatus[pageType]; ok {
		return getError(statusCode, data)
	}

	// Handle special pages
	switch pageType {
	case PageMaintenanceDefault, PageMaintenanceFallback, PageServiceUnavailable:
		return getMaintenance(data)
	default:
		return getError(data.StatusCode, data)
	}
}

// GetPageByStatusCode returns the appropriate page based on HTTP status code
func GetPageByStatusCode(statusCode int, data PageData) (int, string) {
	data.StatusCode = statusCode
	return getError(statusCode, data)
}

// getError returns an error page for any HTTP status code with appropriate defaults
func getError(statusCode int, data PageData) (int, string) {
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}
	if data.StatusText == "" {
		if data.StatusText = http.StatusText(statusCode); data.StatusText == "" {
			data.StatusText = "Error"
		}
	}
	if data.Message == "" {
		if msg, ok := defaultMessages[statusCode]; ok {
			data.Message = msg
		} else {
			data.Message = "An unexpected error occurred while processing your request."
		}
	}
	return statusCode, baseTemplate(fmt.Sprintf("%d Error", statusCode), fmt.Sprintf(`
    <div class="status-code">%d</div>
    <div class="status-text">%s</div>
    <div class="message">%s</div>
    <div class="details">
      <div class="detail-item"><span class="detail-label">Domain:</span><span class="detail-value">%s</span></div>
      <div class="detail-item"><span class="detail-label">Path:</span><span class="detail-value">%s</span></div>
      <div class="detail-item"><span class="detail-label">Request ID:</span><span class="detail-value">%s</span></div>
      <div class="detail-item"><span class="detail-label">Timestamp:</span><span class="detail-value">%s</span></div>
    </div>
    <div class="actions">
      <a href="/" class="btn btn-primary">Go Home</a>
      <a href="javascript:history.back()" class="btn btn-secondary">Go Back</a>
    </div>`, statusCode, data.StatusText, data.Message, data.Domain, data.Path, data.RequestID, data.Timestamp.Format(time.RFC3339)))
}

// baseTemplate returns the shared HTML template with dark/light mode support
func baseTemplate(title, content string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    :root { color-scheme: light dark; }
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #f8fafc; color: #1e293b; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 20px; }
    .container { background: #fff; border-radius: 12px; box-shadow: 0 4px 6px rgba(0,0,0,.1); padding: 40px; max-width: 600px; width: 100%%; text-align: center; }
    .status-code, .status-code-warning { font-size: 72px; font-weight: 700; margin-bottom: 10px; }
    .status-code { color: #3b82f6; }
    .status-code-warning { color: #f59e0b; }
    .status-text, h1 { font-size: 28px; font-weight: 600; color: #1e293b; margin-bottom: 20px; }
    .message, p { font-size: 16px; color: #64748b; line-height: 1.6; margin-bottom: 30px; }
    .badge { display: inline-block; padding: 8px 16px; background: #f59e0b; color: #fff; border-radius: 9999px; font-size: 12px; text-transform: uppercase; letter-spacing: 0.05em; font-weight: 600; margin-bottom: 20px; }
    .details, .meta { background: #f1f5f9; border: 1px solid #e2e8f0; border-radius: 8px; padding: 15px; margin-bottom: 30px; font-size: 13px; color: #475569; }
    .details { text-align: left; }
    .meta { margin-top: 30px; padding: 20px; }
    .detail-item, .meta-item { display: flex; justify-content: space-between; padding: 8px 0; border-bottom: 1px solid #e2e8f0; }
    .detail-item:last-child, .meta-item:last-child { border-bottom: none; }
    .detail-label, .meta-item strong { font-weight: 600; color: #334155; }
    .detail-value { color: #64748b; font-family: 'Courier New', monospace; word-break: break-all; text-align: right; max-width: 60%%; }
    .meta-item { display: block; margin: 12px 0; font-size: 14px; }
    .meta-item strong { min-width: 150px; display: inline-block; }
    .domain { font-family: 'Courier New', monospace; background: #f1f5f9; color: #475569; padding: 12px 20px; border-radius: 8px; display: inline-block; margin: 20px 0; font-size: 14px; border: 1px solid #e2e8f0; }
    .actions { margin-top: 30px; }
    .btn { display: inline-block; padding: 10px 20px; margin: 5px; border-radius: 6px; text-decoration: none; font-size: 14px; transition: all .2s; font-weight: 500; }
    .btn-primary { background: #3b82f6; color: #fff; }
    .btn-primary:hover { background: #2563eb; }
    .btn-secondary { background: #f1f5f9; color: #334155; border: 1px solid #e2e8f0; }
    .btn-secondary:hover { background: #e2e8f0; }
    @media (prefers-color-scheme: dark) {
      body { background: #0f172a; color: #e2e8f0; }
      .container { background: #1e293b; box-shadow: 0 4px 6px rgba(0,0,0,.3); }
      .status-code { color: #60a5fa; }
      .status-code-warning { color: #fbbf24; }
      .status-text, h1 { color: #f1f5f9; }
      .message, p { color: #cbd5e1; }
      .badge { background: #d97706; }
      .details, .meta { background: #334155; border-color: #475569; color: #cbd5e1; }
      .detail-item, .meta-item { border-bottom-color: #475569; }
      .detail-label, .meta-item strong { color: #f1f5f9; }
      .detail-value { color: #94a3b8; }
      .domain { background: #334155; color: #cbd5e1; border-color: #475569; }
      .btn-primary { background: #2563eb; }
      .btn-primary:hover { background: #1d4ed8; }
      .btn-secondary { background: #334155; color: #e2e8f0; border-color: #475569; }
      .btn-secondary:hover { background: #475569; }
    }
  </style>
</head>
<body>
  <div class="container">%s</div>
</body>
</html>`, title, content)
}

// getMaintenance returns a maintenance page, adapting to the data available
func getMaintenance(data PageData) (int, string) {
	if data.CustomContent != "" {
		return http.StatusServiceUnavailable, data.CustomContent
	}

	// Check if this is a 503 service unavailable (has domain but no reason/schedule)
	isServiceUnavailable := data.Domain != "" && data.Reason == "" && data.ScheduledEnd == ""
	if isServiceUnavailable {
		return http.StatusServiceUnavailable, baseTemplate("Service Temporarily Unavailable", fmt.Sprintf(`
    <div class="status-code-warning">503</div>
    <h1>Service Temporarily Unavailable</h1>
    <div class="domain">%s</div>
    <p>The requested service is not currently configured or is temporarily unavailable.</p>
    <p>Please check back later or contact the administrator if this problem persists.</p>`, data.Domain))
	}

	// Simple fallback if no details
	if data.Domain == "" && data.Reason == "" && data.ScheduledEnd == "" {
		return http.StatusServiceUnavailable, baseTemplate("Maintenance", `
    <h1>Service Temporarily Unavailable</h1>
    <p>This service is currently undergoing maintenance. Please try again later.</p>`)
	}

	// Detailed maintenance page
	if data.Reason == "" {
		data.Reason = "Scheduled maintenance"
	}
	if data.ScheduledEnd == "" {
		data.ScheduledEnd = "Not specified"
	}
	if data.Domain == "" {
		data.Domain = "This service"
	}

	return http.StatusServiceUnavailable, baseTemplate("Maintenance Mode", fmt.Sprintf(`
    <div class="badge">Maintenance in Progress</div>
    <h1>%s is temporarily unavailable</h1>
    <p>We're performing maintenance to keep our services running smoothly. Thank you for your patience.</p>
    <div class="meta">
      <div class="meta-item"><strong>Reason:</strong> %s</div>
      <div class="meta-item"><strong>Expected completion:</strong> %s</div>
    </div>`, data.Domain, data.Reason, data.ScheduledEnd))
}
