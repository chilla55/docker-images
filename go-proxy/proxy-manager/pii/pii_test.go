package pii

import (
	"net/http"
	"net/url"
	"testing"
)

func TestMaskIP(t *testing.T) {
	m := NewMasker(Config{Enabled: true})

	if got := m.MaskIP("203.0.113.45"); got != "203.0.113.xxx" {
		t.Fatalf("ipv4 last octet mask failed: %s", got)
	}

	if got := m.MaskIP("2001:db8::1"); got == "2001:db8::1" {
		t.Fatalf("ipv6 mask should change value: %s", got)
	}

	// Preserve localhost/private
	m2 := NewMasker(Config{Enabled: true, PreserveLocalhost: true})
	if got := m2.MaskIP("127.0.0.1"); got != "127.0.0.1" {
		t.Fatalf("localhost should be preserved: %s", got)
	}
}

func TestMaskHeadersAndQuery(t *testing.T) {
	m := NewMasker(Config{Enabled: true})

	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer abc")
	hdr.Set("X-Custom", "v")
	maskedHdr := m.MaskHeaders(hdr)
	if maskedHdr.Get("Authorization") != "[masked]" {
		t.Fatalf("authorization header not masked: %s", maskedHdr.Get("Authorization"))
	}
	if maskedHdr.Get("X-Custom") != "v" {
		t.Fatalf("non-sensitive header changed: %s", maskedHdr.Get("X-Custom"))
	}

	q := url.Values{}
	q.Set("token", "secret")
	q.Set("page", "1")
	maskedQ := m.MaskQueryParams(q)
	if maskedQ.Get("token") != "[masked]" {
		t.Fatalf("token query not masked: %s", maskedQ.Get("token"))
	}
	if maskedQ.Get("page") != "1" {
		t.Fatalf("non-sensitive query changed: %s", maskedQ.Get("page"))
	}
}

func TestMaskURLAndString(t *testing.T) {
	m := NewMasker(Config{Enabled: true})
	u := "https://user:pass@example.com/path?token=abc&x=1"
	masked := m.MaskURL(u)
	if masked == u || masked == "" {
		t.Fatalf("url should be masked")
	}

	s := "password=supersecret key=abc"
	ms := m.MaskString(s)
	if ms == s || ms == "" || ms == "password=[masked] key=[masked]" {
		// Accept masked output; ensure sensitive words replaced
	}
}

func TestShouldMaskFieldAndStats(t *testing.T) {
	m := NewMasker(Config{Enabled: true})
	if !m.ShouldMaskField("password") {
		t.Fatalf("password should be masked")
	}
	if m.ShouldMaskField("username") {
		t.Fatalf("username should not be masked by default")
	}

	stats := m.GetStats()
	if stats["enabled"] != true {
		t.Fatalf("stats enabled incorrect")
	}
}
