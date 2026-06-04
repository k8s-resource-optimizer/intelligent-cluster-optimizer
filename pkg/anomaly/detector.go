package anomaly

import (
	"fmt"
	"time"
)

// AnomalyType represents the type of anomaly detected
type AnomalyType string

const (
	AnomalyTypeCPUSpike     AnomalyType = "cpu_spike"
	AnomalyTypeMemorySpike  AnomalyType = "memory_spike"
	AnomalyTypeCPUDrop      AnomalyType = "cpu_drop"
	AnomalyTypeMemoryDrop   AnomalyType = "memory_drop"
	AnomalyTypeUnusualTrend AnomalyType = "unusual_trend"
)

// Severity represents the severity level of an anomaly
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// DetectionMethod represents which method detected the anomaly
type DetectionMethod string

const (
	MethodZScore        DetectionMethod = "z_score"
	MethodIQR           DetectionMethod = "iqr"
	MethodMovingAverage DetectionMethod = "moving_average"
	MethodConsensus     DetectionMethod = "consensus"
)

// Anomaly represents a detected anomaly in the metrics
type Anomaly struct {
	// Timestamp when the anomaly occurred
	Timestamp time.Time

	// Type of anomaly
	Type AnomalyType

	// Severity level
	Severity Severity

	// Detection method that found this anomaly
	DetectedBy DetectionMethod

	// Value that triggered the anomaly
	Value float64

	// Expected range (lower, upper bounds)
	ExpectedLower float64
	ExpectedUpper float64

	// Deviation from expected (as a ratio or z-score)
	Deviation float64

	// Index in the original data series
	Index int

	// Additional context
	Message string
}

// DetectionResult contains the results of anomaly detection
type DetectionResult struct {
	// Anomalies found
	Anomalies []Anomaly

	// Statistics about the data
	Mean     float64
	StdDev   float64
	Median   float64
	Q1       float64
	Q3       float64
	IQR      float64
	MinValue float64
	MaxValue float64

	// Detection metadata
	Method      DetectionMethod
	Threshold   float64
	SampleCount int
	WindowSize  int
}

// HasAnomalies returns true if any anomalies were detected
func (r *DetectionResult) HasAnomalies() bool {
	return len(r.Anomalies) > 0
}

// AnomalyCount returns the number of anomalies detected
func (r *DetectionResult) AnomalyCount() int {
	return len(r.Anomalies)
}

// HighSeverityCount returns the count of high or critical severity anomalies
func (r *DetectionResult) HighSeverityCount() int {
	count := 0
	for _, a := range r.Anomalies {
		if a.Severity == SeverityHigh || a.Severity == SeverityCritical {
			count++
		}
	}
	return count
}

// RecentHighSeverityCount returns the count of high or critical severity anomalies
// in the last windowSize samples. Used to avoid blocking on historical cyclic patterns.
func (r *DetectionResult) RecentHighSeverityCount(windowSize int) int {
	threshold := r.SampleCount - windowSize
	count := 0
	for _, a := range r.Anomalies {
		if a.Index >= threshold && (a.Severity == SeverityHigh || a.Severity == SeverityCritical) {
			count++
		}
	}
	return count
}

// Summary returns a human-readable summary of the detection result
func (r *DetectionResult) Summary() string {
	if !r.HasAnomalies() {
		return fmt.Sprintf("No anomalies detected (method=%s, samples=%d)", r.Method, r.SampleCount)
	}
	return fmt.Sprintf("Detected %d anomalies (%d high severity) using %s (samples=%d)",
		r.AnomalyCount(), r.HighSeverityCount(), r.Method, r.SampleCount)
}

// Detector is the interface for anomaly detection methods
type Detector interface {
	// Detect analyzes the data and returns anomalies
	Detect(data []float64) *DetectionResult

	// DetectWithTimestamps analyzes data with associated timestamps
	DetectWithTimestamps(data []float64, timestamps []time.Time) *DetectionResult

	// Name returns the detector's method name
	Name() DetectionMethod
}

// Config contains configuration for anomaly detection
type Config struct {
	// Z-Score threshold (default: 3.0, meaning 3 standard deviations)
	ZScoreThreshold float64

	// IQR multiplier (default: 1.5, use 3.0 for extreme outliers only)
	IQRMultiplier float64

	// Moving average window size (default: 10)
	MovingAverageWindow int

	// Moving average deviation threshold (default: 2.0 standard deviations)
	MovingAverageThreshold float64

	// Minimum samples required for detection
	MinSamples int

	// Consensus threshold - minimum methods that must agree (default: 2)
	ConsensusThreshold int
}

// DefaultConfig returns the default anomaly detection configuration
func DefaultConfig() *Config {
	return &Config{
		ZScoreThreshold:        3.0,
		IQRMultiplier:          1.5,
		MovingAverageWindow:    10,
		MovingAverageThreshold: 2.0,
		MinSamples:             10,
		ConsensusThreshold:     2,
	}
}

// determineSeverity determines severity based on deviation magnitude
func determineSeverity(deviation float64) Severity {
	absDeviation := deviation
	if absDeviation < 0 {
		absDeviation = -absDeviation
	}

	switch {
	case absDeviation >= 5.0:
		return SeverityCritical
	case absDeviation >= 4.0:
		return SeverityHigh
	case absDeviation >= 3.0:
		return SeverityMedium
	default:
		return SeverityLow
	}
}
