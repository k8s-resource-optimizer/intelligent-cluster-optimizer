package trends

import "time"

// GrowthPattern represents the type of growth pattern detected
type GrowthPattern string

const (
	PatternLinear      GrowthPattern = "linear"
	PatternExponential GrowthPattern = "exponential"
	PatternLogarithmic GrowthPattern = "logarithmic"
	PatternStable      GrowthPattern = "stable"
	PatternCyclical    GrowthPattern = "cyclical"
	PatternDecreasing  GrowthPattern = "decreasing"
	PatternVolatile    GrowthPattern = "volatile"
)

// GrowthRate represents growth rates at different time scales
type GrowthRate struct {
	Daily   float64 `json:"daily"`   // Percent per day
	Weekly  float64 `json:"weekly"`  // Percent per week
	Monthly float64 `json:"monthly"` // Percent per month
	CAGR    float64 `json:"cagr"`    // Compound Annual Growth Rate (%)
}

// RiskLevel represents the severity of capacity risk
type RiskLevel string

const (
	RiskNone     RiskLevel = "none"
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// CapacityAction represents recommended action based on risk
type CapacityAction string

const (
	ActionMonitor   CapacityAction = "monitor"
	ActionPlan      CapacityAction = "plan"
	ActionImmediate CapacityAction = "immediate"
)

// CapacityStatus represents capacity planning results
type CapacityStatus struct {
	CurrentUtilization float64        `json:"current_utilization"` // Percent (0-100)
	TimeToLimit        *time.Duration `json:"time_to_limit"`       // nil if not approaching limit
	RiskLevel          RiskLevel      `json:"risk_level"`
	RiskScore          float64        `json:"risk_score"` // 0-100
	RecommendedAction  CapacityAction `json:"recommended_action"`
	Message            string         `json:"message"`
}

// ForecastProjection represents a future forecast point
type ForecastProjection struct {
	Horizon    int       `json:"horizon_days"` // Days ahead
	Timestamp  time.Time `json:"timestamp"`
	Value      float64   `json:"value"`
	LowerBound float64   `json:"lower_bound"`
	UpperBound float64   `json:"upper_bound"`
}

// TrendAnalysis represents the complete analysis for a single resource
type TrendAnalysis struct {
	// Growth metrics
	GrowthPattern GrowthPattern `json:"growth_pattern"`
	GrowthRate    GrowthRate    `json:"growth_rate"`
	Acceleration  float64       `json:"acceleration"`   // Second derivative
	TrendStrength float64       `json:"trend_strength"` // 0-1, from decomposition

	// Capacity planning
	CapacityStatus CapacityStatus `json:"capacity_status"`

	// Forecasts
	ShortTerm ForecastProjection `json:"short_term"` // 7 days
	MidTerm   ForecastProjection `json:"mid_term"`   // 30 days
	LongTerm  ForecastProjection `json:"long_term"`  // 90 days

	// Quality metrics
	Confidence  float64 `json:"confidence"` // 0-100
	DataPoints  int     `json:"data_points"`
	DataQuality string  `json:"data_quality"` // "excellent", "good", "fair", "poor"
}

// WorkloadTrend represents trend analysis for a workload
type WorkloadTrend struct {
	Namespace      string        `json:"namespace"`
	Workload       string        `json:"workload"`
	CPUAnalysis    TrendAnalysis `json:"cpu_analysis"`
	MemoryAnalysis TrendAnalysis `json:"memory_analysis"`
	AnalysisTime   time.Time     `json:"analysis_time"`
	LookbackPeriod time.Duration `json:"lookback_period"`
}

// TrendReport represents a comprehensive trend report
type TrendReport struct {
	GeneratedAt    time.Time       `json:"generated_at"`
	Namespace      string          `json:"namespace"`
	LookbackPeriod time.Duration   `json:"lookback_period"`
	WorkloadTrends []WorkloadTrend `json:"workload_trends"`

	// Summary statistics
	TotalWorkloads      int            `json:"total_workloads"`
	CriticalRisks       int            `json:"critical_risks"`
	HighRisks           int            `json:"high_risks"`
	MediumRisks         int            `json:"medium_risks"`
	FastestGrowing      *WorkloadTrend `json:"fastest_growing,omitempty"`
	HighestCapacityRisk *WorkloadTrend `json:"highest_capacity_risk,omitempty"`
}

// AnalyzerConfig holds configuration for the trend analyzer
type AnalyzerConfig struct {
	// Data requirements
	MinDataPoints   int           // Default: 48 (2 days hourly)
	LookbackDefault time.Duration // Default: 30 days

	// Growth pattern thresholds
	StableThreshold     float64 // Default: 5% (growth < 5% = stable)
	HighGrowthThreshold float64 // Default: 20%

	// Capacity thresholds
	SoftLimitPercent float64 // Default: 80%
	HardLimitPercent float64 // Default: 95%
	CapacityBuffer   float64 // Default: 30% (recommend scaling at 70%)

	// Forecast horizons
	ShortTermDays int // Default: 7
	MidTermDays   int // Default: 30
	LongTermDays  int // Default: 90

	// Quality thresholds
	MinConfidence float64 // Default: 70%
}

// DefaultAnalyzerConfig returns default configuration
func DefaultAnalyzerConfig() *AnalyzerConfig {
	return &AnalyzerConfig{
		MinDataPoints:       48,
		LookbackDefault:     30 * 24 * time.Hour,
		StableThreshold:     5.0,
		HighGrowthThreshold: 20.0,
		SoftLimitPercent:    80.0,
		HardLimitPercent:    95.0,
		CapacityBuffer:      30.0,
		ShortTermDays:       7,
		MidTermDays:         30,
		LongTermDays:        90,
		MinConfidence:       70.0,
	}
}
