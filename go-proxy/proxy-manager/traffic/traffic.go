package traffic

import (
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Analyzer performs traffic pattern analysis
type Analyzer struct {
	ipStats   map[string]*IPStats
	uaStats   map[string]*UserAgentStats
	pathStats map[string]*PathStats
	timeSlots map[int64]*TimeSlotStats // unix timestamp -> stats

	mu            sync.RWMutex
	windowSize    time.Duration // how far back to keep data
	cleanupTicker *time.Ticker
}

// IPStats tracks statistics for a single IP address
type IPStats struct {
	RequestCount    int
	ErrorCount      int
	BytesSent       uint64
	BytesReceived   uint64
	FirstSeen       int64
	LastSeen        int64
	Paths           map[string]int // path -> count
	StatusCodes     map[int]int    // status code -> count
	ReputationScore float64        // 0-100, lower is worse
}

// UserAgentStats tracks statistics for user agents
type UserAgentStats struct {
	RequestCount int
	ErrorCount   int
	FirstSeen    int64
	LastSeen     int64
	IPs          map[string]int // IP -> count
	Browser      string         // parsed browser name
	OS           string         // parsed OS name
	IsBot        bool
}

// PathStats tracks statistics for request paths
type PathStats struct {
	RequestCount      int
	ErrorCount        int
	TotalResponseTime float64
	AvgResponseTime   float64
	StatusCodes       map[int]int
	Methods           map[string]int
	UniqueIPs         map[string]bool
}

// TimeSlotStats tracks statistics per time slot
type TimeSlotStats struct {
	Timestamp    int64
	RequestCount int
	ErrorCount   int
	UniqueIPs    int
	TotalBytes   uint64
}

// TrafficAnalysis represents analyzed traffic patterns
type TrafficAnalysis struct {
	// Top Statistics
	TopIPs        []IPRanking        `json:"top_ips"`
	TopPaths      []PathRanking      `json:"top_paths"`
	TopUserAgents []UserAgentRanking `json:"top_user_agents"`

	// Reputation Analysis
	SuspiciousIPs []string `json:"suspicious_ips"`
	BotTraffic    float64  `json:"bot_traffic_percent"`

	// Geographic Distribution (placeholder for future implementation)
	TopCountries []string `json:"top_countries,omitempty"`

	// Pattern Detection
	AnomalousPatterns []AnomalyDetection `json:"anomalous_patterns"`
	TrafficPeaks      []TrafficPeak      `json:"traffic_peaks"`

	// Summary
	TotalUniqueIPs   int   `json:"total_unique_ips"`
	TotalUniquePaths int   `json:"total_unique_paths"`
	TotalUniqueUAs   int   `json:"total_unique_user_agents"`
	AnalysisWindow   int64 `json:"analysis_window_seconds"`
}

// IPRanking represents an IP in rankings
type IPRanking struct {
	IP               string  `json:"ip"`
	RequestCount     int     `json:"request_count"`
	ErrorRate        float64 `json:"error_rate_percent"`
	BytesTransferred uint64  `json:"bytes_transferred"`
	ReputationScore  float64 `json:"reputation_score"`
}

// PathRanking represents a path in rankings
type PathRanking struct {
	Path            string  `json:"path"`
	RequestCount    int     `json:"request_count"`
	ErrorRate       float64 `json:"error_rate_percent"`
	AvgResponseTime float64 `json:"avg_response_time_ms"`
	UniqueIPs       int     `json:"unique_ips"`
}

// UserAgentRanking represents a user agent in rankings
type UserAgentRanking struct {
	UserAgent    string  `json:"user_agent"`
	RequestCount int     `json:"request_count"`
	ErrorRate    float64 `json:"error_rate_percent"`
	IsBot        bool    `json:"is_bot"`
	Browser      string  `json:"browser,omitempty"`
	OS           string  `json:"os,omitempty"`
}

// AnomalyDetection represents detected anomalous patterns
type AnomalyDetection struct {
	Type        string `json:"type"` // "high_error_rate", "unusual_pattern", "rapid_requests"
	Description string `json:"description"`
	Severity    string `json:"severity"` // "low", "medium", "high"
	IP          string `json:"ip,omitempty"`
	Path        string `json:"path,omitempty"`
	Timestamp   int64  `json:"timestamp"`
}

// TrafficPeak represents a traffic spike
type TrafficPeak struct {
	Timestamp    int64 `json:"timestamp"`
	RequestCount int   `json:"request_count"`
	UniqueIPs    int   `json:"unique_ips"`
}

// NewAnalyzer creates a new traffic analyzer
func NewAnalyzer(windowSize time.Duration) *Analyzer {
	if windowSize <= 0 {
		windowSize = 1 * time.Hour // Default: 1 hour window
	}

	a := &Analyzer{
		ipStats:    make(map[string]*IPStats),
		uaStats:    make(map[string]*UserAgentStats),
		pathStats:  make(map[string]*PathStats),
		timeSlots:  make(map[int64]*TimeSlotStats),
		windowSize: windowSize,
	}

	// Start cleanup goroutine
	a.cleanupTicker = time.NewTicker(5 * time.Minute)
	go a.cleanupOldData()

	return a
}

// RecordRequest records a request for traffic analysis
func (a *Analyzer) RecordRequest(ip, path, method, userAgent string, statusCode int, responseTime float64, bytesIn, bytesOut uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now().Unix()

	// Update IP statistics
	if _, exists := a.ipStats[ip]; !exists {
		a.ipStats[ip] = &IPStats{
			FirstSeen:       now,
			Paths:           make(map[string]int),
			StatusCodes:     make(map[int]int),
			ReputationScore: 100.0, // Start with perfect score
		}
	}
	ipStat := a.ipStats[ip]
	ipStat.RequestCount++
	ipStat.LastSeen = now
	ipStat.BytesSent += bytesOut
	ipStat.BytesReceived += bytesIn
	ipStat.Paths[path]++
	ipStat.StatusCodes[statusCode]++
	if statusCode >= 400 {
		ipStat.ErrorCount++
		// Decrease reputation on errors
		ipStat.ReputationScore = max(0, ipStat.ReputationScore-0.5)
	}

	// Update user agent statistics
	if userAgent != "" {
		if _, exists := a.uaStats[userAgent]; !exists {
			a.uaStats[userAgent] = &UserAgentStats{
				FirstSeen: now,
				IPs:       make(map[string]int),
				IsBot:     a.detectBot(userAgent),
			}
			a.parseUserAgent(a.uaStats[userAgent], userAgent)
		}
		uaStat := a.uaStats[userAgent]
		uaStat.RequestCount++
		uaStat.LastSeen = now
		uaStat.IPs[ip]++
		if statusCode >= 400 {
			uaStat.ErrorCount++
		}
	}

	// Update path statistics
	if _, exists := a.pathStats[path]; !exists {
		a.pathStats[path] = &PathStats{
			StatusCodes: make(map[int]int),
			Methods:     make(map[string]int),
			UniqueIPs:   make(map[string]bool),
		}
	}
	pathStat := a.pathStats[path]
	pathStat.RequestCount++
	pathStat.TotalResponseTime += responseTime
	pathStat.AvgResponseTime = pathStat.TotalResponseTime / float64(pathStat.RequestCount)
	pathStat.StatusCodes[statusCode]++
	pathStat.Methods[method]++
	pathStat.UniqueIPs[ip] = true
	if statusCode >= 400 {
		pathStat.ErrorCount++
	}

	// Update time slot statistics (1-minute granularity)
	timeSlot := (now / 60) * 60
	if _, exists := a.timeSlots[timeSlot]; !exists {
		a.timeSlots[timeSlot] = &TimeSlotStats{
			Timestamp: timeSlot,
		}
	}
	slot := a.timeSlots[timeSlot]
	slot.RequestCount++
	slot.TotalBytes += bytesIn + bytesOut
	if statusCode >= 400 {
		slot.ErrorCount++
	}
}

// Analyze performs traffic analysis and returns insights
func (a *Analyzer) Analyze(topN int) TrafficAnalysis {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if topN <= 0 {
		topN = 10
	}

	analysis := TrafficAnalysis{
		TotalUniqueIPs:   len(a.ipStats),
		TotalUniquePaths: len(a.pathStats),
		TotalUniqueUAs:   len(a.uaStats),
		AnalysisWindow:   int64(a.windowSize.Seconds()),
	}

	// Rank IPs
	analysis.TopIPs = a.rankIPs(topN)

	// Rank paths
	analysis.TopPaths = a.rankPaths(topN)

	// Rank user agents
	analysis.TopUserAgents = a.rankUserAgents(topN)

	// Detect suspicious IPs (low reputation score)
	analysis.SuspiciousIPs = a.detectSuspiciousIPs()

	// Calculate bot traffic percentage
	analysis.BotTraffic = a.calculateBotTraffic()

	// Detect anomalies
	analysis.AnomalousPatterns = a.detectAnomalies()

	// Detect traffic peaks
	analysis.TrafficPeaks = a.detectTrafficPeaks()

	return analysis
}

// rankIPs ranks IPs by request count
func (a *Analyzer) rankIPs(topN int) []IPRanking {
	rankings := make([]IPRanking, 0, len(a.ipStats))

	for ip, stats := range a.ipStats {
		errorRate := 0.0
		if stats.RequestCount > 0 {
			errorRate = float64(stats.ErrorCount) / float64(stats.RequestCount) * 100
		}

		rankings = append(rankings, IPRanking{
			IP:               ip,
			RequestCount:     stats.RequestCount,
			ErrorRate:        errorRate,
			BytesTransferred: stats.BytesSent + stats.BytesReceived,
			ReputationScore:  stats.ReputationScore,
		})
	}

	// Sort by request count
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].RequestCount > rankings[j].RequestCount
	})

	if len(rankings) > topN {
		rankings = rankings[:topN]
	}

	return rankings
}

