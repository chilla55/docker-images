package geoip

import (
	"os"
	"testing"
	"time"
)

func TestNew_Disabled(t *testing.T) {
	config := Config{
		Enabled: false,
	}

	tracker, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create disabled tracker: %v", err)
	}

	if tracker.IsEnabled() {
		t.Error("Tracker should be disabled")
	}

	// Operations should be safe on disabled tracker
	loc, err := tracker.Lookup("1.1.1.1")
	if err != nil {
		t.Errorf("Lookup on disabled tracker should not error: %v", err)
	}
	if loc != nil {
		t.Error("Lookup on disabled tracker should return nil")
	}

	stats := tracker.GetStats()
	if stats.LookupsTotal != 0 {
		t.Error("Disabled tracker should have zero stats")
	}
}

func TestNew_MissingDatabase(t *testing.T) {
	config := Config{
		Enabled:      true,
		DatabasePath: "/nonexistent/GeoLite2-City.mmdb",
	}

	tracker, err := New(config)
	if err == nil {
		t.Error("Expected error for missing database")
		if tracker != nil {
			tracker.Close()
		}
	}
}

func TestNew_DefaultPaths(t *testing.T) {
	config := Config{
		Enabled:      true,
		DatabasePath: "", // Should use default
	}

	// This will fail unless the database exists, which is expected
	tracker, err := New(config)
	if err == nil {
		// If database exists, verify defaults
		if tracker.cacheExpiry != 30*time.Minute {
			t.Errorf("Expected default cache expiry 30m, got %v", tracker.cacheExpiry)
		}
		tracker.Close()
	}
}

func TestNew_CustomCacheExpiry(t *testing.T) {
	config := Config{
		Enabled:            true,
		DatabasePath:       "/nonexistent.mmdb",
		CacheExpiryMinutes: 60,
	}

	// Will fail on database open, but we can test the error path
	_, err := New(config)
	if err == nil {
		t.Error("Expected error for nonexistent database")
	}
}

func TestTracker_Lookup_Disabled(t *testing.T) {
	tracker := &Tracker{enabled: false}

	loc, err := tracker.Lookup("8.8.8.8")
	if err != nil {
		t.Errorf("Lookup on disabled tracker should not error: %v", err)
	}
	if loc != nil {
		t.Error("Lookup on disabled tracker should return nil")
	}
}

func TestTracker_GetStats_Disabled(t *testing.T) {
	tracker := &Tracker{enabled: false}

	stats := tracker.GetStats()
	if stats.LookupsTotal != 0 {
		t.Error("Disabled tracker should have zero stats")
	}
	if len(stats.CountryDistribution) != 0 {
		t.Error("Disabled tracker should have empty country distribution")
	}
}

func TestTracker_GetTopCountries_Disabled(t *testing.T) {
	tracker := &Tracker{enabled: false}

	countries := tracker.GetTopCountries(10)
	if countries != nil {
		t.Error("Disabled tracker should return nil for top countries")
	}
}

func TestTracker_GetTopCountries_Sorting(t *testing.T) {
	tracker := &Tracker{
		enabled: true,
		stats: Stats{
			CountryDistribution: map[string]int64{
				"US": 100,
				"DE": 50,
				"FR": 75,
				"GB": 25,
			},
		},
	}

	countries := tracker.GetTopCountries(3)
	if len(countries) != 3 {
		t.Fatalf("Expected 3 countries, got %d", len(countries))
	}

	// Should be sorted by count descending
	if countries[0].Country != "US" || countries[0].Count != 100 {
		t.Errorf("Expected US with 100, got %s with %d", countries[0].Country, countries[0].Count)
	}
	if countries[1].Country != "FR" || countries[1].Count != 75 {
		t.Errorf("Expected FR with 75, got %s with %d", countries[1].Country, countries[1].Count)
	}
	if countries[2].Country != "DE" || countries[2].Count != 50 {
		t.Errorf("Expected DE with 50, got %s with %d", countries[2].Country, countries[2].Count)
	}
}

func TestTracker_GetTopCountries_NoLimit(t *testing.T) {
	tracker := &Tracker{
		enabled: true,
		stats: Stats{
			CountryDistribution: map[string]int64{
				"US": 100,
				"DE": 50,
			},
		},
	}

	countries := tracker.GetTopCountries(0)
	if len(countries) != 2 {
		t.Fatalf("Expected 2 countries, got %d", len(countries))
	}
}

