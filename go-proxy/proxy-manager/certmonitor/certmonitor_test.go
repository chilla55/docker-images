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
    if _, ok := m.GetCertificate("example.com"); ok { t.Fatalf("certificate should be removed") }
}

func TestAddCertificateFromTLSError(t *testing.T) {
    m := NewMonitor()
    if err := m.AddCertificateFromTLS("example.com", &tls.Certificate{}); err == nil {
        t.Fatalf("expected error for empty TLS certificate")
    }
}
