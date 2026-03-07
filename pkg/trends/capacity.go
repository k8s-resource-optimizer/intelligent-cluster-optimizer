package trends

import (
	"fmt"
	"math"
	"time"

	"intelligent-cluster-optimizer/pkg/prediction"
)

// PredictTimeToExhaustion predicts when resource usage will reach the configured limit
func PredictTimeToExhaustion(data []float64, timestamps []time.Time, limit float64, config *AnalyzerConfig) *CapacityStatus {
	if len(data) == 0 || limit <= 0 {
		return &CapacityStatus{
			CurrentUtilization: 0,
			TimeToLimit:        nil,
			RiskLevel:          RiskNone,
			RiskScore:          0,
			RecommendedAction:  ActionMonitor,
			Message:            "Insufficient data or invalid limit",
		}
	}

	// Get current utilization
	currentValue := data[len(data)-1]
	currentUtilization := (currentValue / limit) * 100

	// If already over limit
	if currentUtilization >= 100 {
		return &CapacityStatus{
			CurrentUtilization: currentUtilization,
			TimeToLimit:        nil,
			RiskLevel:          RiskCritical,
			RiskScore:          100,
			RecommendedAction:  ActionImmediate,
			Message:            "Resource usage has exceeded configured limit",
		}
	}

	// If already at high capacity
	if currentUtilization >= config.HardLimitPercent {
		duration := time.Duration(0)
		return &CapacityStatus{
			CurrentUtilization: currentUtilization,
			TimeToLimit:        &duration,
			RiskLevel:          RiskCritical,
			RiskScore:          95,
			RecommendedAction:  ActionImmediate,
			Message:            fmt.Sprintf("Resource usage at %.1f%% of limit", currentUtilization),
		}
	}

	// Use Holt-Winters to forecast future usage
	horizonDays := config.LongTermDays // Forecast up to 90 days ahead
	horizonHours := horizonDays * 24
	forecast, timeToLimit := forecastToLimit(data, timestamps, limit, horizonHours)

	// Calculate acceleration
	acceleration := CalculateAcceleration(data)

	// Calculate risk score
	riskScore := CalculateRiskScore(currentUtilization, timeToLimit, acceleration, forecast)

	// Determine risk level
	riskLevel := determineRiskLevel(riskScore)

	// Recommend action
	action := RecommendCapacityAction(riskLevel, timeToLimit)

	// Generate message
	message := generateCapacityMessage(currentUtilization, timeToLimit, riskLevel)

	return &CapacityStatus{
		CurrentUtilization: currentUtilization,
		TimeToLimit:        timeToLimit,
		RiskLevel:          riskLevel,
		RiskScore:          riskScore,
		RecommendedAction:  action,
		Message:            message,
	}
}

// forecastToLimit uses Holt-Winters to forecast when limit will be reached
func forecastToLimit(data []float64, timestamps []time.Time, limit float64, horizonHours int) (*prediction.ForecastResult, *time.Duration) {
	// Need sufficient data for Holt-Winters
	if len(data) < 48 {
		return nil, nil
	}

	// Create and fit Holt-Winters model
	hw := prediction.NewHoltWinters()
	err := hw.FitWithTimestamps(data, timestamps)
	if err != nil {
		return nil, nil
	}

	// Generate forecast
	forecast, err := hw.Predict(horizonHours)
	if err != nil {
		return nil, nil
	}

	// Find when forecast crosses limit
	for _, f := range forecast.Forecasts {
		if f.Value >= limit {
			// Found the crossing point
			duration := f.Timestamp.Sub(timestamps[len(timestamps)-1])
			return forecast, &duration
		}
	}

	// Limit not reached within forecast horizon
	return forecast, nil
}

// CalculateRiskScore computes a 0-100 risk score based on multiple factors
// Weighting: 40% utilization + 30% time-to-limit + 20% acceleration + 10% forecast uncertainty
func CalculateRiskScore(utilization float64, timeToLimit *time.Duration, acceleration float64, forecast *prediction.ForecastResult) float64 {
	var score float64

	// Factor 1: Current utilization (40 points)
	// Linear scale: 0% = 0 points, 100% = 40 points
	utilizationScore := utilization * 0.4
	score += utilizationScore

	// Factor 2: Time to limit (30 points)
	// Inverse relationship: sooner = higher score
	if timeToLimit != nil {
		days := timeToLimit.Hours() / 24
		if days <= 0 {
			score += 30 // Immediate
		} else if days <= 7 {
			score += 30 * (1 - days/7) // 7 days = 0 points, 0 days = 30 points
		} else if days <= 30 {
			score += 15 * (1 - (days-7)/23) // 30 days = 0 points, 7 days = 15 points
		} else if days <= 90 {
			score += 5 * (1 - (days-30)/60) // 90 days = 0 points, 30 days = 5 points
		}
	}

	// Factor 3: Acceleration (20 points)
	// Positive acceleration (increasing growth rate) is risky
	if acceleration > 0 {
		// Normalize acceleration to 0-20 scale
		accelScore := math.Min(20, acceleration*10)
		score += accelScore
	}

	// Factor 4: Forecast uncertainty (10 points)
	// Higher uncertainty = higher risk
	if forecast != nil && forecast.Metrics != nil {
		// Use SMAPE as uncertainty measure (0-100 scale, lower is better)
		// Invert it: high error = high risk
		uncertaintyScore := math.Min(10, forecast.Metrics.SMAPE/10)
		score += uncertaintyScore
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score
}

// determineRiskLevel maps risk score to risk level
func determineRiskLevel(riskScore float64) RiskLevel {
	if riskScore >= 80 {
		return RiskCritical
	} else if riskScore >= 60 {
		return RiskHigh
	} else if riskScore >= 40 {
		return RiskMedium
	} else if riskScore >= 20 {
		return RiskLow
	}
	return RiskNone
}

// RecommendCapacityAction suggests action based on risk level and time
func RecommendCapacityAction(riskLevel RiskLevel, timeToLimit *time.Duration) CapacityAction {
	switch riskLevel {
	case RiskCritical:
		return ActionImmediate
	case RiskHigh:
		if timeToLimit != nil && timeToLimit.Hours() < 24*7 {
			return ActionImmediate
		}
		return ActionPlan
	case RiskMedium:
		return ActionPlan
	case RiskLow, RiskNone:
		return ActionMonitor
	default:
		return ActionMonitor
	}
}

// generateCapacityMessage creates a human-readable status message
func generateCapacityMessage(utilization float64, timeToLimit *time.Duration, riskLevel RiskLevel) string {
	baseMsg := fmt.Sprintf("Current utilization: %.1f%%", utilization)

	if timeToLimit == nil {
		return baseMsg + ". Resource usage is stable or not approaching limit."
	}

	days := timeToLimit.Hours() / 24
	var timeMsg string
	if days < 1 {
		hours := timeToLimit.Hours()
		timeMsg = fmt.Sprintf("%.1f hours", hours)
	} else if days < 7 {
		timeMsg = fmt.Sprintf("%.1f days", days)
	} else if days < 30 {
		weeks := days / 7
		timeMsg = fmt.Sprintf("%.1f weeks", weeks)
	} else {
		months := days / 30
		timeMsg = fmt.Sprintf("%.1f months", months)
	}

	return fmt.Sprintf("%s. Projected to reach limit in %s (%s risk).", baseMsg, timeMsg, riskLevel)
}
