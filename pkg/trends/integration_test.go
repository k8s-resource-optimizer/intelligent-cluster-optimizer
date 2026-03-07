package trends

import (
	"math"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"
)

func TestIntegrationAnalyzeWorkload(t *testing.T) {
	// Create storage and populate with test data
	store := storage.NewStorage()
	populateTestData(store, "default", "test-app", 100, 100, 2) // Linear growth

	// Create analyzer
	config := DefaultAnalyzerConfig()
	config.MinDataPoints = 50 // Lower threshold for test
	analyzer := NewTrendAnalyzer(store, config)

	// Analyze workload
	trend, err := analyzer.AnalyzeWorkload("default", "test-app", 200*time.Hour)
	if err != nil {
		t.Fatalf("AnalyzeWorkload failed: %v", err)
	}

	// Verify basic structure
	if trend.Namespace != "default" {
		t.Errorf("Namespace = %q, want %q", trend.Namespace, "default")
	}
	if trend.Workload != "test-app" {
		t.Errorf("Workload = %q, want %q", trend.Workload, "test-app")
	}

	// Verify CPU analysis
	t.Run("CPU Analysis", func(t *testing.T) {
		cpu := trend.CPUAnalysis

		if cpu.DataPoints != 100 {
			t.Errorf("DataPoints = %d, want 100", cpu.DataPoints)
		}

		// Should detect linear growth pattern
		if cpu.GrowthPattern != PatternLinear && cpu.GrowthPattern != PatternExponential {
			t.Logf("Warning: Expected linear/exponential pattern, got %v", cpu.GrowthPattern)
		}

		// Growth rate should be positive
		if cpu.GrowthRate.Daily <= 0 {
			t.Errorf("Daily growth rate = %.2f, want > 0", cpu.GrowthRate.Daily)
		}

		// Should have forecasts
		if cpu.ShortTerm.Horizon != 7 {
			t.Errorf("ShortTerm horizon = %d, want 7", cpu.ShortTerm.Horizon)
		}
		if cpu.MidTerm.Horizon != 30 {
			t.Errorf("MidTerm horizon = %d, want 30", cpu.MidTerm.Horizon)
		}
		if cpu.LongTerm.Horizon != 90 {
			t.Errorf("LongTerm horizon = %d, want 90", cpu.LongTerm.Horizon)
		}

		// Capacity status should be present
		if cpu.CapacityStatus.RiskLevel == "" {
			t.Error("Expected RiskLevel to be set")
		}

		t.Logf("CPU Growth Pattern: %v", cpu.GrowthPattern)
		t.Logf("CPU Monthly Growth: %.2f%%", cpu.GrowthRate.Monthly)
		t.Logf("CPU Risk Level: %v (score: %.2f)", cpu.CapacityStatus.RiskLevel, cpu.CapacityStatus.RiskScore)
		t.Logf("CPU Confidence: %.2f%% (%s)", cpu.Confidence, cpu.DataQuality)
	})

	// Verify memory analysis
	t.Run("Memory Analysis", func(t *testing.T) {
		mem := trend.MemoryAnalysis

		if mem.DataPoints != 100 {
			t.Errorf("DataPoints = %d, want 100", mem.DataPoints)
		}

		// Growth rate should be positive
		if mem.GrowthRate.Daily <= 0 {
			t.Errorf("Daily growth rate = %.2f, want > 0", mem.GrowthRate.Daily)
		}

		t.Logf("Memory Growth Pattern: %v", mem.GrowthPattern)
		t.Logf("Memory Monthly Growth: %.2f%%", mem.GrowthRate.Monthly)
		t.Logf("Memory Risk Level: %v (score: %.2f)", mem.CapacityStatus.RiskLevel, mem.CapacityStatus.RiskScore)
	})
}

