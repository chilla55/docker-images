package analytics

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Aggregator performs advanced metrics aggregation and analysis
type Aggregator struct {
	responseTimes []float64 // milliseconds
	bandwidthIn   []uint64  // bytes
	bandwidthOut  []uint64  // bytes
	errorCounts   []int
	requestCounts []int
	timestamps    []int64 // unix timestamps

	mu           sync.RWMutex
	windowSize   int           // number of samples to keep
	samplePeriod time.Duration // how often to sample
	startTime    time.Time
}

// AggregatedMetrics represents advanced metrics calculations
type AggregatedMetrics struct {
	// Response Time Analysis
	ResponseTimeP50    float64 `json:"response_time_p50_ms"`
	ResponseTimeP90    float64 `json:"response_time_p90_ms"`
	ResponseTimeP95    float64 `json:"response_time_p95_ms"`
	ResponseTimeP99    float64 `json:"response_time_p99_ms"`
	ResponseTimeMean   float64 `json:"response_time_mean_ms"`
	ResponseTimeStdDev float64 `json:"response_time_stddev_ms"`

	// Bandwidth Analysis
	BandwidthInMean   float64 `json:"bandwidth_in_mean_bytes"`
	BandwidthOutMean  float64 `json:"bandwidth_out_mean_bytes"`
	BandwidthInTotal  uint64  `json:"bandwidth_in_total_bytes"`
	BandwidthOutTotal uint64  `json:"bandwidth_out_total_bytes"`
	BandwidthInPeak   uint64  `json:"bandwidth_in_peak_bytes"`
	BandwidthOutPeak  uint64  `json:"bandwidth_out_peak_bytes"`

	// Error Rate Analysis
	ErrorRateMean  float64 `json:"error_rate_mean_percent"`
	ErrorRatePeak  float64 `json:"error_rate_peak_percent"`
	ErrorRateTrend string  `json:"error_rate_trend"` // "increasing", "decreasing", "stable"

	// Traffic Analysis
	RequestsPerSecond float64 `json:"requests_per_second"`
	RequestsPeak      int     `json:"requests_peak"`
	TrafficTrend      string  `json:"traffic_trend"` // "increasing", "decreasing", "stable"

	// Time Window
	WindowStart int64 `json:"window_start"`
	WindowEnd   int64 `json:"window_end"`
	SampleCount int   `json:"sample_count"`
}

// NewAggregator creates a new metrics aggregator
func NewAggregator(windowSize int, samplePeriod time.Duration) *Aggregator {
	if windowSize <= 0 {
		windowSize = 1000 // Default: keep last 1000 samples
	}
	if samplePeriod <= 0 {
		samplePeriod = 10 * time.Second // Default: sample every 10 seconds
	}

	return &Aggregator{
		responseTimes: make([]float64, 0, windowSize),
		bandwidthIn:   make([]uint64, 0, windowSize),
		bandwidthOut:  make([]uint64, 0, windowSize),
		errorCounts:   make([]int, 0, windowSize),
		requestCounts: make([]int, 0, windowSize),
		timestamps:    make([]int64, 0, windowSize),
		windowSize:    windowSize,
		samplePeriod:  samplePeriod,
		startTime:     time.Now(),
	}
}

// AddSample adds a new metrics sample
func (a *Aggregator) AddSample(responseTime float64, bytesIn, bytesOut uint64, errors, requests int) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now().Unix()

	// Add new sample
	a.responseTimes = append(a.responseTimes, responseTime)
	a.bandwidthIn = append(a.bandwidthIn, bytesIn)
	a.bandwidthOut = append(a.bandwidthOut, bytesOut)
	a.errorCounts = append(a.errorCounts, errors)
	a.requestCounts = append(a.requestCounts, requests)
	a.timestamps = append(a.timestamps, now)

	// Keep only the last N samples (sliding window)
	if len(a.responseTimes) > a.windowSize {
		a.responseTimes = a.responseTimes[1:]
		a.bandwidthIn = a.bandwidthIn[1:]
		a.bandwidthOut = a.bandwidthOut[1:]
		a.errorCounts = a.errorCounts[1:]
		a.requestCounts = a.requestCounts[1:]
		a.timestamps = a.timestamps[1:]
	}
}

