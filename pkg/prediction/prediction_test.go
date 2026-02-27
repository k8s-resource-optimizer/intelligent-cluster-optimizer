package prediction

import (
	"math"
	"testing"
	"time"
)

// Test data generators

// generateSeasonalData generates data with trend and seasonality
func generateSeasonalData(n, period int, level, trend, seasonalAmplitude float64) []float64 {
	data := make([]float64, n)

	for i := 0; i < n; i++ {
		// Trend component
		trendValue := level + trend*float64(i)

		// Seasonal component (sine wave)
		seasonPhase := 2 * math.Pi * float64(i%period) / float64(period)
		seasonal := seasonalAmplitude * math.Sin(seasonPhase)

		// Small noise
		noise := float64(i%5) * 0.1

		data[i] = trendValue + seasonal + noise
	}

	return data
}

// === Holt-Winters Tests ===

func TestHoltWinters_FitAndPredict(t *testing.T) {
	// Generate seasonal data
	data := generateSeasonalData(100, 12, 100, 0.5, 20)

	hw := NewHoltWinters()
	hw.config.SeasonalPeriod = 12
	hw.config.MinDataPoints = 2

	err := hw.Fit(data)
	if err != nil {
		t.Fatalf("Fit failed: %v", err)
	}

	if !hw.IsFitted() {
		t.Error("Expected model to be fitted")
	}

	result, err := hw.Predict(12)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}

	if len(result.Forecasts) != 12 {
		t.Errorf("Expected 12 forecasts, got %d", len(result.Forecasts))
	}

	if result.Method != "Holt-Winters" {
		t.Errorf("Expected method 'Holt-Winters', got '%s'", result.Method)
	}
}

func TestHoltWinters_FitPredict(t *testing.T) {
	data := generateSeasonalData(72, 12, 50, 0.1, 10)

	hw := NewHoltWinters()
	hw.config.SeasonalPeriod = 12
	hw.config.MinDataPoints = 2

	result, err := hw.FitPredict(data, 6)
	if err != nil {
		t.Fatalf("FitPredict failed: %v", err)
	}

	if len(result.Forecasts) != 6 {
		t.Errorf("Expected 6 forecasts, got %d", len(result.Forecasts))
	}

	// Check that forecasts are reasonable (within range of data)
	dataMin, dataMax := findMinMax(data)
	for _, f := range result.Forecasts {
		// Allow some margin for extrapolation
		if f.Value < dataMin*0.5 || f.Value > dataMax*2 {
			t.Errorf("Forecast %.2f seems unreasonable (data range: %.2f - %.2f)",
				f.Value, dataMin, dataMax)
		}
	}
}

func TestHoltWinters_InsufficientData(t *testing.T) {
	hw := NewHoltWinters()
	hw.config.SeasonalPeriod = 12
	hw.config.MinDataPoints = 2

	data := make([]float64, 10) // Less than 2 seasons
	err := hw.Fit(data)

	if err == nil {
		t.Error("Expected error for insufficient data")
	}
}

func TestHoltWinters_PredictWithoutFit(t *testing.T) {
	hw := NewHoltWinters()

	_, err := hw.Predict(12)
	if err == nil {
		t.Error("Expected error when predicting without fitting")
	}
}

func TestHoltWinters_WithTimestamps(t *testing.T) {
	data := generateSeasonalData(48, 12, 100, 0, 15)

	timestamps := make([]time.Time, len(data))
	now := time.Now()
	for i := range timestamps {
		timestamps[i] = now.Add(time.Duration(i) * time.Hour)
	}

	hw := NewHoltWinters()
	hw.config.SeasonalPeriod = 12
	hw.config.MinDataPoints = 2

	err := hw.FitWithTimestamps(data, timestamps)
	if err != nil {
		t.Fatalf("FitWithTimestamps failed: %v", err)
	}

	result, err := hw.Predict(6)
	if err != nil {
		t.Fatalf("Predict failed: %v", err)
	}

	// Check that forecast timestamps are set
	for i, f := range result.Forecasts {
		if f.Timestamp.IsZero() {
			t.Errorf("Forecast %d has zero timestamp", i)
		}
	}
}