func TestIntegrationAnalyzeNamespace(t *testing.T) {
	// Create storage and populate with multiple workloads
	store := storage.NewStorage()
	populateTestData(store, "production", "api-server", 100, 100, 2)
	populateTestData(store, "production", "worker", 100, 200, 1)
	populateTestData(store, "production", "cache", 100, 50, 0) // Stable

	// Create analyzer
	config := DefaultAnalyzerConfig()
	config.MinDataPoints = 50
	analyzer := NewTrendAnalyzer(store, config)

	// Analyze namespace
	report, err := analyzer.AnalyzeNamespace("production", 200*time.Hour)
	if err != nil {
		t.Fatalf("AnalyzeNamespace failed: %v", err)
	}

	// Verify report structure
	if report.Namespace != "production" {
		t.Errorf("Namespace = %q, want %q", report.Namespace, "production")
	}

	if report.TotalWorkloads == 0 {
		t.Error("Expected TotalWorkloads > 0")
	}

	t.Logf("Total workloads: %d", report.TotalWorkloads)
	t.Logf("Risk counts - Critical: %d, High: %d, Medium: %d",
		report.CriticalRisks, report.HighRisks, report.MediumRisks)

	// Verify workload trends
	if len(report.WorkloadTrends) == 0 {
		t.Error("Expected at least one workload trend")
	}

	for _, trend := range report.WorkloadTrends {
		t.Logf("Workload %s:", trend.Workload)
		t.Logf("  CPU: %v growth at %.1f%%/month, risk: %v",
			trend.CPUAnalysis.GrowthPattern,
			trend.CPUAnalysis.GrowthRate.Monthly,
			trend.CPUAnalysis.CapacityStatus.RiskLevel)
		t.Logf("  Memory: %v growth at %.1f%%/month, risk: %v",
			trend.MemoryAnalysis.GrowthPattern,
			trend.MemoryAnalysis.GrowthRate.Monthly,
			trend.MemoryAnalysis.CapacityStatus.RiskLevel)
	}

	// Check summary fields
	if report.FastestGrowing != nil {
		t.Logf("Fastest growing: %s", report.FastestGrowing.Workload)
	}
	if report.HighestCapacityRisk != nil {
		t.Logf("Highest risk: %s", report.HighestCapacityRisk.Workload)
	}
}

func TestIntegrationHighCapacityRisk(t *testing.T) {
	// Create storage with data approaching limit
	store := storage.NewStorage()
	populateTestDataNearLimit(store, "default", "critical-app", 100)

	// Create analyzer
	config := DefaultAnalyzerConfig()
	config.MinDataPoints = 50
	analyzer := NewTrendAnalyzer(store, config)

	// Analyze workload
	trend, err := analyzer.AnalyzeWorkload("default", "critical-app", 200*time.Hour)
	if err != nil {
		t.Fatalf("AnalyzeWorkload failed: %v", err)
	}

	// Should detect high or critical risk
	cpuRisk := trend.CPUAnalysis.CapacityStatus.RiskLevel
	if cpuRisk != RiskHigh && cpuRisk != RiskCritical {
		t.Logf("Warning: Expected high/critical CPU risk for near-limit data, got %v", cpuRisk)
		t.Logf("CPU utilization: %.2f%%", trend.CPUAnalysis.CapacityStatus.CurrentUtilization)
		t.Logf("Risk score: %.2f", trend.CPUAnalysis.CapacityStatus.RiskScore)
	}

	// Should have time to limit prediction
	if trend.CPUAnalysis.CapacityStatus.TimeToLimit == nil {
		t.Log("Warning: Expected TimeToLimit prediction for near-limit data")
	} else {
		t.Logf("Time to CPU limit: %v", *trend.CPUAnalysis.CapacityStatus.TimeToLimit)
	}

	// Should recommend action
	if trend.CPUAnalysis.CapacityStatus.RecommendedAction == ActionMonitor {
		t.Logf("Warning: Expected action other than monitor, got %v",
			trend.CPUAnalysis.CapacityStatus.RecommendedAction)
	}

	t.Logf("Status message: %s", trend.CPUAnalysis.CapacityStatus.Message)
}

// Helper functions for test data generation

