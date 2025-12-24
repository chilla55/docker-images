package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
)

// TestRealClientIPExtraction tests that we correctly extract the real client IP
// from Cloudflare headers and X-Forwarded-For chains
func TestRealClientIPExtraction(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		cfConnectingIP string
		xForwardedFor  string
		expectedRealIP string
	}{
		{
			name:           "CF-Connecting-IP present (Cloudflare)",
			remoteAddr:     "172.68.1.1:54321",
			cfConnectingIP: "203.0.113.45",
			xForwardedFor:  "203.0.113.45",
			expectedRealIP: "203.0.113.45",
		},
		{
			name:           "X-Forwarded-For with single IP",
			remoteAddr:     "172.68.1.1:54321",
			cfConnectingIP: "",
			xForwardedFor:  "198.51.100.23",
			expectedRealIP: "198.51.100.23",
		},
		{
			name:           "X-Forwarded-For with chain",
			remoteAddr:     "172.68.1.1:54321",
			cfConnectingIP: "",
			xForwardedFor:  "198.51.100.23, 203.0.113.1",
			expectedRealIP: "198.51.100.23",
		},
		{
			name:           "X-Forwarded-For with spaces",
			remoteAddr:     "172.68.1.1:54321",
			cfConnectingIP: "",
			xForwardedFor:  "  198.51.100.23  ,  203.0.113.1  ",
			expectedRealIP: "198.51.100.23",
		},
		{
			name:           "No headers - fallback to RemoteAddr",
			remoteAddr:     "198.51.100.99:54321",
			cfConnectingIP: "",
			xForwardedFor:  "",
			expectedRealIP: "198.51.100.99",
		},
		{
			name:           "CF-Connecting-IP takes precedence",
			remoteAddr:     "172.68.1.1:54321",
			cfConnectingIP: "203.0.113.45",
			xForwardedFor:  "198.51.100.23",
			expectedRealIP: "203.0.113.45",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create backend target
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify X-Real-IP received by backend
				realIP := r.Header.Get("X-Real-IP")
				if realIP != tt.expectedRealIP {
					t.Errorf("X-Real-IP = %q, want %q", realIP, tt.expectedRealIP)
				}

				// Verify X-Forwarded-For contains our proxy IP
				xff := r.Header.Get("X-Forwarded-For")
				proxyIP := stripPort(tt.remoteAddr)
				if !strings.Contains(xff, proxyIP) {
					t.Errorf("X-Forwarded-For %q should contain proxy IP %q", xff, proxyIP)
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			backendURL, _ := url.Parse(backend.URL)

			// Create proxy with our director logic
			proxy := &Backend{
				URL: backendURL,
			}
			proxy.Proxy = createTestProxy(backendURL)

			// Create test request
			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			req.RemoteAddr = tt.remoteAddr

			// Set headers
			if tt.cfConnectingIP != "" {
				req.Header.Set("CF-Connecting-IP", tt.cfConnectingIP)
			}
			if tt.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwardedFor)
			}

			// Make request through proxy
			rr := httptest.NewRecorder()
			proxy.Proxy.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Status = %d, want %d", rr.Code, http.StatusOK)
			}
		})
	}
}

// TestIPv6AddressHandling tests that we correctly handle IPv6 addresses
func TestIPv6AddressHandling(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		expectedRealIP string
	}{
		{
			name:           "IPv6 with port",
			remoteAddr:     "[2001:db8::1]:54321",
			expectedRealIP: "2001:db8::1",
		},
		{
			name:           "IPv4-mapped IPv6",
			remoteAddr:     "[::ffff:192.0.2.1]:54321",
			expectedRealIP: "::ffff:192.0.2.1",
		},
		{
			name:           "IPv6 localhost",
			remoteAddr:     "[::1]:54321",
			expectedRealIP: "::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				realIP := r.Header.Get("X-Real-IP")
				if realIP != tt.expectedRealIP {
					t.Errorf("X-Real-IP = %q, want %q", realIP, tt.expectedRealIP)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			backendURL, _ := url.Parse(backend.URL)
			proxy := &Backend{URL: backendURL}
			proxy.Proxy = createTestProxy(backendURL)

			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			req.RemoteAddr = tt.remoteAddr

			rr := httptest.NewRecorder()
			proxy.Proxy.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Status = %d, want %d", rr.Code, http.StatusOK)
			}
		})
	}
}

