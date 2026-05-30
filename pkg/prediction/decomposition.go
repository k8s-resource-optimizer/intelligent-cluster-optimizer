package prediction

import (
	"math"
	"sort"
)

// DecompositionResult contains the decomposed time series components
type DecompositionResult struct {
	// Original data
	Original []float64

	// Trend component - the long-term progression
	Trend []float64

	// Seasonal component - repeating patterns
	Seasonal []float64

	// Residual component - what's left after removing trend and seasonal
	Residual []float64

	// Period used for decomposition
	Period int

	// Type of decomposition used
	Type DecompositionType
}

// DecompositionType defines how to decompose the series
type DecompositionType string

const (
	DecompositionAdditive       DecompositionType = "additive"
	DecompositionMultiplicative DecompositionType = "multiplicative"
)

// Decomposer decomposes time series into components
type Decomposer struct {
	// Period is the seasonal period (e.g., 24 for hourly data with daily seasonality)
	Period int

	// Type is additive or multiplicative decomposition
	Type DecompositionType

	// TrendWindow is the window size for trend smoothing (odd number, 0 = auto)
	TrendWindow int
}

// NewDecomposer creates a new decomposer with default settings
func NewDecomposer(period int) *Decomposer {
	return &Decomposer{
		Period:      period,
		Type:        DecompositionAdditive,
		TrendWindow: 0, // Auto
	}
}

// Decompose separates the time series into trend, seasonal, and residual components
// Uses classical decomposition (similar to STL but simpler)
func (d *Decomposer) Decompose(data []float64) (*DecompositionResult, error) {
	n := len(data)
	if n < 2*d.Period {
		// Fall back to simpler decomposition for short series
		return d.simpleDecompose(data)
	}

	result := &DecompositionResult{
		Original: make([]float64, n),
		Trend:    make([]float64, n),
		Seasonal: make([]float64, n),
		Residual: make([]float64, n),
		Period:   d.Period,
		Type:     d.Type,
	}

	copy(result.Original, data)

	// Step 1: Extract trend using moving average
	windowSize := d.TrendWindow
	if windowSize == 0 {
		// Auto: use period for centered moving average
		windowSize = d.Period
		if windowSize%2 == 0 {
			windowSize++ // Make it odd for symmetric window
		}
	}
	result.Trend = d.calculateTrend(data, windowSize)

	// Step 2: Detrend the data
	detrended := make([]float64, n)
	for i := 0; i < n; i++ {
		if d.Type == DecompositionAdditive {
			detrended[i] = data[i] - result.Trend[i]
		} else {
			if result.Trend[i] != 0 {
				detrended[i] = data[i] / result.Trend[i]
			} else {
				detrended[i] = 1.0
			}
		}
	}

	// Step 3: Calculate seasonal component by averaging each position
	seasonalPattern := d.calculateSeasonalPattern(detrended)

	// Replicate seasonal pattern across the series
	for i := 0; i < n; i++ {
		result.Seasonal[i] = seasonalPattern[i%d.Period]
	}

	// Step 4: Calculate residuals
	for i := 0; i < n; i++ {
		if d.Type == DecompositionAdditive {
			result.Residual[i] = data[i] - result.Trend[i] - result.Seasonal[i]
		} else {
			if result.Trend[i] != 0 && result.Seasonal[i] != 0 {
				result.Residual[i] = data[i] / (result.Trend[i] * result.Seasonal[i])
			} else {
				result.Residual[i] = 1.0
			}
		}
	}

	return result, nil
}

// simpleDecompose handles short series with basic decomposition
func (d *Decomposer) simpleDecompose(data []float64) (*DecompositionResult, error) {
	n := len(data)

	result := &DecompositionResult{
		Original: make([]float64, n),
		Trend:    make([]float64, n),
		Seasonal: make([]float64, n),
		Residual: make([]float64, n),
		Period:   d.Period,
		Type:     d.Type,
	}

	copy(result.Original, data)

	// Simple trend: linear regression
	slope, intercept := linearRegression(data)
	for i := 0; i < n; i++ {
		result.Trend[i] = intercept + slope*float64(i)
	}

	// Simple seasonal: deviation from trend at each position
	if n >= d.Period {
		seasonalPattern := d.calculateSeasonalPattern(data)
		for i := 0; i < n; i++ {
			result.Seasonal[i] = seasonalPattern[i%d.Period]
		}
	}

	// Residuals
	for i := 0; i < n; i++ {
		if d.Type == DecompositionAdditive {
			result.Residual[i] = data[i] - result.Trend[i] - result.Seasonal[i]
		} else {
			if result.Trend[i] != 0 && result.Seasonal[i] != 0 {
				result.Residual[i] = data[i] / (result.Trend[i] * result.Seasonal[i])
			} else {
				result.Residual[i] = 1.0
			}
		}
	}

	return result, nil
}

// calculateTrend uses centered moving average for trend extraction
func (d *Decomposer) calculateTrend(data []float64, windowSize int) []float64 {
	n := len(data)
	trend := make([]float64, n)

	halfWindow := windowSize / 2

	for i := 0; i < n; i++ {
		start := i - halfWindow
		end := i + halfWindow + 1

		if start < 0 {
			start = 0
		}
		if end > n {
			end = n
		}

		var sum float64
		count := 0
		for j := start; j < end; j++ {
			sum += data[j]
			count++
		}

		if count > 0 {
			trend[i] = sum / float64(count)
		}
	}

	return trend
}

