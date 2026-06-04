package anomaly

import (
	"fmt"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
)

// WorkloadAnomalyResult contains anomaly detection results for a workload
type WorkloadAnomalyResult struct {
	Namespace    string
	WorkloadName string

	// CPU anomalies
	CPUAnomalies  *DetectionResult
	HasCPUAnomaly bool

	// Memory anomalies
	MemoryAnomalies  *DetectionResult
	HasMemoryAnomaly bool

	// Combined assessment
	HasAnyAnomaly      bool
	HighestSeverity    Severity
	ShouldBlockScaling bool
	BlockReason        string
	RecommendedAction  string
	AnomalyCount       int
	HighSeverityCount  int
}

// Summary returns a human-readable summary of the anomaly check
func (r *WorkloadAnomalyResult) Summary() string {
	if !r.HasAnyAnomaly {
		return fmt.Sprintf("No anomalies detected for %s/%s", r.Namespace, r.WorkloadName)
	}
	return fmt.Sprintf("Detected %d anomalies (%d high severity) for %s/%s - %s",
		r.AnomalyCount, r.HighSeverityCount, r.Namespace, r.WorkloadName, r.RecommendedAction)
}

// WorkloadChecker checks for anomalies in workload metrics
type WorkloadChecker struct {
	// Detector to use for anomaly detection
	Detector Detector

	// Config for anomaly detection
	Config *Config

	// BlockOnHighSeverity blocks scaling if high severity anomalies are detected
	BlockOnHighSeverity bool

	// MinAnomaliesForBlock is the minimum number of anomalies required to block
	MinAnomaliesForBlock int

	// RecentWindowSize is how many of the most-recent samples are checked when
	// deciding whether to block scaling. Only anomalies within this window can
	// trigger a block — historical anomalies from cyclic/bursty workloads are
	// ignored so that a known repeating pattern does not permanently prevent scaling.
	RecentWindowSize int
}

// NewWorkloadChecker creates a new workload anomaly checker with default settings
func NewWorkloadChecker() *WorkloadChecker {
	config := DefaultConfig()
	return &WorkloadChecker{
		Detector:             NewConsensusDetectorWithConfig(config),
		Config:               config,
		BlockOnHighSeverity:  true,
		MinAnomaliesForBlock: 1,
		RecentWindowSize:     10,
	}
}

// NewWorkloadCheckerWithConfig creates a workload checker with custom config
func NewWorkloadCheckerWithConfig(config *Config) *WorkloadChecker {
	return &WorkloadChecker{
		Detector:             NewConsensusDetectorWithConfig(config),
		Config:               config,
		BlockOnHighSeverity:  true,
		MinAnomaliesForBlock: 1,
		RecentWindowSize:     10,
	}
}

// CheckWorkload checks for anomalies in workload metrics
func (c *WorkloadChecker) CheckWorkload(namespace, workloadName string, metrics []models.PodMetric) *WorkloadAnomalyResult {
	result := &WorkloadAnomalyResult{
		Namespace:    namespace,
		WorkloadName: workloadName,
	}

	if len(metrics) < c.Config.MinSamples {
		return result
	}

	// Extract CPU and memory values
	cpuValues, memoryValues, timestamps := extractMetricValues(metrics)

	// Check CPU anomalies
	result.CPUAnomalies = c.Detector.DetectWithTimestamps(cpuValues, timestamps)
	result.HasCPUAnomaly = result.CPUAnomalies.HasAnomalies()

	// Check memory anomalies
	result.MemoryAnomalies = c.Detector.DetectWithTimestamps(memoryValues, timestamps)
	result.HasMemoryAnomaly = result.MemoryAnomalies.HasAnomalies()

	// Update anomaly types to reflect resource type
	for i := range result.CPUAnomalies.Anomalies {
		if result.CPUAnomalies.Anomalies[i].Type == AnomalyTypeCPUDrop {
			// Already correct
		} else {
			result.CPUAnomalies.Anomalies[i].Type = AnomalyTypeCPUSpike
		}
	}

	for i := range result.MemoryAnomalies.Anomalies {
		if result.MemoryAnomalies.Anomalies[i].Value < result.MemoryAnomalies.Mean {
			result.MemoryAnomalies.Anomalies[i].Type = AnomalyTypeMemoryDrop
		} else {
			result.MemoryAnomalies.Anomalies[i].Type = AnomalyTypeMemorySpike
		}
	}

	// Combine results
	result.HasAnyAnomaly = result.HasCPUAnomaly || result.HasMemoryAnomaly
	result.AnomalyCount = result.CPUAnomalies.AnomalyCount() + result.MemoryAnomalies.AnomalyCount()
	result.HighSeverityCount = result.CPUAnomalies.HighSeverityCount() + result.MemoryAnomalies.HighSeverityCount()

	// Determine highest severity
	result.HighestSeverity = c.getHighestSeverity(result.CPUAnomalies, result.MemoryAnomalies)

	// Determine if we should block scaling
	result.ShouldBlockScaling, result.BlockReason = c.shouldBlock(result)

	// Set recommended action
	result.RecommendedAction = c.getRecommendedAction(result)

	return result
}

