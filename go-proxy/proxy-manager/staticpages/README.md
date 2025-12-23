# Static Pages Package

The `staticpages` package provides a centralized way to manage all static HTTP content for the go-proxy reverse proxy. This includes error pages, maintenance pages, and service unavailable pages.

## Features

- **Single Source of Truth**: All static HTML content is managed in one place
- **Type-Safe**: Uses constants for page types to prevent typos
- **Flexible**: Supports custom content and fallback pages
- **Consistent Styling**: All pages share a modern, consistent design
- **Status Code Mapping**: Automatically maps HTTP status codes to appropriate pages

## Usage

### Basic Example

```go
import "github.com/chilla55/proxy-manager/staticpages"

// Get a 404 page
data := staticpages.PageData{
    Domain:    "example.com",
    Path:      "/missing-page",
    RequestID: "req-123",
}
statusCode, html := staticpages.GetPage(staticpages.PageError404, data)

// Write to HTTP response
w.WriteHeader(statusCode)
w.Header().Set("Content-Type", "text/html; charset=utf-8")
w.Write([]byte(html))
```

### Using Status Codes

```go
// Automatically select the appropriate page based on status code
data := staticpages.PageData{
    Domain:  "api.example.com",
    Path:    "/v1/users",
    Message: "Rate limit exceeded",
}
statusCode, html := staticpages.GetPageByStatusCode(429, data)
```

### Maintenance Pages

```go
// Default maintenance page
data := staticpages.PageData{
    Domain:       "example.com",
    Reason:       "Scheduled database upgrade",
    ScheduledEnd: "2024-01-15 14:00:00 UTC",
}
statusCode, html := staticpages.GetPage(staticpages.PageMaintenanceDefault, data)

// Custom maintenance page
data := staticpages.PageData{
    CustomContent: "<html><body><h1>Custom Maintenance Page</h1></body></html>",
}
statusCode, html := staticpages.GetPage(staticpages.PageMaintenanceDefault, data)
```

## Available Page Types

### Error Pages
- `PageError400` - Bad Request
- `PageError401` - Unauthorized
- `PageError403` - Forbidden
- `PageError404` - Not Found
- `PageError500` - Internal Server Error
- `PageError502` - Bad Gateway
- `PageError503` - Service Unavailable
- `PageError504` - Gateway Timeout

### Maintenance Pages
- `PageMaintenanceDefault` - Styled maintenance page with reason and end time
- `PageMaintenanceFallback` - Minimal fallback maintenance page

### Service Pages
- `PageServiceUnavailable` - Service unavailable with domain display
- `PageGenericError` - Generic error page for unknown status codes

## PageData Structure

```go
type PageData struct {
    StatusCode    int       // HTTP status code
    StatusText    string    // HTTP status text (e.g., "Not Found")
    Message       string    // Custom error/info message
    Domain        string    // Domain name
    Path          string    // Request path
    RequestID     string    // Unique request identifier
    Timestamp     time.Time // Request timestamp
    Reason        string    // Maintenance reason
    ScheduledEnd  string    // Maintenance end time
    CustomContent string    // Custom HTML content
}
```

## Design Philosophy

All pages follow these design principles:
- **Modern UI**: Clean, gradient backgrounds with card-based layouts
- **Responsive**: Mobile-friendly designs
- **Accessible**: Semantic HTML with proper contrast ratios
- **Informative**: Displays relevant debugging information (domain, path, request ID, timestamp)
- **User-Friendly**: Clear error messages with actionable buttons

## Integration

The staticpages package is used by:
- **errorpage**: Error page rendering with template support
- **maintenance**: Maintenance mode pages
- **proxy**: Service unavailable and maintenance fallback pages

## Testing

Run tests with:
```bash
go test ./staticpages/ -v
```

Run benchmarks:
```bash
go test ./staticpages/ -bench=. -benchmem
```

## Performance

- Pages are generated on-the-fly using `fmt.Sprintf`
- No disk I/O required
- Typical generation time: < 10µs per page
- Zero allocation for page type lookups

## Examples

### Integration with http.HandlerFunc

```go
func errorHandler(w http.ResponseWriter, r *http.Request) {
    data := staticpages.PageData{
        Domain:    r.Host,
        Path:      r.URL.Path,
        Message:   "The requested resource was not found",
        Timestamp: time.Now(),
    }
    
    status, html := staticpages.GetPage(staticpages.PageError404, data)
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    w.WriteHeader(status)
    io.WriteString(w, html)
}
```

### Fallback Error Handling

```go
// If page type is unknown, it automatically falls back to PageGenericError
status, html := staticpages.GetPage("unknown_page_type", data)
// Returns a generic error page with status 500
```

## Future Enhancements

Potential improvements:
- Template caching for frequently used pages
- Internationalization (i18n) support
- Custom CSS theme support
- Dark mode variants
- Accessibility improvements (ARIA labels)
