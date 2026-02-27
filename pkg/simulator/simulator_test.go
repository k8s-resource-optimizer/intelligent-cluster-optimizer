package simulator

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/cost"
	"intelligent-cluster-optimizer/pkg/recommendation"
)

func TestSimulate(t *testing.T) {
	// Create sample recommendations
	recs := []recommendation.WorkloadRecommendation{
		{
			Namespace:    "production",
			WorkloadKind: "Deployment",
			WorkloadName: "api",
			Containers: []recommendation.ContainerRecommendation{
				{
					ContainerName:     "api",
					CurrentCPU:        1000,               // 1 core
					RecommendedCPU:    600,                // 600m
					CurrentMemory:     1024 * 1024 * 1024, // 1Gi
					RecommendedMemory: 768 * 1024 * 1024,  // 768Mi
					Confidence:        85.0,
					EstimatedSavings: &cost.SavingsEstimate{
						SavingsPerDay:   10.0,
						SavingsPerMonth: 300.0,
					},
				},
			},
		},
	}

	// Create scenario
	scenario := Scenario{
		Name:        "Test Simulation",
		Strategy:    StrategyBalanced,
		TimeHorizon: 30 * 24 * time.Hour,
	}

	// Run simulation
	result, err := Simulate(scenario, recs)
	if err != nil {
		t.Fatalf("Simulate failed: %v", err)
	}

	// Verify results
	if result == nil {
		t.Fatal("Result is nil")
		return
	}

	if result.TotalWorkloads != 1 {
		t.Errorf("Expected 1 total workload, got %d", result.TotalWorkloads)
	}

	if result.AffectedWorkloads != 1 {
		t.Errorf("Expected 1 affected workload, got %d", result.AffectedWorkloads)
	}

	if result.EstimatedSavings.Monthly <= 0 {
		t.Errorf("Expected positive monthly savings, got %.2f", result.EstimatedSavings.Monthly)
	}

	if result.RiskScore < 0 || result.RiskScore > 100 {
		t.Errorf("Risk score out of range: %.2f", result.RiskScore)
	}

	if len(result.ResourceChanges) == 0 {
		t.Error("Expected at least one resource change")
	}
}

func TestSimulateNoRecommendations(t *testing.T) {
	scenario := Scenario{
		Name:     "Empty Test",
		Strategy: StrategyBalanced,
	}

	_, err := Simulate(scenario, []recommendation.WorkloadRecommendation{})
	if err == nil {
		t.Error("Expected error for empty recommendations")
	}
}

func TestFilterRecommendations(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		{WorkloadName: "api"},
		{WorkloadName: "web"},
		{WorkloadName: "worker"},
	}

	// Test: filter by specific workloads
	filtered := filterRecommendations(recs, []string{"api", "web"})
	if len(filtered) != 2 {
		t.Errorf("Expected 2 filtered recommendations, got %d", len(filtered))
	}

	// Test: empty filter (should return all)
	filtered = filterRecommendations(recs, []string{})
	if len(filtered) != 3 {
		t.Errorf("Expected 3 recommendations with empty filter, got %d", len(filtered))
	}
}

func TestApplyStrategyAggressive(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		{
			Containers: []recommendation.ContainerRecommendation{
				{
					CurrentCPU:        1000,
					RecommendedCPU:    600,
					CurrentMemory:     1024 * 1024 * 1024,
					RecommendedMemory: 768 * 1024 * 1024,
				},
			},
		},
	}

	adjusted := applyStrategy(recs, StrategyAggressive)

	// Aggressive should reduce more (lower recommended value)
	if adjusted[0].Containers[0].RecommendedCPU >= 600 {
		t.Errorf("Aggressive strategy should reduce CPU more than 600m, got %dm", adjusted[0].Containers[0].RecommendedCPU)
	}

	// Should be less than original recommendation
	if adjusted[0].Containers[0].RecommendedMemory >= 768*1024*1024 {
		t.Error("Aggressive strategy should reduce memory more")
	}
}