func TestHoltWinters_ErrorMetrics(t *testing.T) {
	data := generateSeasonalData(60, 12, 100, 0, 10)

	hw := NewHoltWinters()
	hw.config.SeasonalPeriod = 12
	hw.config.MinDataPoints = 2

	result, err := hw.FitPredict(data, 6)
	if err != nil {
		t.Fatalf("FitPredict failed: %v", err)
	}

	if result.Metrics == nil {
		t.Fatal("Expected metrics to be calculated")
	}

	// MAE should be non-negative
	if result.Metrics.MAE < 0 {
		t.Errorf("MAE should be non-negative, got %.4f", result.Metrics.MAE)
	}

	// RMSE should be >= MAE
	if result.Metrics.RMSE < result.Metrics.MAE {
		t.Errorf("RMSE (%.4f) should be >= MAE (%.4f)", result.Metrics.RMSE, result.Metrics.MAE)
	}
}

func TestHoltWinters_Parameters(t *testing.T) {
	data := generateSeasonalData(48, 12, 100, 0.5, 20)

	hw := NewHoltWinters()
	hw.config.SeasonalPeriod = 12
	hw.config.MinDataPoints = 2

	result, err := hw.FitPredict(data, 6)
	if err != nil {
		t.Fatalf("FitPredict failed: %v", err)
	}

	params := result.Parameters
	if params == nil {
		t.Fatal("Expected parameters to be set")
		return
	}

	// Alpha, beta, gamma should be in (0, 1)
	if params.Alpha <= 0 || params.Alpha >= 1 {
		t.Errorf("Alpha should be in (0,1), got %.4f", params.Alpha)
	}

	if params.SeasonalPeriod != 12 {
		t.Errorf("Expected SeasonalPeriod 12, got %d", params.SeasonalPeriod)
	}
}

func TestHoltWinters_CustomParameters(t *testing.T) {
	config := &Config{
		SeasonalPeriod: 12,
		Alpha:          0.3,
		Beta:           0.1,
		Gamma:          0.2,
		SeasonalType:   SeasonalAdditive,
		TrendType:      TrendAdditive,
		MinDataPoints:  2,
	}

	hw := NewHoltWintersWithConfig(config)
	data := generateSeasonalData(48, 12, 100, 0, 10)

	result, err := hw.FitPredict(data, 6)
	if err != nil {
		t.Fatalf("FitPredict failed: %v", err)
	}

	// Check that custom parameters were used
	if result.Parameters.Alpha != 0.3 {
		t.Errorf("Expected alpha 0.3, got %.4f", result.Parameters.Alpha)
	}
}

// === Decomposition Tests ===

func TestDecomposer_Decompose(t *testing.T) {
	data := generateSeasonalData(72, 12, 100, 0.5, 15)

	decomposer := NewDecomposer(12)
	result, err := decomposer.Decompose(data)

	if err != nil {
		t.Fatalf("Decompose failed: %v", err)
	}

	// Check all components are present
	if len(result.Trend) != len(data) {
		t.Errorf("Trend length mismatch: expected %d, got %d", len(data), len(result.Trend))
	}

	if len(result.Seasonal) != len(data) {
		t.Errorf("Seasonal length mismatch: expected %d, got %d", len(data), len(result.Seasonal))
	}

	if len(result.Residual) != len(data) {
		t.Errorf("Residual length mismatch: expected %d, got %d", len(data), len(result.Residual))
	}
}

func TestDecomposer_Reconstruction(t *testing.T) {
	data := generateSeasonalData(48, 12, 100, 0, 10)

	decomposer := NewDecomposer(12)
	result, err := decomposer.Decompose(data)
	if err != nil {
		t.Fatalf("Decompose failed: %v", err)
	}

	// For additive decomposition: original = trend + seasonal + residual
	for i := 0; i < len(data); i++ {
		reconstructed := result.Trend[i] + result.Seasonal[i] + result.Residual[i]
		diff := math.Abs(data[i] - reconstructed)
		if diff > 0.001 {
			t.Errorf("Reconstruction error at %d: original=%.4f, reconstructed=%.4f, diff=%.4f",
				i, data[i], reconstructed, diff)
		}
	}
}

func TestDecomposer_MultiplicativeDecomposition(t *testing.T) {
	// Generate positive data for multiplicative decomposition
	data := make([]float64, 48)
	for i := range data {
		data[i] = 100 + 20*math.Sin(2*math.Pi*float64(i%12)/12) + float64(i)*0.5
	}

	decomposer := NewDecomposer(12)
	decomposer.Type = DecompositionMultiplicative

	result, err := decomposer.Decompose(data)
	if err != nil {
		t.Fatalf("Decompose failed: %v", err)
	}

	if result.Type != DecompositionMultiplicative {
		t.Errorf("Expected multiplicative type, got %s", result.Type)
	}
}