// rankPaths ranks paths by request count
func (a *Analyzer) rankPaths(topN int) []PathRanking {
	rankings := make([]PathRanking, 0, len(a.pathStats))

	for path, stats := range a.pathStats {
		errorRate := 0.0
		if stats.RequestCount > 0 {
			errorRate = float64(stats.ErrorCount) / float64(stats.RequestCount) * 100
		}

		rankings = append(rankings, PathRanking{
			Path:            path,
			RequestCount:    stats.RequestCount,
			ErrorRate:       errorRate,
			AvgResponseTime: stats.AvgResponseTime,
			UniqueIPs:       len(stats.UniqueIPs),
		})
	}

	// Sort by request count
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].RequestCount > rankings[j].RequestCount
	})

	if len(rankings) > topN {
		rankings = rankings[:topN]
	}

	return rankings
}

// rankUserAgents ranks user agents by request count
func (a *Analyzer) rankUserAgents(topN int) []UserAgentRanking {
	rankings := make([]UserAgentRanking, 0, len(a.uaStats))

	for ua, stats := range a.uaStats {
		errorRate := 0.0
		if stats.RequestCount > 0 {
			errorRate = float64(stats.ErrorCount) / float64(stats.RequestCount) * 100
		}

		rankings = append(rankings, UserAgentRanking{
			UserAgent:    ua,
			RequestCount: stats.RequestCount,
			ErrorRate:    errorRate,
			IsBot:        stats.IsBot,
			Browser:      stats.Browser,
			OS:           stats.OS,
		})
	}

	// Sort by request count
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].RequestCount > rankings[j].RequestCount
	})

	if len(rankings) > topN {
		rankings = rankings[:topN]
	}

	return rankings
}