func TestApplyStrategyConservative(t *testing.T) {
	recs := []recommendation.WorkloadRecommendation{
		{
			Containers: []recommendation.ContainerRecommendation{
				{
					CurrentCPU:        1000,
					RecommendedCPU:    600,
					CurrentMemory:     1024 * 1024 * 1024,
					RecommendedMemory: 768 * 1024 * 1024,
				},
			},
		},
	}

	adjusted := applyStrategy(recs, StrategyConservative)

	// Conservative should reduce less (higher recommended value, closer to current)
	if adjusted[0].Containers[0].RecommendedCPU <= 600 {
		t.Errorf("Conservative strategy should reduce CPU less than 600m, got %dm", adjusted[0].Containers[0].RecommendedCPU)
	}

	// Should be closer to current than original recommendation
	if adjusted[0].Containers[0].RecommendedCPU >= 1000 {
		t.Error("Conservative should still reduce some CPU")
	}
}

func TestCalculatePercentChange(t *testing.T) {
	tests := []struct {
		name        string
		current     int64
		recommended int64
		expected    float64
	}{
		{"40% reduction", 1000, 600, 40.0},
		{"50% reduction", 1000, 500, 50.0},
		{"No change", 1000, 1000, 0.0},
		{"25% increase", 1000, 1250, -25.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculatePercentChange(tt.current, tt.recommended)
			if result != tt.expected {
				t.Errorf("Expected %.1f%%, got %.1f%%", tt.expected, result)
			}
		})
	}
}

func TestCalculateRiskLevel(t *testing.T) {
	tests := []struct {
		score    float64
		expected string
	}{
		{90, "critical"},
		{75, "critical"},
		{60, "high"},
		{50, "high"},
		{40, "medium"},
		{25, "medium"},
		{15, "low"},
		{5, "low"},
	}

	for _, tt := range tests {
		result := calculateRiskLevel(tt.score)
		if result != tt.expected {
			t.Errorf("Score %.1f: expected %s, got %s", tt.score, tt.expected, result)
		}
	}
}

func TestCalculateChangeRisk(t *testing.T) {
	// High change, low confidence = high risk
	risk := calculateChangeRisk(50.0, 50.0, 50.0)
	if risk < 50 {
		t.Errorf("Expected high risk (>50), got %.2f", risk)
	}

	// Low change, high confidence = low risk
	risk = calculateChangeRisk(10.0, 10.0, 90.0)
	if risk > 30 {
		t.Errorf("Expected low risk (<30), got %.2f", risk)
	}
}

func TestGenerateWarnings(t *testing.T) {
	result := &SimulationResult{
		RiskScore:     70,
		SLAImpactProb: 0.15,
		Confidence:    60,
		ResourceChanges: []ResourceDelta{
			{RiskScore: 80},
		},
	}

	warnings := generateWarnings(result, Scenario{})

	if len(warnings) == 0 {
		t.Error("Expected warnings for high-risk simulation")
	}

	// Should warn about high risk
	foundRiskWarning := false
	for _, w := range warnings {
		if len(w) > 0 {
			foundRiskWarning = true
			break
		}
	}
	if !foundRiskWarning {
		t.Error("Expected risk warning")
	}
}

func TestGenerateRecommendations(t *testing.T) {
	result := &SimulationResult{
		RiskScore:     60,
		SLAImpactProb: 0.2,
		EstimatedSavings: SavingsProjection{
			Monthly: 1500,
		},
		Warnings: []string{"warning"},
	}

	recs := generateRecommendations(result, Scenario{})

	if len(recs) == 0 {
		t.Error("Expected recommendations")
	}
}

func TestApplyTrafficGrowth(t *testing.T) {
	delta := ResourceDelta{
		MonthlySavings: 100.0,
		RiskScore:      30.0,
	}

	adjusted := applyTrafficGrowth(delta, 0.2) // 20% growth

	// Savings should decrease
	if adjusted.MonthlySavings >= delta.MonthlySavings {
		t.Error("Expected reduced savings with traffic growth")
	}

	// Risk should increase
	if adjusted.RiskScore <= delta.RiskScore {
		t.Error("Expected increased risk with traffic growth")
	}
}

func TestFormatPercentChange(t *testing.T) {
	tests := []struct {
		change   float64
		expected string
	}{
		{40.0, "-40.0%"},
		{-25.0, "+25.0%"},
		{0.0, "0%"},
	}

	for _, tt := range tests {
		result := formatPercentChange(tt.change)
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}