// CheckMetrics checks raw metric slices for anomalies
func (c *WorkloadChecker) CheckMetrics(cpuValues, memoryValues []float64, timestamps []time.Time) *WorkloadAnomalyResult {
	result := &WorkloadAnomalyResult{}

	if len(cpuValues) < c.Config.MinSamples {
		return result
	}

	// Check CPU anomalies
	result.CPUAnomalies = c.Detector.DetectWithTimestamps(cpuValues, timestamps)
	result.HasCPUAnomaly = result.CPUAnomalies.HasAnomalies()

	// Check memory anomalies
	result.MemoryAnomalies = c.Detector.DetectWithTimestamps(memoryValues, timestamps)
	result.HasMemoryAnomaly = result.MemoryAnomalies.HasAnomalies()

	// Combine results
	result.HasAnyAnomaly = result.HasCPUAnomaly || result.HasMemoryAnomaly
	result.AnomalyCount = result.CPUAnomalies.AnomalyCount() + result.MemoryAnomalies.AnomalyCount()
	result.HighSeverityCount = result.CPUAnomalies.HighSeverityCount() + result.MemoryAnomalies.HighSeverityCount()

	// Determine highest severity
	result.HighestSeverity = c.getHighestSeverity(result.CPUAnomalies, result.MemoryAnomalies)

	// Determine if we should block scaling
	result.ShouldBlockScaling, result.BlockReason = c.shouldBlock(result)

	// Set recommended action
	result.RecommendedAction = c.getRecommendedAction(result)

	return result
}

// extractMetricValues extracts CPU and memory values from pod metrics
func extractMetricValues(metrics []models.PodMetric) (cpuValues, memoryValues []float64, timestamps []time.Time) {
	cpuValues = make([]float64, 0, len(metrics))
	memoryValues = make([]float64, 0, len(metrics))
	timestamps = make([]time.Time, 0, len(metrics))

	for _, pm := range metrics {
		// Aggregate container metrics for the pod
		var totalCPU, totalMemory int64
		for _, cm := range pm.Containers {
			totalCPU += cm.UsageCPU
			totalMemory += cm.UsageMemory
		}

		cpuValues = append(cpuValues, float64(totalCPU))
		memoryValues = append(memoryValues, float64(totalMemory))
		timestamps = append(timestamps, pm.Timestamp)
	}

	return cpuValues, memoryValues, timestamps
}

// getHighestSeverity returns the highest severity from two detection results
func (c *WorkloadChecker) getHighestSeverity(cpu, memory *DetectionResult) Severity {
	highest := SeverityLow

	for _, a := range cpu.Anomalies {
		if severityRank(a.Severity) > severityRank(highest) {
			highest = a.Severity
		}
	}

	for _, a := range memory.Anomalies {
		if severityRank(a.Severity) > severityRank(highest) {
			highest = a.Severity
		}
	}

	return highest
}

// shouldBlock determines if scaling should be blocked based on anomaly results.
// Only anomalies within the most-recent RecentWindowSize samples are considered
// so that long-running cyclic workloads with many historical anomalies are not
// permanently blocked.
func (c *WorkloadChecker) shouldBlock(result *WorkloadAnomalyResult) (bool, string) {
	if !result.HasAnyAnomaly {
		return false, ""
	}

	window := c.RecentWindowSize
	if window <= 0 {
		window = 10
	}

	recentHighCPU := result.CPUAnomalies.RecentHighSeverityCount(window)
	recentHighMem := result.MemoryAnomalies.RecentHighSeverityCount(window)
	recentHighTotal := recentHighCPU + recentHighMem

	// Block on high severity only if anomalies are recent
	if c.BlockOnHighSeverity && recentHighTotal > 0 {
		return true, fmt.Sprintf("%d recent high severity anomalies detected (window=%d samples)", recentHighTotal, window)
	}

	return false, ""
}

// getRecommendedAction returns a recommended action based on anomaly results
func (c *WorkloadChecker) getRecommendedAction(result *WorkloadAnomalyResult) string {
	if !result.HasAnyAnomaly {
		return "proceed with optimization"
	}

	switch result.HighestSeverity {
	case SeverityCritical:
		return "investigate immediately - do not scale"
	case SeverityHigh:
		return "review anomalies before proceeding"
	case SeverityMedium:
		return "proceed with caution, monitor closely"
	default:
		return "proceed with optimization"
	}
}

// IsRecentAnomaly checks if an anomaly was detected in the last N samples
func IsRecentAnomaly(result *DetectionResult, lastN int) bool {
	if !result.HasAnomalies() {
		return false
	}

	threshold := result.SampleCount - lastN
	for _, a := range result.Anomalies {
		if a.Index >= threshold {
			return true
		}
	}
	return false
}
