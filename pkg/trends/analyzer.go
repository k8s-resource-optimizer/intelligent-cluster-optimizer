package trends

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/prediction"
	"intelligent-cluster-optimizer/pkg/storage"
)

// TrendAnalyzer orchestrates the trend analysis workflow
type TrendAnalyzer struct {
	storage *storage.InMemoryStorage
	config  *AnalyzerConfig
}

// NewTrendAnalyzer creates a new trend analyzer
func NewTrendAnalyzer(storage *storage.InMemoryStorage, config *AnalyzerConfig) *TrendAnalyzer {
	if config == nil {
		config = DefaultAnalyzerConfig()
	}
	return &TrendAnalyzer{
		storage: storage,
		config:  config,
	}
}

// AnalyzeWorkload performs trend analysis for a single workload
func (a *TrendAnalyzer) AnalyzeWorkload(namespace, workload string, lookback time.Duration) (*WorkloadTrend, error) {
	if lookback == 0 {
		lookback = a.config.LookbackDefault
	}

	// Fetch historical metrics
	metrics := a.storage.GetMetricsByWorkload(namespace, workload, lookback)
	if len(metrics) < a.config.MinDataPoints {
		return nil, fmt.Errorf("insufficient data: need at least %d points, got %d",
			a.config.MinDataPoints, len(metrics))
	}

	// Sort by timestamp
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].Timestamp.Before(metrics[j].Timestamp)
	})

	// Extract time series for CPU and memory
	cpuData, cpuTimestamps, cpuLimit := extractResourceTimeSeries(metrics, "cpu")
	memData, memTimestamps, memLimit := extractResourceTimeSeries(metrics, "memory")

	// Analyze CPU
	cpuAnalysis, err := a.analyzeResource(cpuData, cpuTimestamps, cpuLimit)
	if err != nil {
		return nil, fmt.Errorf("CPU analysis failed: %w", err)
	}

	// Analyze memory
	memAnalysis, err := a.analyzeResource(memData, memTimestamps, memLimit)
	if err != nil {
		return nil, fmt.Errorf("memory analysis failed: %w", err)
	}

	return &WorkloadTrend{
		Namespace:      namespace,
		Workload:       workload,
		CPUAnalysis:    *cpuAnalysis,
		MemoryAnalysis: *memAnalysis,
		AnalysisTime:   time.Now(),
		LookbackPeriod: lookback,
	}, nil
}

// AnalyzeNamespace performs trend analysis for all workloads in a namespace
func (a *TrendAnalyzer) AnalyzeNamespace(namespace string, lookback time.Duration) (*TrendReport, error) {
	if lookback == 0 {
		lookback = a.config.LookbackDefault
	}

	// Get all metrics for namespace
	metrics := a.storage.GetMetricsByNamespace(namespace, lookback)
	if len(metrics) == 0 {
		return nil, fmt.Errorf("no metrics found for namespace %s", namespace)
	}

	// Group by workload (extract workload name from pod name)
	workloadGroups := groupMetricsByWorkload(metrics)

	// Analyze each workload
	var trends []WorkloadTrend
	var criticalCount, highCount, mediumCount int

	for workloadName := range workloadGroups {
		trend, err := a.AnalyzeWorkload(namespace, workloadName, lookback)
		if err != nil {
			// Skip workloads with insufficient data
			continue
		}

		trends = append(trends, *trend)

		// Count risks
		if trend.CPUAnalysis.CapacityStatus.RiskLevel == RiskCritical ||
			trend.MemoryAnalysis.CapacityStatus.RiskLevel == RiskCritical {
			criticalCount++
		} else if trend.CPUAnalysis.CapacityStatus.RiskLevel == RiskHigh ||
			trend.MemoryAnalysis.CapacityStatus.RiskLevel == RiskHigh {
			highCount++
		} else if trend.CPUAnalysis.CapacityStatus.RiskLevel == RiskMedium ||
			trend.MemoryAnalysis.CapacityStatus.RiskLevel == RiskMedium {
			mediumCount++
		}
	}

	// Find fastest growing and highest risk workloads
	var fastestGrowing *WorkloadTrend
	var highestRisk *WorkloadTrend
	maxGrowth := 0.0
	maxRisk := 0.0

	for i := range trends {
		// Check CPU growth
		if trends[i].CPUAnalysis.GrowthRate.Monthly > maxGrowth {
			maxGrowth = trends[i].CPUAnalysis.GrowthRate.Monthly
			fastestGrowing = &trends[i]
		}
		// Check memory growth
		if trends[i].MemoryAnalysis.GrowthRate.Monthly > maxGrowth {
			maxGrowth = trends[i].MemoryAnalysis.GrowthRate.Monthly
			fastestGrowing = &trends[i]
		}

		// Check CPU risk
		if trends[i].CPUAnalysis.CapacityStatus.RiskScore > maxRisk {
			maxRisk = trends[i].CPUAnalysis.CapacityStatus.RiskScore
			highestRisk = &trends[i]
		}
		// Check memory risk
		if trends[i].MemoryAnalysis.CapacityStatus.RiskScore > maxRisk {
			maxRisk = trends[i].MemoryAnalysis.CapacityStatus.RiskScore
			highestRisk = &trends[i]
		}
	}

	return &TrendReport{
		GeneratedAt:         time.Now(),
		Namespace:           namespace,
		LookbackPeriod:      lookback,
		WorkloadTrends:      trends,
		TotalWorkloads:      len(trends),
		CriticalRisks:       criticalCount,
		HighRisks:           highCount,
		MediumRisks:         mediumCount,
		FastestGrowing:      fastestGrowing,
		HighestCapacityRisk: highestRisk,
	}, nil
}