// GetAggregatedMetrics calculates and returns aggregated metrics
func (a *Aggregator) GetAggregatedMetrics() AggregatedMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()

	metrics := AggregatedMetrics{
		SampleCount: len(a.responseTimes),
	}

	if len(a.timestamps) > 0 {
		metrics.WindowStart = a.timestamps[0]
		metrics.WindowEnd = a.timestamps[len(a.timestamps)-1]
	}

	if metrics.SampleCount == 0 {
		return metrics
	}

	// Calculate response time percentiles
	metrics.ResponseTimeP50 = a.calculatePercentile(a.responseTimes, 0.50)
	metrics.ResponseTimeP90 = a.calculatePercentile(a.responseTimes, 0.90)
	metrics.ResponseTimeP95 = a.calculatePercentile(a.responseTimes, 0.95)
	metrics.ResponseTimeP99 = a.calculatePercentile(a.responseTimes, 0.99)
	metrics.ResponseTimeMean = a.calculateMean(a.responseTimes)
	metrics.ResponseTimeStdDev = a.calculateStdDev(a.responseTimes, metrics.ResponseTimeMean)

	// Calculate bandwidth metrics
	metrics.BandwidthInMean = a.calculateMeanUint64(a.bandwidthIn)
	metrics.BandwidthOutMean = a.calculateMeanUint64(a.bandwidthOut)
	metrics.BandwidthInTotal = a.calculateSumUint64(a.bandwidthIn)
	metrics.BandwidthOutTotal = a.calculateSumUint64(a.bandwidthOut)
	metrics.BandwidthInPeak = a.calculateMaxUint64(a.bandwidthIn)
	metrics.BandwidthOutPeak = a.calculateMaxUint64(a.bandwidthOut)

	// Calculate error rate metrics
	metrics.ErrorRateMean = a.calculateErrorRate()
	metrics.ErrorRatePeak = a.calculatePeakErrorRate()
	metrics.ErrorRateTrend = a.calculateTrend(a.errorRateHistory())

	// Calculate traffic metrics
	if len(a.timestamps) >= 2 {
		duration := float64(a.timestamps[len(a.timestamps)-1] - a.timestamps[0])
		if duration > 0 {
			totalRequests := 0
			for _, count := range a.requestCounts {
				totalRequests += count
			}
			metrics.RequestsPerSecond = float64(totalRequests) / duration
		}
	}
	metrics.RequestsPeak = a.calculateMaxInt(a.requestCounts)
	metrics.TrafficTrend = a.calculateTrend(a.requestHistory())

	return metrics
}

// calculatePercentile calculates the Nth percentile of a dataset
func (a *Aggregator) calculatePercentile(data []float64, percentile float64) float64 {
	if len(data) == 0 {
		return 0
	}

	// Create a copy and sort
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)

	index := percentile * float64(len(sorted)-1)
	lower := int(math.Floor(index))
	upper := int(math.Ceil(index))

	if lower == upper {
		return sorted[lower]
	}

	// Linear interpolation
	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// calculateMean calculates the mean of float64 values
func (a *Aggregator) calculateMean(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}

	sum := 0.0
	for _, val := range data {
		sum += val
	}
	return sum / float64(len(data))
}

// calculateStdDev calculates the standard deviation
func (a *Aggregator) calculateStdDev(data []float64, mean float64) float64 {
	if len(data) == 0 {
		return 0
	}

	variance := 0.0
	for _, val := range data {
		diff := val - mean
		variance += diff * diff
	}
	variance /= float64(len(data))
	return math.Sqrt(variance)
}