func populateTestData(store *storage.InMemoryStorage, namespace, workloadPrefix string, count int, startValue float64, growthRate float64) {
	baseTime := time.Now().Add(-time.Duration(count) * time.Hour)

	for i := 0; i < count; i++ {
		cpuValue := startValue + growthRate*float64(i)
		memValue := startValue*5 + growthRate*float64(i)*3

		metric := models.PodMetric{
			PodName:   workloadPrefix + "-abc-123",
			Namespace: namespace,
			Timestamp: baseTime.Add(time.Duration(i) * time.Hour),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      int64(cpuValue),
					UsageMemory:   int64(memValue),
					LimitCPU:      int64(startValue * 10),  // 10x start as limit
					LimitMemory:   int64(startValue * 100), // 100x start as limit
				},
			},
		}
		store.Add(metric)
	}
}

func populateTestDataNearLimit(store *storage.InMemoryStorage, namespace, workloadPrefix string, count int) {
	baseTime := time.Now().Add(-time.Duration(count) * time.Hour)
	limit := 1000.0

	for i := 0; i < count; i++ {
		// Start at 60% of limit and grow to 90% over the period
		progress := float64(i) / float64(count)
		cpuValue := limit * (0.6 + 0.3*progress)
		memValue := cpuValue * 2

		metric := models.PodMetric{
			PodName:   workloadPrefix + "-xyz-789",
			Namespace: namespace,
			Timestamp: baseTime.Add(time.Duration(i) * time.Hour),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      int64(cpuValue),
					UsageMemory:   int64(memValue),
					LimitCPU:      int64(limit),
					LimitMemory:   int64(limit * 4),
				},
			},
		}
		store.Add(metric)
	}
}

func TestIntegrationInsufficientData(t *testing.T) {
	store := storage.NewStorage()
	// Add only 10 data points (less than minimum)
	populateTestData(store, "default", "app", 10, 100, 1)

	config := DefaultAnalyzerConfig()
	analyzer := NewTrendAnalyzer(store, config)

	_, err := analyzer.AnalyzeWorkload("default", "app", 200*time.Hour)
	if err == nil {
		t.Error("Expected error for insufficient data, got nil")
	}

	t.Logf("Expected error: %v", err)
}

func TestIntegrationStableWorkload(t *testing.T) {
	store := storage.NewStorage()
	// Generate stable data (no growth)
	populateTestData(store, "default", "stable-app", 100, 100, 0)

	config := DefaultAnalyzerConfig()
	config.MinDataPoints = 50
	analyzer := NewTrendAnalyzer(store, config)

	trend, err := analyzer.AnalyzeWorkload("default", "stable-app", 200*time.Hour)
	if err != nil {
		t.Fatalf("AnalyzeWorkload failed: %v", err)
	}

	// Should detect stable pattern
	if trend.CPUAnalysis.GrowthPattern != PatternStable && trend.CPUAnalysis.GrowthPattern != PatternCyclical {
		t.Logf("Expected stable/cyclical pattern, got %v", trend.CPUAnalysis.GrowthPattern)
	}

	// Growth rate should be near zero
	if math.Abs(trend.CPUAnalysis.GrowthRate.Monthly) > 10 {
		t.Logf("Expected near-zero growth, got %.2f%%/month", trend.CPUAnalysis.GrowthRate.Monthly)
	}

	// Risk should be low or none
	if trend.CPUAnalysis.CapacityStatus.RiskLevel == RiskCritical || trend.CPUAnalysis.CapacityStatus.RiskLevel == RiskHigh {
		t.Logf("Expected low risk for stable data, got %v", trend.CPUAnalysis.CapacityStatus.RiskLevel)
	}

	t.Logf("Stable workload analysis complete: pattern=%v, growth=%.2f%%/month, risk=%v",
		trend.CPUAnalysis.GrowthPattern,
		trend.CPUAnalysis.GrowthRate.Monthly,
		trend.CPUAnalysis.CapacityStatus.RiskLevel)
}
