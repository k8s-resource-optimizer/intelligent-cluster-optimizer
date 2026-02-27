package sla

import (
	"testing"
	"time"
)

func TestHealthChecker_CheckHealth(t *testing.T) {
	checker := NewHealthChecker()

	now := time.Now()
	metrics := []Metric{
		{Timestamp: now.Add(-4 * time.Minute), Value: 80.0},
		{Timestamp: now.Add(-3 * time.Minute), Value: 90.0},
		{Timestamp: now.Add(-2 * time.Minute), Value: 85.0},
		{Timestamp: now.Add(-1 * time.Minute), Value: 95.0},
		{Timestamp: now, Value: 88.0},
	}

	slas := []SLADefinition{
		{
			Name:        "latency",
			Type:        SLATypeLatency,
			Target:      100.0,
			Threshold:   50.0,
			Percentile:  95.0,
			Window:      5 * time.Minute,
			Description: "P95 latency SLA",
		},
	}

	result, err := checker.CheckHealth(metrics, slas)
	if err != nil {
		t.Fatalf("Failed to check health: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
		return
	}

	if !result.IsHealthy {
		t.Errorf("Expected healthy status, got unhealthy: %s", result.Message)
	}

	if result.Score < 0 || result.Score > 100 {
		t.Errorf("Health score out of range: %.2f", result.Score)
	}

	t.Logf("Health check result: Score=%.2f, IsHealthy=%v, Message=%s, Violations=%d, Outliers=%d",
		result.Score, result.IsHealthy, result.Message, len(result.Violations), len(result.Outliers))
}

func TestHealthChecker_UnhealthySystem(t *testing.T) {
	checker := NewHealthChecker()

	now := time.Now()
	// Metrics with severe violations
	metrics := []Metric{
		{Timestamp: now.Add(-4 * time.Minute), Value: 300.0}, // High latency
		{Timestamp: now.Add(-3 * time.Minute), Value: 310.0},
		{Timestamp: now.Add(-2 * time.Minute), Value: 320.0},
		{Timestamp: now.Add(-1 * time.Minute), Value: 315.0},
		{Timestamp: now, Value: 325.0},
	}

	slas := []SLADefinition{
		{
			Name:        "latency",
			Type:        SLATypeLatency,
			Target:      100.0,
			Threshold:   50.0,
			Percentile:  95.0,
			Window:      5 * time.Minute,
			Description: "P95 latency SLA",
		},
	}

	result, err := checker.CheckHealth(metrics, slas)
	if err != nil {
		t.Fatalf("Failed to check health: %v", err)
	}

	if result.IsHealthy {
		t.Errorf("Expected unhealthy status for high latency metrics, got score=%.2f", result.Score)
	}

	if len(result.Violations) == 0 {
		t.Error("Expected violations to be detected")
	}

	t.Logf("Unhealthy system: Score=%.2f, Violations=%d, Message=%s",
		result.Score, len(result.Violations), result.Message)
}

func TestHealthChecker_PreOptimizationCheck(t *testing.T) {
	checker := NewHealthChecker()

	now := time.Now()
	// Use only latency metrics that are within acceptable range
	metrics := []Metric{
		{Timestamp: now.Add(-4 * time.Minute), Value: 80.0},
		{Timestamp: now.Add(-3 * time.Minute), Value: 85.0},
		{Timestamp: now.Add(-2 * time.Minute), Value: 90.0},
		{Timestamp: now.Add(-1 * time.Minute), Value: 88.0},
		{Timestamp: now, Value: 92.0},
	}

	// Check only latency SLA
	slas := []SLADefinition{
		{
			Name:        "response-time",
			Type:        SLATypeLatency,
			Target:      100.0,
			Threshold:   50.0,
			Percentile:  95.0,
			Window:      5 * time.Minute,
			Description: "P95 response time should be under 150ms",
		},
	}

	result, err := checker.CheckHealth(metrics, slas)
	if err != nil {
		t.Fatalf("Failed pre-optimization check: %v", err)
	}

	if !result.IsHealthy {
		t.Errorf("Expected healthy status before optimization: %s", result.Message)
	}

	t.Logf("Pre-optimization health: Score=%.2f, IsHealthy=%v", result.Score, result.IsHealthy)
}

func TestHealthChecker_PostOptimizationCheck(t *testing.T) {
	checker := NewHealthChecker()

	now := time.Now()
	metrics := []Metric{
		{Timestamp: now.Add(-4 * time.Minute), Value: 75.0},
		{Timestamp: now.Add(-3 * time.Minute), Value: 78.0},
		{Timestamp: now.Add(-2 * time.Minute), Value: 82.0},
		{Timestamp: now.Add(-1 * time.Minute), Value: 80.0},
		{Timestamp: now, Value: 85.0},
	}

	// Check only latency SLA
	slas := []SLADefinition{
		{
			Name:        "response-time",
			Type:        SLATypeLatency,
			Target:      100.0,
			Threshold:   50.0,
			Percentile:  95.0,
			Window:      5 * time.Minute,
			Description: "P95 response time should be under 150ms",
		},
	}

	result, err := checker.CheckHealth(metrics, slas)
	if err != nil {
		t.Fatalf("Failed post-optimization check: %v", err)
	}

	if !result.IsHealthy {
		t.Errorf("Expected healthy status after optimization: %s", result.Message)
	}

	t.Logf("Post-optimization health: Score=%.2f, IsHealthy=%v", result.Score, result.IsHealthy)
}

func TestHealthChecker_CompareHealth_Improvement(t *testing.T) {
	checker := NewHealthChecker()

	now := time.Now()

	slas := []SLADefinition{
		{
			Name:        "response-time",
			Type:        SLATypeLatency,
			Target:      100.0,
			Threshold:   50.0,
			Percentile:  95.0,
			Window:      5 * time.Minute,
			Description: "P95 response time should be under 150ms",
		},
	}

	// Pre-optimization: Higher latency
	preMetrics := []Metric{
		{Timestamp: now, Value: 150.0},
		{Timestamp: now, Value: 155.0},
		{Timestamp: now, Value: 160.0},
	}

	// Post-optimization: Lower latency (improvement)
	postMetrics := []Metric{
		{Timestamp: now, Value: 80.0},
		{Timestamp: now, Value: 85.0},
		{Timestamp: now, Value: 90.0},
	}

	preResult, _ := checker.CheckHealth(preMetrics, slas)
	postResult, _ := checker.CheckHealth(postMetrics, slas)

	impact, err := checker.CompareHealth(preResult, postResult)
	if err != nil {
		t.Fatalf("Failed to compare health: %v", err)
	}

	if impact.ImpactScore <= 0 {
		t.Error("Expected positive impact score for improvement")
	}

	t.Logf("Impact analysis: Score=%.2f, Recommendation=%s, ViolationsAdded=%d, ViolationsResolved=%d",
		impact.ImpactScore, impact.Recommendation, len(impact.ViolationsAdded), len(impact.ViolationsResolved))
}

func TestHealthChecker_CompareHealth_Degradation(t *testing.T) {
	checker := NewHealthChecker()

	now := time.Now()

	slas := []SLADefinition{
		{
			Name:        "response-time",
			Type:        SLATypeLatency,
			Target:      100.0,
			Threshold:   50.0,
			Percentile:  95.0,
			Window:      5 * time.Minute,
			Description: "P95 response time should be under 150ms",
		},
	}

	// Pre-optimization: Good latency
	preMetrics := []Metric{
		{Timestamp: now, Value: 80.0},
		{Timestamp: now, Value: 85.0},
		{Timestamp: now, Value: 90.0},
	}

	// Post-optimization: High latency (degradation)
	postMetrics := []Metric{
		{Timestamp: now, Value: 200.0},
		{Timestamp: now, Value: 210.0},
		{Timestamp: now, Value: 220.0},
	}

	preResult, _ := checker.CheckHealth(preMetrics, slas)
	postResult, _ := checker.CheckHealth(postMetrics, slas)

	impact, err := checker.CompareHealth(preResult, postResult)
	if err != nil {
		t.Fatalf("Failed to compare health: %v", err)
	}

	if impact.ImpactScore >= 0 {
		t.Error("Expected negative impact score for degradation")
	}

	t.Logf("Degradation detected: Score=%.2f, Recommendation=%s",
		impact.ImpactScore, impact.Recommendation)
}

func TestIsSystemHealthy(t *testing.T) {
	result := &HealthCheckResult{
		IsHealthy: true,
		Score:     85.0,
	}

	if !IsSystemHealthy(result, 70.0) {
		t.Error("Expected system to be healthy with score 85 and min 70")
	}

	if IsSystemHealthy(result, 90.0) {
		t.Error("Expected system to be unhealthy with score 85 and min 90")
	}
}

func TestShouldBlockOptimization(t *testing.T) {
	tests := []struct {
		name        string
		result      *HealthCheckResult
		shouldBlock bool
	}{
		{
			name: "healthy system",
			result: &HealthCheckResult{
				IsHealthy:  true,
				Score:      90.0,
				Violations: []SLAViolation{},
			},
			shouldBlock: false,
		},
		{
			name: "unhealthy system",
			result: &HealthCheckResult{
				IsHealthy: false,
				Score:     50.0,
			},
			shouldBlock: true,
		},
		{
			name: "critical violation",
			result: &HealthCheckResult{
				IsHealthy: true,
				Score:     85.0,
				Violations: []SLAViolation{
					{
						Severity: 0.9, // Critical
						Message:  "High severity violation",
					},
				},
			},
			shouldBlock: true,
		},
		{
			name:        "nil result",
			result:      nil,
			shouldBlock: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldBlock, reason := ShouldBlockOptimization(tt.result)
			if shouldBlock != tt.shouldBlock {
				t.Errorf("ShouldBlockOptimization() = %v, want %v (reason: %s)",
					shouldBlock, tt.shouldBlock, reason)
			}
			if shouldBlock {
				t.Logf("Optimization blocked: %s", reason)
			}
		})
	}
}

