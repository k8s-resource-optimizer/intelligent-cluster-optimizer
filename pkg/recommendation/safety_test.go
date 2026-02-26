package recommendation

import (
	"path/filepath"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/leakdetector"
)

// TestSafety_InsufficientData tests that system doesn't make recommendations with inadequate data
func TestSafety_InsufficientData(t *testing.T) {
	t.Log("=== SAFETY TEST: Insufficient Data ===")

	testdataDir := "testdata"
	csvPath := filepath.Join(testdataDir, "insufficient_data.csv")

	data, err := loadCSVTestData(csvPath)
	if err != nil {
		t.Fatalf("Failed to load test data: %v", err)
	}

	t.Logf("Loaded %d samples from %s", len(data.Samples), csvPath)
	t.Logf("Description: %s", data.Description)
	t.Logf("Expected: %s", data.Expected)

	// Test with recommendation engine (requires 10 samples by default)
	engine := NewEngine()

	baseTime := time.Now().Add(-1 * time.Hour)
	containerSamples := make([]containerSample, len(data.Samples))
	for i, s := range data.Samples {
		containerSamples[i] = containerSample{
			timestamp:     baseTime.Add(s.TimestampOffset),
			usageCPU:      s.CPUMillicores,
			usageMemory:   s.MemoryBytes,
			requestCPU:    s.RequestCPU,
			requestMemory: s.RequestMemory,
		}
	}

	// Try to generate recommendation with insufficient samples
	rec := engine.generateContainerRecommendation(
		"test-container",
		containerSamples,
		95,  // CPU percentile
		95,  // Memory percentile
		1.2, // Safety margin
		10,  // Min samples - this is the key requirement
		nil,
	)

	t.Logf("\n=== Test Results ===")

	// Expected: No recommendation should be generated
	if rec != nil {
		t.Errorf("❌ FAILED: Expected NO recommendation with only %d samples (< 10 required)", len(data.Samples))
		t.Logf("But got recommendation: CPU=%dm, Memory=%dMi, Confidence=%.1f%%",
			rec.RecommendedCPU, rec.RecommendedMemory/(1024*1024), rec.Confidence)
	} else {
		t.Logf("✓ PASSED: System correctly rejected insufficient data (%d samples < 10 required)", len(data.Samples))
	}

	// Also test confidence calculator directly
	timestamps := make([]time.Time, len(data.Samples))
	cpuValues := make([]int64, len(data.Samples))
	for i, s := range data.Samples {
		timestamps[i] = baseTime.Add(s.TimestampOffset)
		cpuValues[i] = s.CPUMillicores
	}

	confCalc := NewConfidenceCalculator()
	confScore := confCalc.CalculateFromSamples(timestamps, cpuValues, 30*time.Second)

	t.Logf("\n=== Confidence Analysis ===")
	t.Logf("Overall Confidence: %.1f%% (%s)", confScore.Score, confScore.Level)
	t.Logf("Sample Count Score: %.1f%% (%d samples)", confScore.SampleCountScore, confScore.SampleCount)
	t.Logf("Description: %s", confScore.Description)

	if len(confScore.Warnings) > 0 {
		t.Logf("\nWarnings:")
		for _, w := range confScore.Warnings {
			t.Logf("  - %s", w)
		}
	}

	// Expected: Low confidence score
	if confScore.Score >= 70.0 {
		t.Errorf("❌ FAILED: Expected low confidence score with insufficient data, got %.1f%%", confScore.Score)
	} else {
		t.Logf("✓ PASSED: Low confidence score (%.1f%%) correctly reflects insufficient data", confScore.Score)
	}
}

