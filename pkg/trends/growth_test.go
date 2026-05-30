package trends

import (
	"math"
	"testing"
	"time"
)

func TestDetectGrowthPattern(t *testing.T) {
	config := DefaultAnalyzerConfig()

	tests := []struct {
		name     string
		data     []float64
		expected GrowthPattern
	}{
		{
			name:     "stable pattern",
			data:     generateConstant(100, 100),
			expected: PatternStable,
		},
		{
			name:     "linear growth",
			data:     generateLinear(100, 100, 2),
			expected: PatternLinear,
		},
		{
			name:     "exponential growth",
			data:     generateExponential(50, 10, 1.05),
			expected: PatternExponential,
		},
		{
			name:     "decreasing pattern",
			data:     generateLinear(100, 100, -1),
			expected: PatternDecreasing,
		},
		{
			name:     "volatile pattern",
			data:     generateVolatile(100, 100, 55),
			expected: PatternVolatile,
		},
		{
			name:     "logarithmic growth",
			data:     generateLogarithmic(100, 10),
			expected: PatternLogarithmic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamps := generateTimestamps(len(tt.data), time.Hour)
			result := DetectGrowthPattern(tt.data, timestamps, config)

			if result != tt.expected {
				t.Errorf("DetectGrowthPattern() = %v, want %v", result, tt.expected)
				t.Logf("Data sample: %v", tt.data[:min(10, len(tt.data))])
			}
		})
	}
}

func TestCalculateGrowthRates(t *testing.T) {
	// Linear growth: 100, 102, 104, 106...
	data := generateLinear(100, 100, 2)
	timestamps := generateTimestamps(len(data), time.Hour)

	rates := CalculateGrowthRates(data, timestamps)

	// Expected daily growth rate: 2% per hour * 24 hours = 48% per day
	expectedDaily := 48.0
	tolerance := 5.0 // Allow 5% tolerance

	if math.Abs(rates.Daily-expectedDaily) > tolerance {
		t.Errorf("CalculateGrowthRates() daily = %.2f, want ~%.2f", rates.Daily, expectedDaily)
	}

	// Weekly should be 7x daily
	if math.Abs(rates.Weekly-rates.Daily*7) > 1.0 {
		t.Errorf("Weekly rate %.2f should be ~7x daily rate %.2f", rates.Weekly, rates.Daily)
	}

	// Monthly should be ~30x daily
	if math.Abs(rates.Monthly-rates.Daily*30) > 1.0 {
		t.Errorf("Monthly rate %.2f should be ~30x daily rate %.2f", rates.Monthly, rates.Daily)
	}
}

func TestCalculateCAGR(t *testing.T) {
	tests := []struct {
		name     string
		data     []float64
		periods  int
		expected float64
	}{
		{
			name:     "no growth",
			data:     []float64{100, 100},
			periods:  8760, // 1 year in hours
			expected: 0,
		},
		{
			name:     "positive growth",
			data:     []float64{100, 110},
			periods:  8760, // 1 year
			expected: 10,   // 10% growth
		},
		{
			name:     "doubling",
			data:     []float64{100, 200},
			periods:  8760, // 1 year
			expected: 100,  // 100% growth
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateCAGR(tt.data, tt.periods)

			if math.Abs(result-tt.expected) > 1.0 {
				t.Errorf("CalculateCAGR() = %.2f, want %.2f", result, tt.expected)
			}
		})
	}
}

func TestCalculateAcceleration(t *testing.T) {
	tests := []struct {
		name     string
		data     []float64
		expected string // "positive", "negative", "zero"
	}{
		{
			name:     "constant acceleration",
			data:     []float64{0, 1, 4, 9, 16, 25}, // x^2
			expected: "positive",
		},
		{
			name:     "no acceleration",
			data:     generateLinear(50, 100, 2),
			expected: "zero",
		},
		{
			name:     "negative acceleration",
			data:     generateLogarithmic(50, 10),
			expected: "negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accel := CalculateAcceleration(tt.data)

			switch tt.expected {
			case "positive":
				if accel <= 0 {
					t.Errorf("CalculateAcceleration() = %.4f, want positive", accel)
				}
			case "negative":
				if accel >= 0 {
					t.Errorf("CalculateAcceleration() = %.4f, want negative", accel)
				}
			case "zero":
				if math.Abs(accel) > 0.5 {
					t.Errorf("CalculateAcceleration() = %.4f, want ~0", accel)
				}
			}
		})
	}
}

func TestLinearRegression(t *testing.T) {
	// Test with perfect linear data: y = 2x + 5
	data := make([]float64, 10)
	for i := range data {
		data[i] = 2*float64(i) + 5
	}

	slope, intercept := linearRegression(data)

	if math.Abs(slope-2.0) > 0.01 {
		t.Errorf("linearRegression() slope = %.4f, want 2.0", slope)
	}

	if math.Abs(intercept-5.0) > 0.01 {
		t.Errorf("linearRegression() intercept = %.4f, want 5.0", intercept)
	}
}

func TestCoefficientOfVariation(t *testing.T) {
	tests := []struct {
		name     string
		data     []float64
		expected string // "low", "medium", "high"
	}{
		{
			name:     "constant data",
			data:     generateConstant(50, 100),
			expected: "low",
		},
		{
			name:     "moderate variation",
			data:     generateLinear(50, 100, 1),
			expected: "low",
		},
		{
			name:     "high variation",
			data:     generateVolatile(50, 100, 55),
			expected: "high",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cv := coefficientOfVariation(tt.data)

			switch tt.expected {
			case "low":
				if cv > 0.2 {
					t.Errorf("coefficientOfVariation() = %.4f, want < 0.2", cv)
				}
			case "medium":
				if cv < 0.2 || cv > 0.5 {
					t.Errorf("coefficientOfVariation() = %.4f, want 0.2-0.5", cv)
				}
			case "high":
				if cv < 0.5 {
					t.Errorf("coefficientOfVariation() = %.4f, want > 0.5", cv)
				}
			}
		})
	}
}

// Test data generators

func generateConstant(n int, value float64) []float64 {
	data := make([]float64, n)
	for i := range data {
		data[i] = value
	}
	return data
}

func generateLinear(n int, start, slope float64) []float64 {
	data := make([]float64, n)
	for i := range data {
		data[i] = start + slope*float64(i)
	}
	return data
}

func generateExponential(n int, start, rate float64) []float64 {
	data := make([]float64, n)
	for i := range data {
		data[i] = start * math.Pow(rate, float64(i))
	}
	return data
}

func generateLogarithmic(n int, scale float64) []float64 {
	data := make([]float64, n)
	for i := range data {
		data[i] = scale * math.Log(float64(i+1)+1)
	}
	return data
}

func generateVolatile(n int, baseline, amplitude float64) []float64 {
	data := make([]float64, n)
	for i := range data {
		// Random-like variation using sin waves
		noise := amplitude * (math.Sin(float64(i)*0.5) + math.Sin(float64(i)*0.13))
		data[i] = baseline + noise
	}
	return data
}

func generateTimestamps(n int, interval time.Duration) []time.Time {
	timestamps := make([]time.Time, n)
	start := time.Now().Add(-time.Duration(n) * interval)
	for i := range timestamps {
		timestamps[i] = start.Add(time.Duration(i) * interval)
	}
	return timestamps
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
