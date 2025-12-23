package geoip

import (
	"net"
	"sync"
	"time"

	"github.com/oschwald/geoip2-golang"
	"github.com/rs/zerolog/log"
)

// Tracker handles GeoIP lookups for client IP addresses
type Tracker struct {
	db                    *geoip2.Reader
	enabled               bool
	alertOnUnusualCountry bool
	expectedCountries     map[string]bool // Expected countries for this service
	locationCache         map[string]*Location
	cacheMutex            sync.RWMutex
	stats                 Stats
	statsMutex            sync.RWMutex
	cacheExpiry           time.Duration
}

// Location represents a GeoIP lookup result
type Location struct {
	IP          string    `json:"ip"`
	CountryCode string    `json:"country_code"` // ISO 3166-1 alpha-2 (e.g., "DE", "US")
	CountryName string    `json:"country_name"`
	City        string    `json:"city"`
	Latitude    float64   `json:"latitude"`
	Longitude   float64   `json:"longitude"`
	Timezone    string    `json:"timezone"`
	Timestamp   time.Time `json:"timestamp"`
}

// Stats tracks GeoIP system statistics
type Stats struct {
	LookupsTotal        int64            `json:"lookups_total"`
	LookupsSuccess      int64            `json:"lookups_success"`
	LookupsFailed       int64            `json:"lookups_failed"`
	CacheHits           int64            `json:"cache_hits"`
	CacheMisses         int64            `json:"cache_misses"`
	UnusualCountries    int64            `json:"unusual_countries"`
	CountryDistribution map[string]int64 `json:"country_distribution"`
}

// Config represents GeoIP configuration
type Config struct {
	Enabled               bool     `yaml:"enabled"`
	DatabasePath          string   `yaml:"database_path"`
	AlertOnUnusualCountry bool     `yaml:"alert_on_unusual_country"`
	ExpectedCountries     []string `yaml:"expected_countries"` // Empty = all allowed
	CacheExpiryMinutes    int      `yaml:"cache_expiry_minutes"`
}

// New creates a new GeoIP tracker
func New(config Config) (*Tracker, error) {
	if !config.Enabled {
		log.Info().Msg("GeoIP tracking disabled")
		return &Tracker{enabled: false}, nil
	}

	// Default database path
	if config.DatabasePath == "" {
		config.DatabasePath = "/data/GeoLite2-City.mmdb"
	}

	// Default cache expiry
	cacheExpiry := 30 * time.Minute
	if config.CacheExpiryMinutes > 0 {
		cacheExpiry = time.Duration(config.CacheExpiryMinutes) * time.Minute
	}

	// Open GeoIP database
	db, err := geoip2.Open(config.DatabasePath)
	if err != nil {
		log.Error().Err(err).Str("path", config.DatabasePath).Msg("Failed to open GeoIP database")
		return nil, err
	}

	// Build expected countries map
	expectedCountries := make(map[string]bool)
	for _, country := range config.ExpectedCountries {
		expectedCountries[country] = true
	}

	tracker := &Tracker{
		db:                    db,
		enabled:               true,
		alertOnUnusualCountry: config.AlertOnUnusualCountry,
		expectedCountries:     expectedCountries,
		locationCache:         make(map[string]*Location),
		cacheExpiry:           cacheExpiry,
		stats: Stats{
			CountryDistribution: make(map[string]int64),
		},
	}

	log.Info().
		Str("database", config.DatabasePath).
		Bool("alert_unusual", config.AlertOnUnusualCountry).
		Int("expected_countries", len(expectedCountries)).
		Msg("GeoIP tracker initialized")

	return tracker, nil
}