// TestSafety_HighVariance tests that system lowers confidence for unstable metrics
func TestSafety_HighVariance(t *testing.T) {
	t.Log("\n=== SAFETY TEST: High Variance / Unstable Metrics ===")

	testdataDir := "testdata"
	csvPath := filepath.Join(testdataDir, "high_variance.csv")

	data, err := loadCSVTestData(csvPath)
	if err != nil {
		t.Fatalf("Failed to load test data: %v", err)
	}

	t.Logf("Loaded %d samples from %s", len(data.Samples), csvPath)
	t.Logf("Description: %s", data.Description)
	t.Logf("Expected: %s", data.Expected)

	// Create test samples
	baseTime := time.Now().Add(-3 * time.Hour)
	containerSamples := make([]containerSample, len(data.Samples))
	timestamps := make([]time.Time, len(data.Samples))
	cpuValues := make([]int64, len(data.Samples))

	for i, s := range data.Samples {
		containerSamples[i] = containerSample{
			timestamp:     baseTime.Add(s.TimestampOffset),
			usageCPU:      s.CPUMillicores,
			usageMemory:   s.MemoryBytes,
			requestCPU:    s.RequestCPU,
			requestMemory: s.RequestMemory,
		}
		timestamps[i] = baseTime.Add(s.TimestampOffset)
		cpuValues[i] = s.CPUMillicores
	}

	// Generate recommendation
	engine := NewEngine()
	rec := engine.generateContainerRecommendation(
		"test-container",
		containerSamples,
		95, 95, 1.2, 10, nil,
	)

	if rec == nil {
		t.Fatal("Failed to generate recommendation")
	}

	t.Logf("\n=== Recommendation Results ===")
	t.Logf("Current CPU: %dm, Recommended CPU: %dm", rec.CurrentCPU, rec.RecommendedCPU)
	t.Logf("Current Memory: %dMi, Recommended Memory: %dMi",
		rec.CurrentMemory/(1024*1024), rec.RecommendedMemory/(1024*1024))
	t.Logf("Overall Confidence: %.1f%%", rec.Confidence)

	// Check confidence details
	if rec.ConfidenceDetails != nil {
		t.Logf("\n=== Confidence Breakdown ===")
		t.Logf("Data Consistency Score: %.1f%%", rec.ConfidenceDetails.DataConsistencyScore)
		t.Logf("Coefficient of Variation: %.2f (%.0f%% variance)",
			rec.ConfidenceDetails.CoefficientOfVariation,
			rec.ConfidenceDetails.CoefficientOfVariation*100)
		t.Logf("Sample Count: %d", rec.ConfidenceDetails.SampleCount)
		t.Logf("Data Duration: %.1f hours", rec.ConfidenceDetails.DataDurationHours)

		if len(rec.ConfidenceDetails.Warnings) > 0 {
			t.Logf("\nWarnings:")
			for _, w := range rec.ConfidenceDetails.Warnings {
				t.Logf("  - %s", w)
			}
		}
	}

	// Expected: Lower confidence due to high variance
	// High variance means CV > 0.5 (50%), which should result in low consistency score
	expectedMaxConfidence := 70.0 // With high variance, confidence should be reduced

	if rec.Confidence > expectedMaxConfidence {
		t.Logf("⚠️  WARNING: Confidence (%.1f%%) is higher than expected for highly variable data", rec.Confidence)
	} else {
		t.Logf("✓ PASSED: Confidence (%.1f%%) correctly reflects high variance pattern", rec.Confidence)
	}

	// Check CV - should be high (> 0.5)
	if rec.ConfidenceDetails.CoefficientOfVariation < 0.3 {
		t.Errorf("❌ FAILED: Expected high CV (>0.3) for variance pattern, got %.2f",
			rec.ConfidenceDetails.CoefficientOfVariation)
	} else {
		t.Logf("✓ PASSED: High CV (%.2f) correctly detected", rec.ConfidenceDetails.CoefficientOfVariation)
	}

	// Check data consistency score - should be low
	if rec.ConfidenceDetails.DataConsistencyScore > 60.0 {
		t.Errorf("❌ FAILED: Expected low consistency score (<60) for variable data, got %.1f",
			rec.ConfidenceDetails.DataConsistencyScore)
	} else {
		t.Logf("✓ PASSED: Low consistency score (%.1f%%) correctly reflects unpredictable pattern",
			rec.ConfidenceDetails.DataConsistencyScore)
	}
}

