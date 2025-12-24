package certmonitor

import (
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"
)

func TestCertMonitorBasic(t *testing.T) {
	m := NewMonitor()

	// Add certificate from x509
	cert := &x509.Certificate{
		NotBefore: time.Now().Add(-24 * time.Hour),
		NotAfter:  time.Now().Add(20 * 24 * time.Hour),
		Issuer:    pkix.Name{CommonName: "Test CA"},
		Subject:   pkix.Name{CommonName: "example.com"},
		DNSNames:  []string{"example.com"},
	}
	m.AddCertificate("example.com", cert)

	info, ok := m.GetCertificate("example.com")
	if !ok || info.Domain != "example.com" {
		t.Fatalf("certificate not stored")
	}

	stats := m.GetStats()
	if stats.TotalCertificates != 1 || stats.HealthyCount+stats.WarningCount+stats.UrgentCount+stats.CriticalCount+stats.ExpiredCount < 1 {
		t.Fatalf("stats not populated correctly")
	}

	// Expiring list
	exp := m.GetExpiringCertificates(LevelWarning)
	if len(exp) == 0 {
		t.Fatalf("expected expiring certificates")
	}

	// Toggle enabled/disabled
	m.Disable()
	if m.IsEnabled() {
		t.Fatalf("monitor should be disabled")
	}
	m.Enable()

	// Remove
	m.RemoveCertificate("example.com")
	if _, ok := m.GetCertificate("example.com"); ok {
		t.Fatalf("certificate should be removed")
	}
}

func TestAddCertificateFromTLSError(t *testing.T) {
	m := NewMonitor()
	if err := m.AddCertificateFromTLS("example.com", &tls.Certificate{}); err == nil {
		t.Fatalf("expected error for empty TLS certificate")
	}
}

func TestGetAllCertificates(t *testing.T) {
	m := NewMonitor()

	// Add multiple certificates
	cert1 := &x509.Certificate{
		NotBefore: time.Now().Add(-24 * time.Hour),
		NotAfter:  time.Now().Add(20 * 24 * time.Hour),
		Subject:   pkix.Name{CommonName: "example1.com"},
		DNSNames:  []string{"example1.com"},
	}
	cert2 := &x509.Certificate{
		NotBefore: time.Now().Add(-24 * time.Hour),
		NotAfter:  time.Now().Add(30 * 24 * time.Hour),
		Subject:   pkix.Name{CommonName: "example2.com"},
		DNSNames:  []string{"example2.com"},
	}

	m.AddCertificate("example1.com", cert1)
	m.AddCertificate("example2.com", cert2)

	all := m.GetAllCertificates()
	if len(all) != 2 {
		t.Errorf("Expected 2 certificates, got %d", len(all))
	}
}

func TestGetExpiredCertificates(t *testing.T) {
	m := NewMonitor()

	// Add an expired certificate
	expiredCert := &x509.Certificate{
		NotBefore: time.Now().Add(-30 * 24 * time.Hour),
		NotAfter:  time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
		Subject:   pkix.Name{CommonName: "expired.com"},
		DNSNames:  []string{"expired.com"},
	}

	// Add a valid certificate
	validCert := &x509.Certificate{
		NotBefore: time.Now().Add(-24 * time.Hour),
		NotAfter:  time.Now().Add(30 * 24 * time.Hour),
		Subject:   pkix.Name{CommonName: "valid.com"},
		DNSNames:  []string{"valid.com"},
	}

	m.AddCertificate("expired.com", expiredCert)
	m.AddCertificate("valid.com", validCert)

	expired := m.GetExpiredCertificates()
	if len(expired) != 1 {
		t.Errorf("Expected 1 expired certificate, got %d", len(expired))
	}

	if len(expired) > 0 && expired[0].Domain != "expired.com" {
		t.Errorf("Expected expired.com, got %s", expired[0].Domain)
	}
}

func TestCheckAll(t *testing.T) {
	m := NewMonitor()

	// Add certificates
	cert1 := &x509.Certificate{
		NotBefore: time.Now().Add(-24 * time.Hour),
		NotAfter:  time.Now().Add(20 * 24 * time.Hour),
		Subject:   pkix.Name{CommonName: "example1.com"},
		DNSNames:  []string{"example1.com"},
	}
	cert2 := &x509.Certificate{
		NotBefore: time.Now().Add(-24 * time.Hour),
		NotAfter:  time.Now().Add(5 * 24 * time.Hour), // Expires in 5 days
		Subject:   pkix.Name{CommonName: "example2.com"},
		DNSNames:  []string{"example2.com"},
	}

	m.AddCertificate("example1.com", cert1)
	m.AddCertificate("example2.com", cert2)

	m.CheckAll()

	// After CheckAll, should be able to get stats
	stats := m.GetStats()
	if stats.TotalCertificates != 2 {
		t.Errorf("Expected 2 total certificates, got %d", stats.TotalCertificates)
	}
}
