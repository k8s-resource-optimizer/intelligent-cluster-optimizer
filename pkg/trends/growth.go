package trends

import (
	"math"
	"time"

	"intelligent-cluster-optimizer/pkg/prediction"
)

// DetectGrowthPattern analyzes time series data to classify growth pattern
func DetectGrowthPattern(data []float64, timestamps []time.Time, config *AnalyzerConfig) GrowthPattern {
	if len(data) < 2 {
		return PatternStable
	}

	slope, intercept := linearRegression(data)
	avgValue := mean(data)
	if avgValue == 0 {
		return PatternStable
	}

	n := float64(len(data))
	growthRate := (slope / avgValue) * 100 * n

	// 1. Stable
	if math.Abs(growthRate) < config.StableThreshold {
		return PatternStable
	}

	// 2. Decreasing — only when the linear fit is strong (R²>0.7) so that volatile
	//    data with a negative net slope is not misclassified as decreasing.
	r2 := rSquared(data, slope, intercept)
	if growthRate < -config.StableThreshold && r2 > 0.7 {
		return PatternDecreasing
	}

	// 3. Exponential — checked before volatile because exponential series have
	//    a wide value range that inflates CV above the volatile threshold.
	if slope > 0 && testExponentialFit(data, slope) {
		return PatternExponential
	}

	// 4. Logarithmic — positive but decelerating growth.
	if testLogarithmicFit(data) {
		return PatternLogarithmic
	}

	// 5. Volatile — high coefficient of variation with no clear deterministic pattern.
	cv := coefficientOfVariation(data)
	if cv > 0.5 {
		return PatternVolatile
	}

	// 6. Cyclical — decomposition-based check for strong seasonality vs weak trend.
	period := prediction.DetectSeasonalPeriod(data, 24)
	decomposer := prediction.NewDecomposer(period)
	if result, err := decomposer.Decompose(data); err == nil {
		trendStrength, _ := result.GetStrength()
		if trendStrength < 0.3 {
			return PatternCyclical
		}
	}

	return PatternLinear
}

// CalculateGrowthRates computes growth rates at different time scales
func CalculateGrowthRates(data []float64, timestamps []time.Time) GrowthRate {
	if len(data) < 2 {
		return GrowthRate{}
	}

	// Linear regression to get trend
	slope, _ := linearRegression(data)

	// Use the starting value as baseline so that growth rate reflects percentage
	// change relative to where the series began (e.g. slope=2 on a series starting
	// at 100 gives 2%/point, matching the intuitive "2% per interval" definition).
	baseline := data[0]
	if baseline <= 0 {
		baseline = mean(data)
	}
	if baseline == 0 {
		return GrowthRate{}
	}

	// Calculate interval between data points
	var interval time.Duration
	if len(timestamps) >= 2 {
		interval = timestamps[1].Sub(timestamps[0])
	} else {
		interval = time.Hour // Default assume hourly
	}

	// Growth per data point as percentage
	growthPerPoint := (slope / baseline) * 100

	// Convert to different time scales
	daily := growthPerPoint * float64(24*time.Hour/interval)
	weekly := daily * 7
	monthly := daily * 30

	// Calculate CAGR (Compound Annual Growth Rate)
	cagr := CalculateCAGR(data, len(data))

	return GrowthRate{
		Daily:   daily,
		Weekly:  weekly,
		Monthly: monthly,
		CAGR:    cagr,
	}
}

// CalculateCAGR calculates Compound Annual Growth Rate
// CAGR = (ending_value / beginning_value) ^ (1 / years) - 1
func CalculateCAGR(data []float64, periods int) float64 {
	if len(data) < 2 || periods < 1 {
		return 0
	}

	beginning := data[0]
	ending := data[len(data)-1]

	if beginning <= 0 || ending <= 0 {
		return 0
	}

	// Assume each period is 1 hour, convert to years
	years := float64(periods) / (365.25 * 24)
	if years <= 0 {
		return 0
	}

	cagr := (math.Pow(ending/beginning, 1/years) - 1) * 100
	return cagr
}