func TestShouldRollback(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		impact         *OptimizationImpact
		shouldRollback bool
	}{
		{
			name: "significant improvement",
			impact: &OptimizationImpact{
				PreOptimization: HealthCheckResult{
					Timestamp: now,
					Score:     60.0,
				},
				PostOptimization: HealthCheckResult{
					Timestamp: now,
					Score:     90.0,
				},
				ImpactScore: 0.3,
			},
			shouldRollback: false,
		},
		{
			name: "significant degradation",
			impact: &OptimizationImpact{
				PreOptimization: HealthCheckResult{
					Timestamp: now,
					Score:     90.0,
				},
				PostOptimization: HealthCheckResult{
					Timestamp: now,
					Score:     50.0,
				},
				ImpactScore: -0.4,
			},
			shouldRollback: true,
		},
		{
			name: "critical violation introduced",
			impact: &OptimizationImpact{
				ImpactScore: -0.05,
				ViolationsAdded: []SLAViolation{
					{
						Severity: 0.9,
						Message:  "Critical violation",
					},
				},
			},
			shouldRollback: true,
		},
		{
			name: "multiple violations introduced",
			impact: &OptimizationImpact{
				ImpactScore: -0.05,
				ViolationsAdded: []SLAViolation{
					{Severity: 0.5},
					{Severity: 0.5},
					{Severity: 0.5},
				},
			},
			shouldRollback: true,
		},
		{
			name:           "nil impact",
			impact:         nil,
			shouldRollback: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldRollback, reason := ShouldRollback(tt.impact)
			if shouldRollback != tt.shouldRollback {
				t.Errorf("ShouldRollback() = %v, want %v (reason: %s)",
					shouldRollback, tt.shouldRollback, reason)
			}
			if shouldRollback {
				t.Logf("Rollback recommended: %s", reason)
			}
		})
	}
}