func TestTracker_ClearCache(t *testing.T) {
	tracker := &Tracker{
		enabled:       true,
		locationCache: make(map[string]*Location),
	}

	// Add some cache entries
	tracker.locationCache["1.1.1.1"] = &Location{IP: "1.1.1.1"}
	tracker.locationCache["8.8.8.8"] = &Location{IP: "8.8.8.8"}

	if len(tracker.locationCache) != 2 {
		t.Fatal("Cache should have 2 entries")
	}

	tracker.ClearCache()

	if len(tracker.locationCache) != 0 {
		t.Error("Cache should be empty after clear")
	}
}

func TestTracker_ClearCache_Disabled(t *testing.T) {
	tracker := &Tracker{enabled: false}
	tracker.ClearCache() // Should not panic
}

func TestTracker_Close_Disabled(t *testing.T) {
	tracker := &Tracker{enabled: false}
	err := tracker.Close()
	if err != nil {
		t.Errorf("Close on disabled tracker should not error: %v", err)
	}
}

func TestTracker_Close_NoDatabase(t *testing.T) {
	tracker := &Tracker{enabled: true, db: nil}
	err := tracker.Close()
	if err != nil {
		t.Errorf("Close with nil database should not error: %v", err)
	}
}

func TestConfig_Defaults(t *testing.T) {
	config := Config{
		Enabled: false,
	}

	if config.DatabasePath != "" {
		t.Error("DatabasePath should be empty by default")
	}
	if config.AlertOnUnusualCountry {
		t.Error("AlertOnUnusualCountry should be false by default")
	}
	if config.CacheExpiryMinutes != 0 {
		t.Error("CacheExpiryMinutes should be 0 by default")
	}
}

func TestConfig_ExpectedCountries(t *testing.T) {
	config := Config{
		Enabled:           true,
		ExpectedCountries: []string{"DE", "US", "GB"},
	}

	if len(config.ExpectedCountries) != 3 {
		t.Errorf("Expected 3 countries, got %d", len(config.ExpectedCountries))
	}
}

// TestTracker_GetStats_Copy verifies that GetStats returns a copy, not the original
func TestTracker_GetStats_Copy(t *testing.T) {
	tracker := &Tracker{
		enabled: true,
		stats: Stats{
			LookupsTotal:        100,
			CountryDistribution: map[string]int64{"US": 50, "DE": 50},
		},
	}

	stats1 := tracker.GetStats()
	stats1.CountryDistribution["FR"] = 25 // Modify the copy

	stats2 := tracker.GetStats()
	if _, exists := stats2.CountryDistribution["FR"]; exists {
		t.Error("Modification to stats copy should not affect original")
	}
}

// TestTracker_IsEnabled tests the IsEnabled method
func TestTracker_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"Enabled", true},
		{"Disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := &Tracker{enabled: tt.enabled}
			if tracker.IsEnabled() != tt.enabled {
				t.Errorf("IsEnabled() = %v, want %v", tracker.IsEnabled(), tt.enabled)
			}
		})
	}
}

// TestTracker_Stats_Threading tests concurrent stats access
func TestTracker_Stats_Threading(t *testing.T) {
	tracker := &Tracker{
		enabled: true,
		stats: Stats{
			CountryDistribution: make(map[string]int64),
		},
	}

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			tracker.statsMutex.Lock()
			tracker.stats.LookupsTotal++
			tracker.stats.CountryDistribution["US"]++
			tracker.statsMutex.Unlock()
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = tracker.GetStats()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	stats := tracker.GetStats()
	if stats.LookupsTotal != 100 {
		t.Errorf("Expected 100 lookups, got %d", stats.LookupsTotal)
	}
	if stats.CountryDistribution["US"] != 100 {
		t.Errorf("Expected 100 US requests, got %d", stats.CountryDistribution["US"])
	}
}

// TestLocation_Fields verifies Location struct fields
func TestLocation_Fields(t *testing.T) {
	loc := &Location{
		IP:          "1.1.1.1",
		CountryCode: "US",
		CountryName: "United States",
		City:        "Los Angeles",
		Latitude:    34.0522,
		Longitude:   -118.2437,
		Timezone:    "America/Los_Angeles",
		Timestamp:   time.Now(),
	}

	if loc.IP != "1.1.1.1" {
		t.Errorf("IP mismatch: got %s", loc.IP)
	}
	if loc.CountryCode != "US" {
		t.Errorf("CountryCode mismatch: got %s", loc.CountryCode)
	}
	if loc.City != "Los Angeles" {
		t.Errorf("City mismatch: got %s", loc.City)
	}
}

// Helper to check if file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