// Lookup performs a GeoIP lookup for the given IP address
func (t *Tracker) Lookup(ipStr string) (*Location, error) {
	if !t.enabled {
		return nil, nil
	}

	// Update stats
	t.statsMutex.Lock()
	t.stats.LookupsTotal++
	t.statsMutex.Unlock()

	// Check cache first
	t.cacheMutex.RLock()
	if cached, ok := t.locationCache[ipStr]; ok {
		// Check if cache entry is still valid
		if time.Since(cached.Timestamp) < t.cacheExpiry {
			t.cacheMutex.RUnlock()
			t.statsMutex.Lock()
			t.stats.CacheHits++
			t.statsMutex.Unlock()
			return cached, nil
		}
	}
	t.cacheMutex.RUnlock()

	// Cache miss - perform lookup
	t.statsMutex.Lock()
	t.stats.CacheMisses++
	t.statsMutex.Unlock()

	ip := net.ParseIP(ipStr)
	if ip == nil {
		t.statsMutex.Lock()
		t.stats.LookupsFailed++
		t.statsMutex.Unlock()
		return nil, nil
	}

	// Perform GeoIP lookup
	record, err := t.db.City(ip)
	if err != nil {
		t.statsMutex.Lock()
		t.stats.LookupsFailed++
		t.statsMutex.Unlock()
		return nil, err
	}

	location := &Location{
		IP:          ipStr,
		CountryCode: record.Country.IsoCode,
		CountryName: record.Country.Names["en"],
		City:        record.City.Names["en"],
		Latitude:    record.Location.Latitude,
		Longitude:   record.Location.Longitude,
		Timezone:    record.Location.TimeZone,
		Timestamp:   time.Now(),
	}

	// Update stats
	t.statsMutex.Lock()
	t.stats.LookupsSuccess++
	t.stats.CountryDistribution[location.CountryCode]++
	t.statsMutex.Unlock()

	// Check for unusual country
	if t.alertOnUnusualCountry && len(t.expectedCountries) > 0 {
		if !t.expectedCountries[location.CountryCode] {
			t.statsMutex.Lock()
			t.stats.UnusualCountries++
			t.statsMutex.Unlock()

			log.Warn().
				Str("ip", ipStr).
				Str("country", location.CountryCode).
				Str("city", location.City).
				Msg("Unusual country detected")
		}
	}

	// Cache the result
	t.cacheMutex.Lock()
	t.locationCache[ipStr] = location
	t.cacheMutex.Unlock()

	return location, nil
}

// GetStats returns current GeoIP statistics
func (t *Tracker) GetStats() Stats {
	if !t.enabled {
		return Stats{
			CountryDistribution: make(map[string]int64),
		}
	}

	t.statsMutex.RLock()
	defer t.statsMutex.RUnlock()

	// Create a copy of the stats
	stats := Stats{
		LookupsTotal:        t.stats.LookupsTotal,
		LookupsSuccess:      t.stats.LookupsSuccess,
		LookupsFailed:       t.stats.LookupsFailed,
		CacheHits:           t.stats.CacheHits,
		CacheMisses:         t.stats.CacheMisses,
		UnusualCountries:    t.stats.UnusualCountries,
		CountryDistribution: make(map[string]int64),
	}

	for country, count := range t.stats.CountryDistribution {
		stats.CountryDistribution[country] = count
	}

	return stats
}

// GetTopCountries returns the top N countries by request count
func (t *Tracker) GetTopCountries(limit int) []struct {
	Country string
	Count   int64
} {
	if !t.enabled {
		return nil
	}

	t.statsMutex.RLock()
	defer t.statsMutex.RUnlock()

	// Convert map to slice for sorting
	type countryCount struct {
		Country string
		Count   int64
	}

	var countries []countryCount
	for country, count := range t.stats.CountryDistribution {
		countries = append(countries, countryCount{Country: country, Count: count})
	}

	// Simple bubble sort (good enough for small N)
	for i := 0; i < len(countries); i++ {
		for j := i + 1; j < len(countries); j++ {
			if countries[j].Count > countries[i].Count {
				countries[i], countries[j] = countries[j], countries[i]
			}
		}
	}

	// Limit results
	if limit > 0 && limit < len(countries) {
		countries = countries[:limit]
	}

	// Convert back to result format
	result := make([]struct {
		Country string
		Count   int64
	}, len(countries))

	for i, cc := range countries {
		result[i].Country = cc.Country
		result[i].Count = cc.Count
	}

	return result
}

// ClearCache removes all cached entries
func (t *Tracker) ClearCache() {
	if !t.enabled {
		return
	}

	t.cacheMutex.Lock()
	t.locationCache = make(map[string]*Location)
	t.cacheMutex.Unlock()

	log.Info().Msg("GeoIP cache cleared")
}

// IsEnabled returns whether GeoIP tracking is enabled
func (t *Tracker) IsEnabled() bool {
	return t.enabled
}

// Close closes the GeoIP database
func (t *Tracker) Close() error {
	if !t.enabled || t.db == nil {
		return nil
	}
	return t.db.Close()
}