func TestHealthChecker_ViolationTracking(t *testing.T) {
	checker := NewHealthChecker()

	now := time.Now()

	slas := []SLADefinition{
		{
			Name:        "response-time",
			Type:        SLATypeLatency,
			Target:      100.0,
			Threshold:   50.0,
			Percentile:  95.0,
			Window:      5 * time.Minute,
			Description: "P95 response time should be under 150ms",
		},
	}

	// Pre: One violation
	preMetrics := []Metric{
		{Timestamp: now, Value: 160.0}, // Violates latency SLA
	}

	// Post: No violations
	postMetrics := []Metric{
		{Timestamp: now, Value: 80.0}, // Within SLA
	}

	preResult, _ := checker.CheckHealth(preMetrics, slas)
	postResult, _ := checker.CheckHealth(postMetrics, slas)

	impact, err := checker.CompareHealth(preResult, postResult)
	if err != nil {
		t.Fatalf("Failed to compare health: %v", err)
	}

	// Should have resolved violations
	if len(impact.ViolationsResolved) == 0 {
		t.Error("Expected violations to be resolved")
	}

	if len(impact.ViolationsAdded) > 0 {
		t.Error("Expected no new violations")
	}

	t.Logf("Violations resolved: %d, Violations added: %d",
		len(impact.ViolationsResolved), len(impact.ViolationsAdded))
}