// TestXForwardedForChainPreservation tests that existing X-Forwarded-For chains
// are preserved and our proxy IP is appended correctly
func TestXForwardedForChainPreservation(t *testing.T) {
	tests := []struct {
		name           string
		remoteAddr     string
		existingXFF    string
		cfConnectingIP string
		expectedRealIP string
		shouldContain  []string // IPs that should be in the final XFF chain
	}{
		{
			name:           "Preserve Cloudflare chain",
			remoteAddr:     "172.68.1.1:54321",
			existingXFF:    "203.0.113.45, 172.68.2.1",
			cfConnectingIP: "203.0.113.45",
			expectedRealIP: "203.0.113.45",
			shouldContain:  []string{"203.0.113.45", "172.68.2.1", "172.68.1.1"},
		},
		{
			name:           "Preserve multi-proxy chain",
			remoteAddr:     "10.0.0.1:54321",
			existingXFF:    "203.0.113.45, 172.68.1.1, 172.68.2.1",
			cfConnectingIP: "",
			expectedRealIP: "203.0.113.45",
			shouldContain:  []string{"203.0.113.45", "172.68.1.1", "172.68.2.1", "10.0.0.1"},
		},
		{
			name:           "Start new chain",
			remoteAddr:     "10.0.0.1:54321",
			existingXFF:    "",
			cfConnectingIP: "",
			expectedRealIP: "10.0.0.1",
			shouldContain:  []string{"10.0.0.1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				xff := r.Header.Get("X-Forwarded-For")

				// Check that all expected IPs are in the chain
				for _, ip := range tt.shouldContain {
					if !strings.Contains(xff, ip) {
						t.Errorf("X-Forwarded-For %q should contain %q", xff, ip)
					}
				}

				realIP := r.Header.Get("X-Real-IP")
				if realIP != tt.expectedRealIP {
					t.Errorf("X-Real-IP = %q, want %q", realIP, tt.expectedRealIP)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer backend.Close()

			backendURL, _ := url.Parse(backend.URL)
			proxy := &Backend{URL: backendURL}
			proxy.Proxy = createTestProxy(backendURL)

			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.existingXFF != "" {
				req.Header.Set("X-Forwarded-For", tt.existingXFF)
			}
			if tt.cfConnectingIP != "" {
				req.Header.Set("CF-Connecting-IP", tt.cfConnectingIP)
			}

			rr := httptest.NewRecorder()
			proxy.Proxy.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Status = %d, want %d", rr.Code, http.StatusOK)
			}
		})
	}
}

// Helper function to create a test proxy with our IP extraction logic
func createTestProxy(target *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director

	proxy.Director = func(req *http.Request) {
		// Save original host before director changes it
		originalHost := req.Host

		// Extract real client IP (handling Cloudflare proxy)
		realClientIP := ""
		// 1. Try CF-Connecting-IP (Cloudflare's real client IP header)
		if cfIP := req.Header.Get("CF-Connecting-IP"); cfIP != "" {
			realClientIP = cfIP
		}
		// 2. Try first IP in X-Forwarded-For (standard proxy chain)
		if realClientIP == "" {
			if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
				if idx := strings.Index(xff, ","); idx > 0 {
					realClientIP = strings.TrimSpace(xff[:idx])
				} else {
					realClientIP = strings.TrimSpace(xff)
				}
			}
		}
		// 3. Fallback to immediate connection IP
		if realClientIP == "" {
			realClientIP = stripPort(req.RemoteAddr)
		}

		// Get proxy IP (immediate connection, likely Cloudflare)
		proxyIP := stripPort(req.RemoteAddr)

		// Call original director (this sets req.Host to backend host)
		originalDirector(req)

		// Add/preserve X-Forwarded headers (handling Cloudflare proxy)
		if req.Header.Get("X-Forwarded-Host") == "" {
			req.Header.Set("X-Forwarded-Host", originalHost)
		}
		if req.Header.Get("X-Forwarded-Proto") == "" {
			req.Header.Set("X-Forwarded-Proto", "https")
		}
		if req.Header.Get("X-Real-IP") == "" {
			req.Header.Set("X-Real-IP", realClientIP)
		}
		// Append proxy IP to X-Forwarded-For chain (preserves Cloudflare chain + adds our proxy)
		if existing := req.Header.Get("X-Forwarded-For"); existing != "" {
			req.Header.Set("X-Forwarded-For", existing+", "+proxyIP)
		} else {
			req.Header.Set("X-Forwarded-For", realClientIP)
		}
	}

	return proxy
}