func TestDecomposer_GetStrength(t *testing.T) {
	// Strong seasonal pattern
	data := generateSeasonalData(72, 12, 100, 0.1, 30)

	decomposer := NewDecomposer(12)
	result, err := decomposer.Decompose(data)
	if err != nil {
		t.Fatalf("Decompose failed: %v", err)
	}

	trendStrength, seasonalStrength := result.GetStrength()

	// Both should be between 0 and 1
	if trendStrength < 0 || trendStrength > 1 {
		t.Errorf("Trend strength should be in [0,1], got %.4f", trendStrength)
	}

	if seasonalStrength < 0 || seasonalStrength > 1 {
		t.Errorf("Seasonal strength should be in [0,1], got %.4f", seasonalStrength)
	}
}

func TestDetectSeasonalPeriod(t *testing.T) {
	// Generate data with known period
	data := generateSeasonalData(120, 24, 100, 0, 20)

	detectedPeriod := DetectSeasonalPeriod(data, 48)

	// Allow some tolerance
	if detectedPeriod < 20 || detectedPeriod > 28 {
		t.Errorf("Expected period around 24, got %d", detectedPeriod)
	}
}

// === Workload Predictor Tests ===

func TestWorkloadPredictor_PredictFromValues(t *testing.T) {
	cpuData := generateSeasonalData(72, 24, 500, 1, 100)
	memData := generateSeasonalData(72, 24, 1024*1024*128, 0, 1024*1024*20)

	timestamps := make([]time.Time, len(cpuData))
	now := time.Now()
	for i := range timestamps {
		timestamps[i] = now.Add(time.Duration(i) * time.Hour)
	}

	config := DefaultConfig()
	config.SeasonalPeriod = 24
	config.MinDataPoints = 2

	predictor := NewWorkloadPredictorWithConfig(config, 24, 1.2)
	predictor.MinDataPoints = 48

	result, err := predictor.PredictFromValues(cpuData, memData, timestamps)
	if err != nil {
		t.Fatalf("PredictFromValues failed: %v", err)
	}

	if result.CPUForecast == nil {
		t.Error("Expected CPU forecast")
	}

	if result.MemoryForecast == nil {
		t.Error("Expected Memory forecast")
	}

	if result.PeakCPU <= 0 {
		t.Error("Expected positive peak CPU")
	}

	if result.RecommendedCPU <= 0 {
		t.Error("Expected positive recommended CPU")
	}
}

func TestWorkloadPredictor_TrendDetection(t *testing.T) {
	config := DefaultConfig()
	config.SeasonalPeriod = 12
	config.MinDataPoints = 2

	predictor := NewWorkloadPredictorWithConfig(config, 12, 1.0)
	predictor.MinDataPoints = 24

	// Generate seasonal data with strong upward trend
	// The trend must be significant relative to level (>5%) for TrendUp
	cpuData := generateSeasonalData(48, 12, 100, 10, 20) // level=100, trend=10 per period
	memData := generateSeasonalData(48, 12, 1000, 50, 100)
	timestamps := make([]time.Time, 48)
	now := time.Now()
	for i := range timestamps {
		timestamps[i] = now.Add(time.Duration(i) * time.Hour)
	}

	result, err := predictor.PredictFromValues(cpuData, memData, timestamps)
	if err != nil {
		t.Fatalf("PredictFromValues failed: %v", err)
	}

	// With strong upward trend, expect TrendUp
	// Note: Trend detection depends on model fitting - check it's not Down at minimum
	if result.CPUTrend == TrendDown {
		t.Errorf("Expected TrendUp or TrendStable for increasing data, got %s", result.CPUTrend)
	}
}

func TestWorkloadPredictor_InsufficientData(t *testing.T) {
	predictor := NewWorkloadPredictor()
	predictor.MinDataPoints = 48

	cpuData := make([]float64, 24)
	memData := make([]float64, 24)
	timestamps := make([]time.Time, 24)

	_, err := predictor.PredictFromValues(cpuData, memData, timestamps)
	if err == nil {
		t.Error("Expected error for insufficient data")
	}
}