// CalculateAcceleration calculates the second derivative (rate of change of growth rate)
func CalculateAcceleration(data []float64) float64 {
	if len(data) < 3 {
		return 0
	}

	// Calculate second derivative using central differences
	n := len(data)

	// Use the middle section for more stable estimation
	start := n / 4
	end := 3 * n / 4
	if end-start < 3 {
		start = 0
		end = n
	}

	var sumSecondDeriv float64
	count := 0

	for i := start + 1; i < end-1; i++ {
		// Second derivative: (y[i+1] - 2*y[i] + y[i-1])
		secondDeriv := data[i+1] - 2*data[i] + data[i-1]
		sumSecondDeriv += secondDeriv
		count++
	}

	if count == 0 {
		return 0
	}

	return sumSecondDeriv / float64(count)
}

// testExponentialFit tests if data fits exponential growth better than linear.
// Uses R² comparison: the log-transform must fit the data significantly better
// than a plain linear model to avoid misclassifying near-linear series.
func testExponentialFit(data []float64, linearSlope float64) bool {
	if len(data) < 3 || linearSlope <= 0 {
		return false
	}

	logData := make([]float64, 0, len(data))
	for _, v := range data {
		if v > 0 {
			logData = append(logData, math.Log(v))
		}
	}

	if len(logData) < len(data)*3/4 {
		return false
	}

	expSlope, expIntercept := linearRegression(logData)
	if expSlope <= 0.005 {
		return false
	}

	// The log-transform R² must be meaningfully better than the raw linear R²
	// to confirm true exponential growth rather than near-linear growth.
	r2Exp := rSquared(logData, expSlope, expIntercept)
	linSlope, linIntercept := linearRegression(data)
	r2Lin := rSquared(data, linSlope, linIntercept)

	return r2Exp > 0.95 && r2Exp-r2Lin > 0.03
}

// testLogarithmicFit tests if data fits logarithmic growth pattern
func testLogarithmicFit(data []float64) bool {
	if len(data) < 3 {
		return false
	}

	// Logarithmic growth: y = a + b*log(x)
	// Test by checking if rate of growth is decreasing

	// Split data into two halves
	mid := len(data) / 2
	firstHalf := data[:mid]
	secondHalf := data[mid:]

	// Calculate growth rates for each half
	slope1, _ := linearRegression(firstHalf)
	slope2, _ := linearRegression(secondHalf)

	// Logarithmic pattern: positive growth but decelerating
	// slope2 should be positive but less than slope1
	return slope1 > 0 && slope2 > 0 && slope2 < slope1*0.7
}

// Helper functions

func mean(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	var sum float64
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

func stdDev(data []float64) float64 {
	if len(data) < 2 {
		return 0
	}

	avg := mean(data)
	var sumSq float64
	for _, v := range data {
		diff := v - avg
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(len(data)-1))
}

func coefficientOfVariation(data []float64) float64 {
	avg := mean(data)
	if avg == 0 {
		return 0
	}
	sd := stdDev(data)
	return sd / math.Abs(avg)
}

func linearRegression(data []float64) (slope, intercept float64) {
	n := float64(len(data))
	if n < 2 {
		if n == 1 {
			return 0, data[0]
		}
		return 0, 0
	}

	var sumX, sumY, sumXY, sumX2 float64
	for i, y := range data {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0, sumY / n
	}

	slope = (n*sumXY - sumX*sumY) / denominator
	intercept = (sumY - slope*sumX) / n

	return slope, intercept
}

// rSquared computes the coefficient of determination (R²) for a linear fit
// y = intercept + slope*i over the data slice.
func rSquared(data []float64, slope, intercept float64) float64 {
	avg := mean(data)
	var ssTot, ssRes float64
	for i, y := range data {
		predicted := intercept + slope*float64(i)
		diff := y - avg
		ssTot += diff * diff
		resid := y - predicted
		ssRes += resid * resid
	}
	if ssTot == 0 {
		return 1.0
	}
	r2 := 1.0 - ssRes/ssTot
	if r2 < 0 {
		return 0
	}
	return r2
}