// detectSuspiciousIPs returns IPs with low reputation scores
func (a *Analyzer) detectSuspiciousIPs() []string {
	suspicious := make([]string, 0)

	for ip, stats := range a.ipStats {
		// Flag IPs with reputation < 50 and significant activity
		if stats.ReputationScore < 50.0 && stats.RequestCount > 10 {
			suspicious = append(suspicious, ip)
		}
	}

	return suspicious
}

// calculateBotTraffic calculates percentage of bot traffic
func (a *Analyzer) calculateBotTraffic() float64 {
	totalRequests := 0
	botRequests := 0

	for _, stats := range a.uaStats {
		totalRequests += stats.RequestCount
		if stats.IsBot {
			botRequests += stats.RequestCount
		}
	}

	if totalRequests == 0 {
		return 0
	}

	return float64(botRequests) / float64(totalRequests) * 100
}

// detectAnomalies detects anomalous traffic patterns
func (a *Analyzer) detectAnomalies() []AnomalyDetection {
	anomalies := make([]AnomalyDetection, 0)
	now := time.Now().Unix()

	// Detect IPs with high error rates
	for ip, stats := range a.ipStats {
		if stats.RequestCount > 20 {
			errorRate := float64(stats.ErrorCount) / float64(stats.RequestCount) * 100
			if errorRate > 50 {
				anomalies = append(anomalies, AnomalyDetection{
					Type:        "high_error_rate",
					Description: "IP has unusually high error rate",
					Severity:    "medium",
					IP:          ip,
					Timestamp:   now,
				})
			}
		}

		// Detect rapid requests (potential attack)
		if stats.RequestCount > 100 && (now-stats.FirstSeen) < 60 {
			anomalies = append(anomalies, AnomalyDetection{
				Type:        "rapid_requests",
				Description: "IP made many requests in short time",
				Severity:    "high",
				IP:          ip,
				Timestamp:   now,
			})
		}
	}

	// Detect paths with high error rates
	for path, stats := range a.pathStats {
		if stats.RequestCount > 10 {
			errorRate := float64(stats.ErrorCount) / float64(stats.RequestCount) * 100
			if errorRate > 75 {
				anomalies = append(anomalies, AnomalyDetection{
					Type:        "high_error_rate",
					Description: "Path has unusually high error rate",
					Severity:    "high",
					Path:        path,
					Timestamp:   now,
				})
			}
		}
	}

	return anomalies
}