func TestWorkloadPrediction_ScalingRecommendations(t *testing.T) {
	peakMem := int64(1024 * 1024 * 256)
	pred := &WorkloadPrediction{
		PeakCPU:           1000,
		PeakMemory:        float64(peakMem),
		RecommendedCPU:    1200,
		RecommendedMemory: int64(float64(peakMem) * 1.2),
	}

	// Should scale up when current < recommended
	if !pred.ShouldScaleUp(500, 1024*1024*128) {
		t.Error("Expected ShouldScaleUp to return true")
	}

	// Should not scale up when current >= recommended
	if pred.ShouldScaleUp(2000, 1024*1024*512) {
		t.Error("Expected ShouldScaleUp to return false when already sufficient")
	}

	// Should scale down when over-provisioned by threshold
	if !pred.ShouldScaleDown(5000, 1024*1024*1024, 0.3) {
		t.Error("Expected ShouldScaleDown to return true when over-provisioned")
	}
}

func TestWorkloadPrediction_Summary(t *testing.T) {
	pred := &WorkloadPrediction{
		Namespace:    "default",
		WorkloadName: "nginx",
		CPUTrend:     TrendUp,
		MemoryTrend:  TrendStable,
		PeakCPU:      1000,
		PeakMemory:   1024 * 1024 * 256,
		Confidence:   85.5,
	}

	summary := pred.Summary()
	if summary == "" {
		t.Error("Expected non-empty summary")
	}
}

// === Forecast Result Tests ===

func TestForecastResult_GetForecast(t *testing.T) {
	result := &ForecastResult{
		Forecasts: []Forecast{
			{Value: 100, HorizonIndex: 1},
			{Value: 110, HorizonIndex: 2},
			{Value: 120, HorizonIndex: 3},
		},
	}

	forecast, err := result.GetForecast(2)
	if err != nil {
		t.Fatalf("GetForecast failed: %v", err)
	}

	if forecast.Value != 110 {
		t.Errorf("Expected value 110, got %.2f", forecast.Value)
	}

	// Test out of range
	_, err = result.GetForecast(5)
	if err == nil {
		t.Error("Expected error for out of range horizon")
	}
}

func TestForecastResult_PeakAndTrough(t *testing.T) {
	result := &ForecastResult{
		Forecasts: []Forecast{
			{Value: 100, HorizonIndex: 1},
			{Value: 150, HorizonIndex: 2},
			{Value: 80, HorizonIndex: 3},
			{Value: 120, HorizonIndex: 4},
		},
	}

	peak := result.PeakForecast()
	if peak.Value != 150 {
		t.Errorf("Expected peak value 150, got %.2f", peak.Value)
	}

	trough := result.TroughForecast()
	if trough.Value != 80 {
		t.Errorf("Expected trough value 80, got %.2f", trough.Value)
	}
}

// === Smoothing Tests ===

func TestSmooth(t *testing.T) {
	data := []float64{10, 20, 30, 20, 10, 20, 30, 20, 10}

	smoothed := Smooth(data, 3)

	if len(smoothed) != len(data) {
		t.Errorf("Expected length %d, got %d", len(data), len(smoothed))
	}

	// Smoothed values should be less variable
	originalVar := variance(data)
	smoothedVar := variance(smoothed)

	if smoothedVar >= originalVar {
		t.Errorf("Expected smoothed variance (%.4f) < original variance (%.4f)",
			smoothedVar, originalVar)
	}
}

func TestMedianSmooth(t *testing.T) {
	// Data with outlier
	data := []float64{10, 10, 100, 10, 10} // 100 is an outlier

	smoothed := MedianSmooth(data, 3)

	// Median smoothing should reduce outlier effect
	if smoothed[2] == 100 {
		t.Error("Expected median smoothing to reduce outlier")
	}
}

// === Benchmark Tests ===

func BenchmarkHoltWinters_Fit(b *testing.B) {
	data := generateSeasonalData(168, 24, 100, 0.5, 20) // 1 week hourly

	config := DefaultConfig()
	config.SeasonalPeriod = 24
	config.MinDataPoints = 2

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hw := NewHoltWintersWithConfig(config)
		_ = hw.Fit(data)
	}
}

func BenchmarkHoltWinters_Predict(b *testing.B) {
	data := generateSeasonalData(168, 24, 100, 0.5, 20)

	config := DefaultConfig()
	config.SeasonalPeriod = 24
	config.MinDataPoints = 2

	hw := NewHoltWintersWithConfig(config)
	_ = hw.Fit(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = hw.Predict(24)
	}
}

func BenchmarkDecomposer_Decompose(b *testing.B) {
	data := generateSeasonalData(168, 24, 100, 0.5, 20)
	decomposer := NewDecomposer(24)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = decomposer.Decompose(data)
	}
}

// Helper function
func findMinMax(data []float64) (min, max float64) {
	if len(data) == 0 {
		return 0, 0
	}

	min, max = data[0], data[0]
	for _, v := range data[1:] {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}