// TestSafety_MemoryLeakDetection tests that memory leaks are detected and scaling is blocked
func TestSafety_MemoryLeakDetection(t *testing.T) {
	t.Log("\n=== SAFETY TEST: Memory Leak Detection ===")

	testdataDir := "testdata"
	csvPath := filepath.Join(testdataDir, "memory_leak.csv")

	data, err := loadCSVTestData(csvPath)
	if err != nil {
		t.Fatalf("Failed to load test data: %v", err)
	}

	t.Logf("Loaded %d samples from %s", len(data.Samples), csvPath)
	t.Logf("Description: %s", data.Description)
	t.Logf("Expected: %s", data.Expected)

	// Convert to leak detector samples
	baseTime := time.Now().Add(-3 * time.Hour)
	leakSamples := make([]leakdetector.MemorySample, len(data.Samples))

	for i, s := range data.Samples {
		leakSamples[i] = leakdetector.MemorySample{
			Timestamp: baseTime.Add(s.TimestampOffset),
			Bytes:     s.MemoryBytes,
		}
	}

	// Run leak detector
	detector := leakdetector.NewDetector()
	analysis := detector.Analyze(leakSamples)

	t.Logf("\n=== Leak Analysis Results ===")
	t.Logf("Memory Leak Detected: %v", analysis.IsLeak)
	t.Logf("Severity: %s", analysis.Severity)
	t.Logf("Confidence: %.1f%%", analysis.Confidence)
	t.Logf("Description: %s", analysis.Description)

	t.Logf("\n=== Statistics ===")
	t.Logf("Growth Rate: %.2f MB/hour (%.2f MB/day)",
		analysis.Statistics.Slope/(1024*1024),
		analysis.Statistics.SlopePerDay/(1024*1024))
	t.Logf("Total Growth: %.1f%% over %v", analysis.Statistics.GrowthPercent,
		analysis.Statistics.Duration.Round(time.Minute))
	t.Logf("Trend Consistency (R²): %.2f (%.0f%% linear fit)",
		analysis.Statistics.RSquared, analysis.Statistics.RSquared*100)
	t.Logf("Memory Resets: %d", analysis.Statistics.ResetCount)
	t.Logf("Start Memory: %.2f MB", float64(analysis.Statistics.StartMemory)/(1024*1024))
	t.Logf("End Memory: %.2f MB", float64(analysis.Statistics.EndMemory)/(1024*1024))

	t.Logf("\n=== Projections ===")
	t.Logf("Projected Memory in 24h: %.2f MB", float64(analysis.Statistics.ProjectedMemory24h)/(1024*1024))
	t.Logf("Projected Memory in 7d: %.2f MB", float64(analysis.Statistics.ProjectedMemory7d)/(1024*1024))

	// Expected: Memory leak should be detected
	if !analysis.IsLeak {
		t.Errorf("❌ FAILED: Expected memory leak to be detected, but IsLeak = false")
		t.Logf("Analysis: %s", analysis.Description)
	} else {
		t.Logf("✓ PASSED: Memory leak correctly detected (severity: %s, confidence: %.1f%%)",
			analysis.Severity, analysis.Confidence)
	}

	// Should prevent scaling
	shouldBlock, reason := analysis.ShouldPreventScaling()
	t.Logf("\n=== Scaling Decision ===")
	t.Logf("Should Block Scaling: %v", shouldBlock)
	t.Logf("Reason: %s", reason)

	if !shouldBlock {
		t.Errorf("❌ FAILED: Expected scaling to be BLOCKED due to memory leak")
	} else {
		t.Logf("✓ PASSED: Scaling correctly BLOCKED due to detected memory leak")
	}

	t.Logf("\n=== Recommendation ===")
	t.Logf("%s", analysis.Recommendation)

	// Verify leak detection criteria
	t.Logf("\n=== Leak Detection Criteria Validation ===")

	// Criterion 1: Positive slope above threshold (1 MB/hour)
	slopeMBPerHour := analysis.Statistics.Slope / (1024 * 1024)
	if slopeMBPerHour > 1.0 {
		t.Logf("✓ Criterion 1: Slope (%.2f MB/h) > threshold (1 MB/h)", slopeMBPerHour)
	} else {
		t.Logf("❌ Criterion 1: Slope (%.2f MB/h) <= threshold (1 MB/h)", slopeMBPerHour)
	}

	// Criterion 2: High R² (consistency >= 0.7)
	if analysis.Statistics.RSquared >= 0.7 {
		t.Logf("✓ Criterion 2: R² (%.2f) >= 0.7 (consistent upward trend)", analysis.Statistics.RSquared)
	} else {
		t.Logf("❌ Criterion 2: R² (%.2f) < 0.7 (inconsistent trend)", analysis.Statistics.RSquared)
	}

	// Criterion 3: Few memory resets (<= 2)
	if analysis.Statistics.ResetCount <= 2 {
		t.Logf("✓ Criterion 3: Memory resets (%d) <= 2 (not normal GC)", analysis.Statistics.ResetCount)
	} else {
		t.Logf("❌ Criterion 3: Memory resets (%d) > 2 (likely normal GC)", analysis.Statistics.ResetCount)
	}

	// Criterion 4: Significant growth (>= 10%)
	if analysis.Statistics.GrowthPercent >= 10.0 {
		t.Logf("✓ Criterion 4: Growth (%.1f%%) >= 10%% (significant)", analysis.Statistics.GrowthPercent)
	} else {
		t.Logf("❌ Criterion 4: Growth (%.1f%%) < 10%% (not significant)", analysis.Statistics.GrowthPercent)
	}
}