// analyzeResource performs complete trend analysis for a single resource type
func (a *TrendAnalyzer) analyzeResource(data []float64, timestamps []time.Time, limit float64) (*TrendAnalysis, error) {
	if len(data) < a.config.MinDataPoints {
		return nil, fmt.Errorf("insufficient data points: %d < %d", len(data), a.config.MinDataPoints)
	}

	// Detect growth pattern
	pattern := DetectGrowthPattern(data, timestamps, a.config)

	// Calculate growth rates
	growthRates := CalculateGrowthRates(data, timestamps)

	// Calculate acceleration
	acceleration := CalculateAcceleration(data)

	// Get trend strength from decomposition
	trendStrength := 0.0
	period := prediction.DetectSeasonalPeriod(data, 24)
	decomposer := prediction.NewDecomposer(period)
	if result, err := decomposer.Decompose(data); err == nil {
		trendStrength, _ = result.GetStrength()
	}

	// Predict capacity exhaustion
	capacityStatus := PredictTimeToExhaustion(data, timestamps, limit, a.config)

	// Generate forecasts
	shortTerm, midTerm, longTerm := a.generateForecasts(data, timestamps)

	// Assess data quality and confidence
	dataQuality, confidence := assessDataQuality(data, len(data), trendStrength)

	return &TrendAnalysis{
		GrowthPattern:  pattern,
		GrowthRate:     growthRates,
		Acceleration:   acceleration,
		TrendStrength:  trendStrength,
		CapacityStatus: *capacityStatus,
		ShortTerm:      shortTerm,
		MidTerm:        midTerm,
		LongTerm:       longTerm,
		Confidence:     confidence,
		DataPoints:     len(data),
		DataQuality:    dataQuality,
	}, nil
}

// generateForecasts creates forecast projections for different time horizons
func (a *TrendAnalyzer) generateForecasts(data []float64, timestamps []time.Time) (ForecastProjection, ForecastProjection, ForecastProjection) {
	// Use Holt-Winters for forecasting
	hw := prediction.NewHoltWinters()
	if err := hw.FitWithTimestamps(data, timestamps); err != nil {
		// Return empty forecasts if fitting fails
		return ForecastProjection{}, ForecastProjection{}, ForecastProjection{}
	}

	// Calculate time interval
	var interval time.Duration
	if len(timestamps) >= 2 {
		interval = timestamps[1].Sub(timestamps[0])
	} else {
		interval = time.Hour
	}

	// Generate forecasts for each horizon
	lastTimestamp := timestamps[len(timestamps)-1]

	shortTerm := a.createForecast(hw, a.config.ShortTermDays, lastTimestamp, interval)
	midTerm := a.createForecast(hw, a.config.MidTermDays, lastTimestamp, interval)
	longTerm := a.createForecast(hw, a.config.LongTermDays, lastTimestamp, interval)

	return shortTerm, midTerm, longTerm
}

