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

// GetPage returns the HTTP status code and HTML content for the specified page type
// If the page type is not found, it returns a generic error page
func GetPage(pageType PageType, data PageData) (int, string) {
	// Set defaults if not provided
	if data.Timestamp.IsZero() {
		data.Timestamp = time.Now()
	}
	if data.RequestID == "" {
		data.RequestID = generateRequestID()
	}

	switch pageType {
	case PageError400:
		return getError400(data)
	case PageError401:
		return getError401(data)
	case PageError403:
		return getError403(data)
	case PageError404:
		return getError404(data)
	case PageError500:
		return getError500(data)
	case PageError502:
		return getError502(data)
	case PageError503:
		return getError503(data)
	case PageError504:
		return getError504(data)
	case PageMaintenanceDefault:
		return getMaintenanceDefault(data)
	case PageMaintenanceFallback:
		return getMaintenanceFallback(data)
	case PageServiceUnavailable:
		return getServiceUnavailable(data)
	case PageGenericError:
		return getGenericError(data)
	default:
		// Fallback to generic error page
		return getGenericError(data)
	}
}

// GetPageByStatusCode returns the appropriate page based on HTTP status code
func GetPageByStatusCode(statusCode int, data PageData) (int, string) {
	data.StatusCode = statusCode
	data.StatusText = http.StatusText(statusCode)
	if data.StatusText == "" {
		data.StatusText = "Error"
	}

	switch statusCode {
	case 400:
		return GetPage(PageError400, data)
	case 401:
		return GetPage(PageError401, data)
	case 403:
		return GetPage(PageError403, data)
	case 404:
		return GetPage(PageError404, data)
	case 500:
		return GetPage(PageError500, data)
	case 502:
		return GetPage(PageError502, data)
	case 503:
		return GetPage(PageError503, data)
	case 504:
		return GetPage(PageError504, data)
	default:
		return GetPage(PageGenericError, data)
	}
}

// getError400 returns Bad Request error page
func getError400(data PageData) (int, string) {
	if data.StatusText == "" {
		data.StatusText = "Bad Request"
	}
	if data.Message == "" {
		data.Message = "The request could not be understood by the server due to malformed syntax."
	}
	return http.StatusBadRequest, getErrorHTML(400, data)
}

// getError401 returns Unauthorized error page
func getError401(data PageData) (int, string) {
	if data.StatusText == "" {
		data.StatusText = "Unauthorized"
	}
	if data.Message == "" {
		data.Message = "Authentication is required to access this resource."
	}
	return http.StatusUnauthorized, getErrorHTML(401, data)
}

// getError403 returns Forbidden error page
func getError403(data PageData) (int, string) {
	if data.StatusText == "" {
		data.StatusText = "Forbidden"
	}
	if data.Message == "" {
		data.Message = "You don't have permission to access this resource."
	}
	return http.StatusForbidden, getErrorHTML(403, data)
}

// getError404 returns Not Found error page
func getError404(data PageData) (int, string) {
	if data.StatusText == "" {
		data.StatusText = "Not Found"
	}
	if data.Message == "" {
		data.Message = "The requested resource could not be found on this server."
	}
	return http.StatusNotFound, getErrorHTML(404, data)
}

// getError500 returns Internal Server Error page
func getError500(data PageData) (int, string) {
	if data.StatusText == "" {
		data.StatusText = "Internal Server Error"
	}
	if data.Message == "" {
		data.Message = "The server encountered an internal error and was unable to complete your request."
	}
	return http.StatusInternalServerError, getErrorHTML(500, data)
}

// getError502 returns Bad Gateway error page
func getError502(data PageData) (int, string) {
	if data.StatusText == "" {
		data.StatusText = "Bad Gateway"
	}
	if data.Message == "" {
		data.Message = "The server received an invalid response from an upstream server."
	}
	return http.StatusBadGateway, getErrorHTML(502, data)
}

// getError503 returns Service Unavailable error page
func getError503(data PageData) (int, string) {
	if data.StatusText == "" {
		data.StatusText = "Service Unavailable"
	}
	if data.Message == "" {
		data.Message = "The service is temporarily unavailable. Please try again later."
	}
	return http.StatusServiceUnavailable, getErrorHTML(503, data)
}

// getError504 returns Gateway Timeout error page
func getError504(data PageData) (int, string) {
	if data.StatusText == "" {
		data.StatusText = "Gateway Timeout"
	}
	if data.Message == "" {
		data.Message = "The server did not receive a timely response from an upstream server."
	}
	return http.StatusGatewayTimeout, getErrorHTML(504, data)
}

// getGenericError returns a generic error page
func getGenericError(data PageData) (int, string) {
	statusCode := data.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusInternalServerError
	}
	if data.StatusText == "" {
		data.StatusText = http.StatusText(statusCode)
		if data.StatusText == "" {
			data.StatusText = "Error"
		}
	}
	if data.Message == "" {
		data.Message = "An unexpected error occurred while processing your request."
	}
	return statusCode, getErrorHTML(statusCode, data)
}

// getErrorHTML returns the standard error page HTML template
func getErrorHTML(statusCode int, data PageData) string {
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
		statusCode, statusCode, data.StatusText, data.Message, data.Domain, data.Path, data.RequestID, data.Timestamp.Format(time.RFC3339))
}

// getMaintenanceDefault returns the default maintenance page with custom styling
func getMaintenanceDefault(data PageData) (int, string) {
	// If custom HTML content is provided, return it
	if data.CustomContent != "" {
		return http.StatusServiceUnavailable, data.CustomContent
	}

	// Otherwise return default maintenance page
	endTime := data.ScheduledEnd
	if endTime == "" {
		endTime = "Not specified"
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
</html>`, data.Domain, data.Reason, endTime)

	return http.StatusServiceUnavailable, html
}

// getMaintenanceFallback returns a minimal maintenance page
func getMaintenanceFallback(data PageData) (int, string) {
	html := `<!DOCTYPE html>
<html><head><title>Maintenance</title></head>
<body><h1>Service Temporarily Unavailable</h1>
<p>This service is currently undergoing maintenance. Please try again later.</p>
</body></html>`
	return http.StatusOK, html
}

// getServiceUnavailable returns a service unavailable page with retry headers
func getServiceUnavailable(data PageData) (int, string) {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Service Temporarily Unavailable</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: #fff;
        }
        .container {
            text-align: center;
            padding: 2rem;
            max-width: 600px;
        }
        h1 {
            font-size: 3rem;
            margin: 0 0 1rem 0;
            font-weight: 700;
        }
        .status-code {
            font-size: 8rem;
            font-weight: 900;
            margin: 0;
            line-height: 1;
            opacity: 0.3;
        }
        p {
            font-size: 1.25rem;
            margin: 1rem 0;
            opacity: 0.9;
        }
        .domain {
            font-family: monospace;
            background: rgba(255,255,255,0.2);
            padding: 0.5rem 1rem;
            border-radius: 0.5rem;
            display: inline-block;
            margin: 1rem 0;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="status-code">503</div>
        <h1>Service Temporarily Unavailable</h1>
        <div class="domain">%s</div>
        <p>The requested service is not currently configured or is temporarily unavailable.</p>
        <p>Please check back later or contact the administrator if this problem persists.</p>
    </div>
</body>
</html>`, data.Domain)

	return http.StatusServiceUnavailable, html
}

// generateRequestID generates a simple request ID for tracking
func generateRequestID() string {
	return fmt.Sprintf("req-%d", time.Now().UnixNano())
}