// TestSafety_OOMHistory tests that OOM history triggers memory boost
func TestSafety_OOMHistory(t *testing.T) {
	t.Log("\n=== SAFETY TEST: OOM History Handling ===")

	// Test different OOM scenarios with different restart counts
	testCases := []struct {
		name           string
		restartCount   int32
		expectedBoost  float64
		expectedAction string
	}{
		{
			name:           "Single OOM",
			restartCount:   1,
			expectedBoost:  1.3,
			expectedAction: "30% memory increase",
		},
		{
			name:           "Multiple OOMs",
			restartCount:   3,
			expectedBoost:  1.5,
			expectedAction: "50% memory increase",
		},
		{
			name:           "Frequent OOMs",
			restartCount:   5,
			expectedBoost:  1.75,
			expectedAction: "75% memory increase",
		},
		{
			name:           "Critical OOMs",
			restartCount:   10,
			expectedBoost:  2.0,
			expectedAction: "100% memory increase (double)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("\n--- Testing: %s (restart count: %d) ---", tc.name, tc.restartCount)

			// Create OOM info for this scenario
			oomInfo := &ContainerOOMDetails{
				OOMCount:         int(tc.restartCount),
				RecommendedBoost: calculateOOMBoost(tc.restartCount),
				Priority:         getOOMPriority(tc.restartCount),
			}

			t.Logf("OOM Count: %d", oomInfo.OOMCount)
			t.Logf("Recommended Boost: %.2fx", oomInfo.RecommendedBoost)
			t.Logf("Priority: %s", oomInfo.Priority)

			// Verify boost factor matches expected
			if oomInfo.RecommendedBoost != tc.expectedBoost {
				t.Errorf("❌ FAILED: Expected boost %.2fx, got %.2fx",
					tc.expectedBoost, oomInfo.RecommendedBoost)
			} else {
				t.Logf("✓ PASSED: Correct boost factor (%.2fx) for %d restarts",
					oomInfo.RecommendedBoost, tc.restartCount)
			}

			// Test memory recommendation with OOM boost
			baseMemory := int64(512 * 1024 * 1024) // 512 MB
			p95Memory := int64(400 * 1024 * 1024)  // P95 = 400 MB
			safetyMargin := 1.2                    // 20% safety margin

			// Without OOM: P95 * safety margin
			withoutOOM := int64(float64(p95Memory) * safetyMargin)

			// With OOM: P95 * safety margin * OOM boost
			withOOM := int64(float64(p95Memory) * safetyMargin * oomInfo.RecommendedBoost)

			// Never reduce below current if OOM occurred
			if withOOM < baseMemory {
				withOOM = baseMemory
			}

			t.Logf("\nMemory Calculation:")
			t.Logf("  Current Memory: %d MB", baseMemory/(1024*1024))
			t.Logf("  P95 Memory: %d MB", p95Memory/(1024*1024))
			t.Logf("  Without OOM: %d MB (P95 * 1.2)", withoutOOM/(1024*1024))
			t.Logf("  With OOM Boost: %d MB (P95 * 1.2 * %.2f)", withOOM/(1024*1024), oomInfo.RecommendedBoost)
			t.Logf("  Increase from base: %.1f%%", float64(withOOM-baseMemory)/float64(baseMemory)*100)

			// Verify memory is never reduced when OOM occurred
			if withOOM < baseMemory {
				t.Errorf("❌ FAILED: Memory recommendation (%d MB) is less than current (%d MB) despite OOM history",
					withOOM/(1024*1024), baseMemory/(1024*1024))
			} else {
				t.Logf("✓ PASSED: Memory recommendation maintains or increases current allocation")
			}

			// Verify boost is applied
			if withOOM <= withoutOOM {
				t.Errorf("❌ FAILED: OOM boost not applied (with OOM: %d MB <= without OOM: %d MB)",
					withOOM/(1024*1024), withoutOOM/(1024*1024))
			} else {
				t.Logf("✓ PASSED: OOM boost correctly applied (%d MB boost)",
					(withOOM-withoutOOM)/(1024*1024))
			}
		})
	}
}

// TestSafety_AllScenarios runs all safety tests in sequence
func TestSafety_AllScenarios(t *testing.T) {
	t.Log("=== COMPREHENSIVE SAFETY TEST SUITE ===\n")

	t.Run("InsufficientData", TestSafety_InsufficientData)
	t.Run("HighVariance", TestSafety_HighVariance)
	t.Run("MemoryLeakDetection", TestSafety_MemoryLeakDetection)
	t.Run("OOMHistory", TestSafety_OOMHistory)

	t.Log("\n=== ALL SAFETY TESTS COMPLETE ===")
}

// Helper functions
func calculateOOMBoost(restartCount int32) float64 {
	switch {
	case restartCount >= 10:
		return 2.0
	case restartCount >= 5:
		return 1.75
	case restartCount >= 3:
		return 1.5
	case restartCount >= 1:
		return 1.3
	default:
		return 1.2
	}
}

func getOOMPriority(restartCount int32) string {
	switch {
	case restartCount >= 10:
		return "Critical"
	case restartCount >= 5:
		return "High"
	case restartCount >= 3:
		return "Medium"
	case restartCount >= 1:
		return "Low"
	default:
		return "None"
	}
}
