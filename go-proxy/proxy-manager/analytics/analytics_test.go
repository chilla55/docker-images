package analytics

import (
	"testing"
	"time"
)

func TestNewAggregator(t *testing.T) {
	agg := NewAggregator(1000, 10*time.Second)
	if agg == nil {
		t.Fatal("NewAggregator returned nil")
	}
	if agg.windowSize != 1000 {
		t.Errorf("expected windowSize 1000, got %d", agg.windowSize)
	}
}

func TestAddSample(t *testing.T) {
	agg := NewAggregator(100, 5*time.Second)

	// Add samples: responseTime (ms), bytesIn, bytesOut, errors, requests
	agg.AddSample(100.0, 1024, 2048, 0, 1)
	agg.AddSample(150.0, 2048, 4096, 0, 1)
	agg.AddSample(200.0, 512, 1024, 1, 1)

	metrics := agg.GetAggregatedMetrics()
	if metrics.SampleCount != 3 {
		t.Errorf("expected 3 samples, got %d", metrics.SampleCount)
	}
}

func TestAggregate(t *testing.T) {
	agg := NewAggregator(100, 5*time.Second)

	// Add various samples
	for i := 0; i < 10; i++ {
		agg.AddSample(float64(100+i*10), uint64(1000+i*100), uint64(2000+i*100), 0, 1)
	}

	metrics := agg.GetAggregatedMetrics()

	if metrics.SampleCount != 10 {
		t.Errorf("expected 10 samples, got %d", metrics.SampleCount)
	}

	if metrics.ResponseTimeMean == 0 {
		t.Error("expected response time mean to be calculated")
	}
}

func TestWindowSize(t *testing.T) {
	agg := NewAggregator(5, 1*time.Second)

	// Add more samples than window size
	for i := 0; i < 10; i++ {
		agg.AddSample(100.0, 1024, 2048, 0, 1)
	}

	metrics := agg.GetAggregatedMetrics()

	// Should only keep last 5 samples
	if metrics.SampleCount > 5 {
		t.Errorf("expected max 5 samples due to window, got %d", metrics.SampleCount)
	}
}

func TestResponseTimePercentiles(t *testing.T) {
	agg := NewAggregator(100, 5*time.Second)

	// Add samples with known distribution
	for i := 1; i <= 100; i++ {
		agg.AddSample(float64(i), 1024, 2048, 0, 1)
	}

	metrics := agg.GetAggregatedMetrics()

	// P50 should be around 50
	if metrics.ResponseTimeP50 < 45 || metrics.ResponseTimeP50 > 55 {
		t.Errorf("expected P50 around 50, got %.2f", metrics.ResponseTimeP50)
	}

	// P90 should be around 90
	if metrics.ResponseTimeP90 < 85 || metrics.ResponseTimeP90 > 95 {
		t.Errorf("expected P90 around 90, got %.2f", metrics.ResponseTimeP90)
	}
}