// createForecast generates a forecast projection for a specific horizon
func (a *TrendAnalyzer) createForecast(hw *prediction.HoltWinters, days int, lastTimestamp time.Time, interval time.Duration) ForecastProjection {
	hours := days * 24
	periodsAhead := int(time.Duration(hours) * time.Hour / interval)

	forecast, err := hw.Predict(periodsAhead)
	if err != nil || len(forecast.Forecasts) == 0 {
		return ForecastProjection{Horizon: days}
	}

	// Get the forecast at the target horizon
	targetForecast := forecast.Forecasts[len(forecast.Forecasts)-1]

	return ForecastProjection{
		Horizon:    days,
		Timestamp:  targetForecast.Timestamp,
		Value:      targetForecast.Value,
		LowerBound: targetForecast.LowerBound,
		UpperBound: targetForecast.UpperBound,
	}
}

// Helper functions

// extractResourceTimeSeries extracts time series data for a specific resource
func extractResourceTimeSeries(metrics []models.PodMetric, resourceType string) ([]float64, []time.Time, float64) {
	data := make([]float64, 0, len(metrics))
	timestamps := make([]time.Time, 0, len(metrics))
	var maxLimit float64

	for _, m := range metrics {
		var value float64
		var limit int64

		for _, c := range m.Containers {
			if resourceType == "cpu" {
				value += float64(c.UsageCPU)
				if c.LimitCPU > limit {
					limit = c.LimitCPU
				}
			} else { // memory
				value += float64(c.UsageMemory)
				if c.LimitMemory > limit {
					limit = c.LimitMemory
				}
			}
		}

		data = append(data, value)
		timestamps = append(timestamps, m.Timestamp)

		if float64(limit) > maxLimit {
			maxLimit = float64(limit)
		}
	}

	// If no limit configured, use a reasonable default based on usage
	if maxLimit == 0 {
		max := 0.0
		for _, v := range data {
			if v > max {
				max = v
			}
		}
		maxLimit = max * 2 // 2x peak usage as default limit
	}

	return data, timestamps, maxLimit
}

// groupMetricsByWorkload groups pod metrics by workload name
func groupMetricsByWorkload(metrics []models.PodMetric) map[string]bool {
	workloads := make(map[string]bool)

	for _, m := range metrics {
		// Extract workload name from pod name
		// Typically: workload-name-hash-pod or workload-name-pod
		workloadName := extractWorkloadName(m.PodName)
		workloads[workloadName] = true
	}

	return workloads
}

// extractWorkloadName extracts the workload name from a pod name.
// Kubernetes pod names follow "{workload}-{replicaset-hash}-{pod-hash}" for
// Deployments or "{workload}-{ordinal}" for StatefulSets. We strip the last
// segment always, and the second-to-last segment only when it looks like a
// hash (short ≤5 chars, or contains at least one digit).
func extractWorkloadName(podName string) string {
	parts := strings.Split(podName, "-")
	if len(parts) <= 2 {
		return podName
	}

	// Always strip the last segment (pod hash or ordinal).
	parts = parts[:len(parts)-1]

	// Strip second-to-last only if it looks like a generated suffix.
	if len(parts) >= 2 && isSuffixSegment(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}

	return strings.Join(parts, "-")
}

// isSuffixSegment returns true when a hyphen-separated segment looks like a
// Kubernetes-generated hash or ordinal rather than a meaningful name part.
// Heuristic: length ≤ 5 (short random suffix) OR contains at least one digit
// (e.g. "5d59f5d8c8", "67b8c9", "pod1").
func isSuffixSegment(s string) bool {
	if len(s) <= 5 {
		return true
	}
	for _, c := range s {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

// assessDataQuality evaluates data quality and confidence
func assessDataQuality(data []float64, dataPoints int, trendStrength float64) (string, float64) {
	// Base confidence on data points
	var baseConfidence float64
	if dataPoints >= 720 { // 30 days hourly
		baseConfidence = 90
	} else if dataPoints >= 336 { // 14 days hourly
		baseConfidence = 80
	} else if dataPoints >= 168 { // 7 days hourly
		baseConfidence = 70
	} else if dataPoints >= 48 { // 2 days hourly
		baseConfidence = 60
	} else {
		baseConfidence = 50
	}

	// Adjust based on trend strength
	confidence := baseConfidence * (0.7 + 0.3*trendStrength)

	// Determine quality label
	var quality string
	if confidence >= 85 {
		quality = "excellent"
	} else if confidence >= 70 {
		quality = "good"
	} else if confidence >= 54 {
		quality = "fair"
	} else {
		quality = "poor"
	}

	return quality, confidence
}
