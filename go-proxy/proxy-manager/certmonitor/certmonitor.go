package certmonitor

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Monitor tracks SSL/TLS certificate expiry dates
type Monitor struct {
	certs      map[string]*CertInfo
	certsMutex sync.RWMutex
	enabled    bool
}

// CertInfo represents SSL/TLS certificate information
type CertInfo struct {
	Domain        string    `json:"domain"`
	Issuer        string    `json:"issuer"`
	Subject       string    `json:"subject"`
	NotBefore     time.Time `json:"not_before"`
	NotAfter      time.Time `json:"not_after"`
	DaysRemaining int       `json:"days_remaining"`
	WarningLevel  string    `json:"warning_level"` // ok, warning, urgent, critical
	SerialNumber  string    `json:"serial_number"`
	SignatureAlgo string    `json:"signature_algorithm"`
	PublicKeyAlgo string    `json:"public_key_algorithm"`
	DNSNames      []string  `json:"dns_names"`
	LastChecked   time.Time `json:"last_checked"`
}

// WarningLevel constants
const (
	LevelOK       = "ok"
	LevelWarning  = "warning"  // 30 days
	LevelUrgent   = "urgent"   // 14 days
	LevelCritical = "critical" // 7 days
)

// NewMonitor creates a new certificate monitor
func NewMonitor() *Monitor {
	return &Monitor{
		certs:   make(map[string]*CertInfo),
		enabled: true,
	}
}

// AddCertificate adds or updates a certificate for monitoring
func (m *Monitor) AddCertificate(domain string, cert *x509.Certificate) {
	if !m.enabled || cert == nil {
		return
	}

	info := m.parseCertificate(domain, cert)

	m.certsMutex.Lock()
	m.certs[domain] = info
	m.certsMutex.Unlock()

	// Log warnings for expiring certificates
	if info.WarningLevel != LevelOK {
		log.Warn().
			Str("domain", domain).
			Str("warning_level", info.WarningLevel).
			Int("days_remaining", info.DaysRemaining).
			Time("expires", info.NotAfter).
			Msg("Certificate expiring soon")
	}
}

// AddCertificateFromTLS adds a certificate from tls.Certificate
func (m *Monitor) AddCertificateFromTLS(domain string, tlsCert *tls.Certificate) error {
	if tlsCert == nil || len(tlsCert.Certificate) == 0 {
		return fmt.Errorf("invalid TLS certificate")
	}

	// Parse the first certificate in the chain (leaf certificate)
	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	m.AddCertificate(domain, cert)
	return nil
}

// parseCertificate extracts information from an x509 certificate
func (m *Monitor) parseCertificate(domain string, cert *x509.Certificate) *CertInfo {
	now := time.Now()
	daysRemaining := int(time.Until(cert.NotAfter).Hours() / 24)

	info := &CertInfo{
		Domain:        domain,
		Issuer:        cert.Issuer.String(),
		Subject:       cert.Subject.String(),
		NotBefore:     cert.NotBefore,
		NotAfter:      cert.NotAfter,
		DaysRemaining: daysRemaining,
		SerialNumber:  cert.SerialNumber.String(),
		SignatureAlgo: cert.SignatureAlgorithm.String(),
		PublicKeyAlgo: cert.PublicKeyAlgorithm.String(),
		DNSNames:      cert.DNSNames,
		LastChecked:   now,
	}

	// Calculate warning level
	switch {
	case daysRemaining <= 7:
		info.WarningLevel = LevelCritical
	case daysRemaining <= 14:
		info.WarningLevel = LevelUrgent
	case daysRemaining <= 30:
		info.WarningLevel = LevelWarning
	default:
		info.WarningLevel = LevelOK
	}

	return info
}

// GetCertificate returns certificate info for a domain
func (m *Monitor) GetCertificate(domain string) (*CertInfo, bool) {
	m.certsMutex.RLock()
	defer m.certsMutex.RUnlock()

	info, exists := m.certs[domain]
	return info, exists
}

// GetAllCertificates returns all monitored certificates
func (m *Monitor) GetAllCertificates() map[string]*CertInfo {
	m.certsMutex.RLock()
	defer m.certsMutex.RUnlock()

	// Create a copy to avoid race conditions
	result := make(map[string]*CertInfo, len(m.certs))
	for domain, info := range m.certs {
		// Create a copy of the cert info
		infoCopy := *info
		result[domain] = &infoCopy
	}

	return result
}