// calculateMeanUint64 calculates the mean of uint64 values
func (a *Aggregator) calculateMeanUint64(data []uint64) float64 {
	if len(data) == 0 {
		return 0
	}

	sum := uint64(0)
	for _, val := range data {
		sum += val
	}
	return float64(sum) / float64(len(data))
}

// calculateSumUint64 calculates the sum of uint64 values
func (a *Aggregator) calculateSumUint64(data []uint64) uint64 {
	sum := uint64(0)
	for _, val := range data {
		sum += val
	}
	return sum
}

// calculateMaxUint64 finds the maximum uint64 value
func (a *Aggregator) calculateMaxUint64(data []uint64) uint64 {
	if len(data) == 0 {
		return 0
	}

	max := data[0]
	for _, val := range data[1:] {
		if val > max {
			max = val
		}
	}
	return max
}

// calculateMaxInt finds the maximum int value
func (a *Aggregator) calculateMaxInt(data []int) int {
	if len(data) == 0 {
		return 0
	}

	max := data[0]
	for _, val := range data[1:] {
		if val > max {
			max = val
		}
	}
	return max
}

// errorRateHistory returns error rate for each sample
func (a *Aggregator) errorRateHistory() []float64 {
	history := make([]float64, len(a.errorCounts))
	for i := range a.errorCounts {
		if a.requestCounts[i] > 0 {
			history[i] = float64(a.errorCounts[i]) / float64(a.requestCounts[i]) * 100
		}
	}
	return history
}

// requestHistory returns normalized request counts
func (a *Aggregator) requestHistory() []float64 {
	history := make([]float64, len(a.requestCounts))
	for i := range a.requestCounts {
		history[i] = float64(a.requestCounts[i])
	}
	return history
}

// calculateErrorRate calculates mean error rate
func (a *Aggregator) calculateErrorRate() float64 {
	totalErrors := 0
	totalRequests := 0
	for i := range a.errorCounts {
		totalErrors += a.errorCounts[i]
		totalRequests += a.requestCounts[i]
	}

	if totalRequests == 0 {
		return 0
	}
	return float64(totalErrors) / float64(totalRequests) * 100
}

// calculatePeakErrorRate calculates peak error rate in any sample
func (a *Aggregator) calculatePeakErrorRate() float64 {
	peak := 0.0
	for i := range a.errorCounts {
		if a.requestCounts[i] > 0 {
			rate := float64(a.errorCounts[i]) / float64(a.requestCounts[i]) * 100
			if rate > peak {
				peak = rate
			}
		}
	}
	return peak
}

// calculateTrend determines if values are increasing, decreasing, or stable
func (a *Aggregator) calculateTrend(data []float64) string {
	if len(data) < 3 {
		return "stable"
	}

	// Simple linear regression slope calculation
	n := float64(len(data))
	sumX, sumY, sumXY, sumX2 := 0.0, 0.0, 0.0, 0.0

	for i, y := range data {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	// Calculate slope: (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)
	numerator := n*sumXY - sumX*sumY
	denominator := n*sumX2 - sumX*sumX

	if denominator == 0 {
		return "stable"
	}

	slope := numerator / denominator

	// Determine trend based on slope
	// Use relative slope compared to mean value
	mean := sumY / n
	if mean == 0 {
		return "stable"
	}

	relativeSlope := slope / mean

	if relativeSlope > 0.05 { // More than 5% increase per sample
		return "increasing"
	} else if relativeSlope < -0.05 { // More than 5% decrease per sample
		return "decreasing"
	}
	return "stable"
}

// Reset clears all metrics data
func (a *Aggregator) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.responseTimes = make([]float64, 0, a.windowSize)
	a.bandwidthIn = make([]uint64, 0, a.windowSize)
	a.bandwidthOut = make([]uint64, 0, a.windowSize)
	a.errorCounts = make([]int, 0, a.windowSize)
	a.requestCounts = make([]int, 0, a.windowSize)
	a.timestamps = make([]int64, 0, a.windowSize)
	a.startTime = time.Now()

	log.Info().Msg("Analytics aggregator reset")
}