// detectTrafficPeaks detects traffic spikes in time slots
func (a *Analyzer) detectTrafficPeaks() []TrafficPeak {
	if len(a.timeSlots) == 0 {
		return nil
	}

	// Calculate average requests per slot
	totalRequests := 0
	for _, slot := range a.timeSlots {
		totalRequests += slot.RequestCount
	}
	avgRequests := float64(totalRequests) / float64(len(a.timeSlots))

	// Find slots with > 2x average requests
	peaks := make([]TrafficPeak, 0)
	threshold := avgRequests * 2.0

	for _, slot := range a.timeSlots {
		if float64(slot.RequestCount) > threshold {
			peaks = append(peaks, TrafficPeak{
				Timestamp:    slot.Timestamp,
				RequestCount: slot.RequestCount,
				UniqueIPs:    slot.UniqueIPs,
			})
		}
	}

	// Sort by timestamp
	sort.Slice(peaks, func(i, j int) bool {
		return peaks[i].Timestamp > peaks[j].Timestamp
	})

	return peaks
}

// detectBot checks if user agent string indicates a bot
func (a *Analyzer) detectBot(userAgent string) bool {
	ua := strings.ToLower(userAgent)
	botKeywords := []string{
		"bot", "crawler", "spider", "scraper", "curl", "wget",
		"python", "java", "ruby", "perl", "go-http-client",
	}

	for _, keyword := range botKeywords {
		if strings.Contains(ua, keyword) {
			return true
		}
	}

	return false
}

// parseUserAgent extracts browser and OS from user agent string
func (a *Analyzer) parseUserAgent(stats *UserAgentStats, userAgent string) {
	ua := strings.ToLower(userAgent)

	// Simple browser detection
	if strings.Contains(ua, "chrome") {
		stats.Browser = "Chrome"
	} else if strings.Contains(ua, "firefox") {
		stats.Browser = "Firefox"
	} else if strings.Contains(ua, "safari") {
		stats.Browser = "Safari"
	} else if strings.Contains(ua, "edge") {
		stats.Browser = "Edge"
	}

	// Simple OS detection
	if strings.Contains(ua, "windows") {
		stats.OS = "Windows"
	} else if strings.Contains(ua, "mac") || strings.Contains(ua, "darwin") {
		stats.OS = "macOS"
	} else if strings.Contains(ua, "linux") {
		stats.OS = "Linux"
	} else if strings.Contains(ua, "android") {
		stats.OS = "Android"
	} else if strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") {
		stats.OS = "iOS"
	}
}

// cleanupOldData removes data outside the time window
func (a *Analyzer) cleanupOldData() {
	for range a.cleanupTicker.C {
		a.mu.Lock()

		cutoff := time.Now().Add(-a.windowSize).Unix()

		// Clean up IP stats
		for ip, stats := range a.ipStats {
			if stats.LastSeen < cutoff {
				delete(a.ipStats, ip)
			}
		}

		// Clean up user agent stats
		for ua, stats := range a.uaStats {
			if stats.LastSeen < cutoff {
				delete(a.uaStats, ua)
			}
		}

		// Clean up time slots
		for ts := range a.timeSlots {
			if ts < cutoff {
				delete(a.timeSlots, ts)
			}
		}

		a.mu.Unlock()

		log.Debug().
			Int("ip_stats", len(a.ipStats)).
			Int("ua_stats", len(a.uaStats)).
			Int("path_stats", len(a.pathStats)).
			Int("time_slots", len(a.timeSlots)).
			Msg("Traffic analyzer cleanup complete")
	}
}

// Stop stops the cleanup ticker
func (a *Analyzer) Stop() {
	if a.cleanupTicker != nil {
		a.cleanupTicker.Stop()
	}
}

// GetIPReputation returns the reputation score for an IP
func (a *Analyzer) GetIPReputation(ip string) float64 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if stats, exists := a.ipStats[ip]; exists {
		return stats.ReputationScore
	}
	return 100.0 // Default: innocent until proven guilty
}

// IsPrivateIP checks if an IP is in a private range
func IsPrivateIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
	}

	for _, cidr := range privateRanges {
		_, ipNet, _ := net.ParseCIDR(cidr)
		if ipNet.Contains(parsedIP) {
			return true
		}
	}

	return false
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