// calculateSeasonalPattern averages values at each seasonal position
func (d *Decomposer) calculateSeasonalPattern(data []float64) []float64 {
	n := len(data)
	period := d.Period

	pattern := make([]float64, period)
	counts := make([]int, period)

	for i := 0; i < n; i++ {
		pos := i % period
		pattern[pos] += data[i]
		counts[pos]++
	}

	// Average
	for i := 0; i < period; i++ {
		if counts[i] > 0 {
			pattern[i] /= float64(counts[i])
		}
	}

	// Normalize
	if d.Type == DecompositionAdditive {
		// Center around 0
		var sum float64
		for _, v := range pattern {
			sum += v
		}
		avg := sum / float64(period)
		for i := range pattern {
			pattern[i] -= avg
		}
	} else {
		// Normalize to average 1
		var sum float64
		for _, v := range pattern {
			sum += v
		}
		avg := sum / float64(period)
		if avg != 0 {
			for i := range pattern {
				pattern[i] /= avg
			}
		}
	}

	return pattern
}

// linearRegression calculates simple linear regression (y = mx + b)
func linearRegression(data []float64) (slope, intercept float64) {
	n := float64(len(data))
	if n < 2 {
		return 0, data[0]
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

// GetStrength calculates the strength of trend and seasonal components
// Returns values between 0 and 1, where 1 means very strong
func (r *DecompositionResult) GetStrength() (trendStrength, seasonalStrength float64) {
	n := len(r.Original)
	if n == 0 {
		return 0, 0
	}

	// Variance of residuals
	varResidual := variance(r.Residual)

	// Variance of residual + seasonal
	resSeasonal := make([]float64, n)
	for i := 0; i < n; i++ {
		if r.Type == DecompositionAdditive {
			resSeasonal[i] = r.Residual[i] + r.Seasonal[i]
		} else {
			resSeasonal[i] = r.Residual[i] * r.Seasonal[i]
		}
	}
	varResSeasonal := variance(resSeasonal)

	// Variance of residual + trend
	resTrend := make([]float64, n)
	for i := 0; i < n; i++ {
		if r.Type == DecompositionAdditive {
			resTrend[i] = r.Residual[i] + r.Trend[i]
		} else {
			resTrend[i] = r.Residual[i] * r.Trend[i]
		}
	}
	varResTrend := variance(resTrend)

	// Strength calculations (standard Hyndman formula):
	// Trend strength   = 1 - Var(R) / Var(T+R)
	// Seasonal strength = 1 - Var(R) / Var(S+R)
	if varResTrend > 0 {
		trendStrength = math.Max(0, 1-varResidual/varResTrend)
	}
	if varResSeasonal > 0 {
		seasonalStrength = math.Max(0, 1-varResidual/varResSeasonal)
	}

	return trendStrength, seasonalStrength
}

// variance calculates the variance of a slice
func variance(data []float64) float64 {
	n := len(data)
	if n < 2 {
		return 0
	}

	var sum float64
	for _, v := range data {
		sum += v
	}
	mean := sum / float64(n)

	var sumSq float64
	for _, v := range data {
		diff := v - mean
		sumSq += diff * diff
	}

	return sumSq / float64(n-1)
}

// DetectSeasonalPeriod attempts to automatically detect the seasonal period
// using autocorrelation analysis
func DetectSeasonalPeriod(data []float64, maxPeriod int) int {
	n := len(data)
	if n < 4 {
		return 1
	}

	if maxPeriod == 0 || maxPeriod > n/2 {
		maxPeriod = n / 2
	}

	// Calculate autocorrelation for each lag
	acf := make([]float64, maxPeriod+1)
	mean := 0.0
	for _, v := range data {
		mean += v
	}
	mean /= float64(n)

	var variance float64
	for _, v := range data {
		diff := v - mean
		variance += diff * diff
	}

	if variance == 0 {
		return 1
	}

	for lag := 1; lag <= maxPeriod; lag++ {
		var sum float64
		for i := lag; i < n; i++ {
			sum += (data[i] - mean) * (data[i-lag] - mean)
		}
		acf[lag] = sum / variance
	}

	// Find the first significant peak
	bestPeriod := 1
	bestACF := 0.0

	for lag := 2; lag <= maxPeriod; lag++ {
		// Check if this is a local maximum
		if lag > 1 && lag < maxPeriod {
			if acf[lag] > acf[lag-1] && acf[lag] > acf[lag+1] && acf[lag] > bestACF && acf[lag] > 0.3 {
				bestPeriod = lag
				bestACF = acf[lag]
			}
		}
	}

	return bestPeriod
}

// Smooth applies smoothing to a time series
func Smooth(data []float64, windowSize int) []float64 {
	n := len(data)
	result := make([]float64, n)

	halfWindow := windowSize / 2

	for i := 0; i < n; i++ {
		start := i - halfWindow
		end := i + halfWindow + 1

		if start < 0 {
			start = 0
		}
		if end > n {
			end = n
		}

		var sum float64
		for j := start; j < end; j++ {
			sum += data[j]
		}
		result[i] = sum / float64(end-start)
	}

	return result
}

// MedianSmooth applies median smoothing (more robust to outliers)
func MedianSmooth(data []float64, windowSize int) []float64 {
	n := len(data)
	result := make([]float64, n)

	halfWindow := windowSize / 2

	for i := 0; i < n; i++ {
		start := i - halfWindow
		end := i + halfWindow + 1

		if start < 0 {
			start = 0
		}
		if end > n {
			end = n
		}

		window := make([]float64, end-start)
		copy(window, data[start:end])
		sort.Float64s(window)

		mid := len(window) / 2
		if len(window)%2 == 0 {
			result[i] = (window[mid-1] + window[mid]) / 2
		} else {
			result[i] = window[mid]
		}
	}

	return result
}