// GetExpiringCertificates returns certificates that are expiring soon
func (m *Monitor) GetExpiringCertificates(minWarningLevel string) []*CertInfo {
	m.certsMutex.RLock()
	defer m.certsMutex.RUnlock()

	var expiring []*CertInfo

	for _, info := range m.certs {
		// Filter by warning level
		include := false
		switch minWarningLevel {
		case LevelCritical:
			include = info.WarningLevel == LevelCritical
		case LevelUrgent:
			include = info.WarningLevel == LevelUrgent || info.WarningLevel == LevelCritical
		case LevelWarning:
			include = info.WarningLevel != LevelOK
		default:
			include = true
		}

		if include {
			infoCopy := *info
			expiring = append(expiring, &infoCopy)
		}
	}

	return expiring
}

// GetExpiredCertificates returns certificates that have already expired
func (m *Monitor) GetExpiredCertificates() []*CertInfo {
	m.certsMutex.RLock()
	defer m.certsMutex.RUnlock()

	var expired []*CertInfo
	now := time.Now()

	for _, info := range m.certs {
		if info.NotAfter.Before(now) {
			infoCopy := *info
			expired = append(expired, &infoCopy)
		}
	}

	return expired
}

// GetStats returns certificate monitoring statistics
func (m *Monitor) GetStats() CertStats {
	m.certsMutex.RLock()
	defer m.certsMutex.RUnlock()

	stats := CertStats{
		TotalCertificates: len(m.certs),
	}

	now := time.Now()

	for _, info := range m.certs {
		if info.NotAfter.Before(now) {
			stats.ExpiredCount++
		}

		switch info.WarningLevel {
		case LevelCritical:
			stats.CriticalCount++
		case LevelUrgent:
			stats.UrgentCount++
		case LevelWarning:
			stats.WarningCount++
		case LevelOK:
			stats.HealthyCount++
		}
	}

	return stats
}

// CertStats represents certificate monitoring statistics
type CertStats struct {
	TotalCertificates int `json:"total_certificates"`
	HealthyCount      int `json:"healthy_count"`
	WarningCount      int `json:"warning_count"`
	UrgentCount       int `json:"urgent_count"`
	CriticalCount     int `json:"critical_count"`
	ExpiredCount      int `json:"expired_count"`
}

// RemoveCertificate removes a certificate from monitoring
func (m *Monitor) RemoveCertificate(domain string) {
	m.certsMutex.Lock()
	defer m.certsMutex.Unlock()

	delete(m.certs, domain)
	log.Info().Str("domain", domain).Msg("Certificate removed from monitoring")
}

// Enable enables certificate monitoring
func (m *Monitor) Enable() {
	m.enabled = true
	log.Info().Msg("Certificate monitoring enabled")
}

// Disable disables certificate monitoring
func (m *Monitor) Disable() {
	m.enabled = false
	log.Info().Msg("Certificate monitoring disabled")
}

// IsEnabled returns whether certificate monitoring is enabled
func (m *Monitor) IsEnabled() bool {
	return m.enabled
}

// CheckAll rechecks all certificates and updates their expiry info
func (m *Monitor) CheckAll() {
	m.certsMutex.Lock()
	defer m.certsMutex.Unlock()

	now := time.Now()

	for domain, info := range m.certs {
		// Recalculate days remaining
		daysRemaining := int(time.Until(info.NotAfter).Hours() / 24)
		info.DaysRemaining = daysRemaining
		info.LastChecked = now

		// Recalculate warning level
		switch {
		case daysRemaining <= 7:
			info.WarningLevel = LevelCritical
		case daysRemaining <= 14:
			info.WarningLevel = LevelUrgent
		case daysRemaining <= 30:
			info.WarningLevel = LevelWarning
		default:
			info.WarningLevel = LevelOK
		}

		// Log if status changed to warning/critical
		if info.WarningLevel != LevelOK {
			log.Warn().
				Str("domain", domain).
				Str("warning_level", info.WarningLevel).
				Int("days_remaining", daysRemaining).
				Time("expires", info.NotAfter).
				Msg("Certificate expiring soon")
		}
	}

	log.Debug().Int("count", len(m.certs)).Msg("Rechecked all certificates")
}

// StartPeriodicCheck starts periodic certificate expiry checks
func (m *Monitor) StartPeriodicCheck(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Info().
			Dur("interval", interval).
			Msg("Started periodic certificate expiry checks")

		for range ticker.C {
			if m.enabled {
				m.CheckAll()
			}
		}
	}()
}
