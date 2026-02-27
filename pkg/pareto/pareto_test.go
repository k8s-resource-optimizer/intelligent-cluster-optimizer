package pareto

import (
	"testing"
)

// === Objective Tests ===

func TestSolution_AddObjective(t *testing.T) {
	sol := NewSolution("test", "default", "app")

	sol.AddObjective(ObjectiveCost, "Cost", 10.5, 0.3, true)

	if len(sol.Objectives) != 1 {
		t.Errorf("Expected 1 objective, got %d", len(sol.Objectives))
	}

	obj := sol.Objectives[ObjectiveCost]
	if obj.Value != 10.5 {
		t.Errorf("Expected value 10.5, got %f", obj.Value)
	}
	if obj.Weight != 0.3 {
		t.Errorf("Expected weight 0.3, got %f", obj.Weight)
	}
	if !obj.IsMinimize {
		t.Error("Expected IsMinimize to be true")
	}
}

func TestSolution_SetCostObjective(t *testing.T) {
	sol := NewSolution("test", "default", "app")
	sol.CPURequest = 1000                  // 1 core
	sol.MemoryRequest = 1024 * 1024 * 1024 // 1 GB

	sol.SetCostObjective(0.05, 0.01, 0.25)

	obj := sol.Objectives[ObjectiveCost]
	if obj == nil {
		t.Fatal("Cost objective not set")
		return
	}

	// Expected: 1 core * 0.05 + 1 GB * 0.01 = 0.06
	expectedCost := 0.06
	tolerance := 0.0001
	if obj.Value < expectedCost-tolerance || obj.Value > expectedCost+tolerance {
		t.Errorf("Expected cost ~%f, got %f", expectedCost, obj.Value)
	}
}

func TestSolution_Dominates(t *testing.T) {
	// Solution A: better cost, worse performance
	solA := NewSolution("a", "default", "app")
	solA.AddObjective(ObjectiveCost, "Cost", 5.0, 0.5, true)          // Lower is better
	solA.AddObjective(ObjectivePerformance, "Perf", 60.0, 0.5, false) // Higher is better

	// Solution B: worse cost, better performance
	solB := NewSolution("b", "default", "app")
	solB.AddObjective(ObjectiveCost, "Cost", 10.0, 0.5, true)
	solB.AddObjective(ObjectivePerformance, "Perf", 80.0, 0.5, false)

	// Neither should dominate the other (trade-off)
	if solA.Dominates(solB) {
		t.Error("A should not dominate B (trade-off)")
	}
	if solB.Dominates(solA) {
		t.Error("B should not dominate A (trade-off)")
	}

	// Solution C: better in everything
	solC := NewSolution("c", "default", "app")
	solC.AddObjective(ObjectiveCost, "Cost", 4.0, 0.5, true)
	solC.AddObjective(ObjectivePerformance, "Perf", 90.0, 0.5, false)

	// C should dominate both A and B
	if !solC.Dominates(solA) {
		t.Error("C should dominate A")
	}
	if !solC.Dominates(solB) {
		t.Error("C should dominate B")
	}
}

func TestSolution_CalculateOverallScore(t *testing.T) {
	sol := NewSolution("test", "default", "app")

	// Add objectives with normalized values
	sol.AddObjective(ObjectiveCost, "Cost", 0.5, 0.5, true)
	sol.AddObjective(ObjectivePerformance, "Perf", 0.8, 0.5, false)

	// Set normalized values
	sol.Objectives[ObjectiveCost].Normalized = 0.5
	sol.Objectives[ObjectivePerformance].Normalized = 0.8

	sol.CalculateOverallScore()

	// Cost (minimize): score = 1 - 0.5 = 0.5
	// Performance (maximize): score = 0.8
	// Weighted: (0.5 * 0.5 + 0.8 * 0.5) / 1.0 = 0.65
	expectedScore := 0.65
	if sol.OverallScore != expectedScore {
		t.Errorf("Expected score %f, got %f", expectedScore, sol.OverallScore)
	}
}

// === Optimizer Tests ===

func TestOptimizer_Optimize(t *testing.T) {
	opt := NewOptimizer()

	// Create test solutions
	solutions := []*Solution{
		createTestSolution("cheap", 5.0, 60.0),
		createTestSolution("balanced", 7.5, 75.0),
		createTestSolution("expensive", 10.0, 90.0),
	}

	result := opt.Optimize(solutions)

	if result == nil {
		t.Fatal("Optimize returned nil")
		return
	}

	if len(result.AllSolutions) != 3 {
		t.Errorf("Expected 3 solutions, got %d", len(result.AllSolutions))
	}

	// All solutions should be on Pareto frontier (trade-offs)
	if len(result.ParetoFrontier) != 3 {
		t.Errorf("Expected 3 solutions on frontier, got %d", len(result.ParetoFrontier))
	}

	if result.BestSolution == nil {
		t.Error("BestSolution should not be nil")
	}
}

func TestOptimizer_Optimize_WithDomination(t *testing.T) {
	opt := NewOptimizer()

	// Create solutions where one dominates another
	solutions := []*Solution{
		createTestSolution("dominated", 10.0, 60.0),  // Worse in both
		createTestSolution("dominant", 5.0, 90.0),    // Better in both
		createTestSolution("alternative", 7.0, 70.0), // Trade-off with dominant
	}

	result := opt.Optimize(solutions)

	// Dominated solution should have higher rank
	for _, s := range result.AllSolutions {
		if s.ID == "dominated" && s.ParetoRank == 0 {
			t.Error("Dominated solution should not be on Pareto frontier")
		}
		if s.ID == "dominant" && s.ParetoRank != 0 {
			t.Error("Dominant solution should be on Pareto frontier")
		}
	}

	// Frontier should not include dominated solution
	for _, s := range result.ParetoFrontier {
		if s.ID == "dominated" {
			t.Error("Dominated solution should not be on Pareto frontier")
		}
	}
}

func TestOptimizer_GenerateSolutionSet(t *testing.T) {
	opt := NewOptimizer()

	solutions := opt.GenerateSolutionSet(
		"default", "myapp",
		1000, 1024*1024*1024, // current
		500, 512*1024*1024, // avg
		800, 768*1024*1024, // peak
		700, 700*1024*1024, // p95
		900, 900*1024*1024, // p99
		85.0, // confidence
	)

	// Should generate 6 strategies
	if len(solutions) != 6 {
		t.Errorf("Expected 6 solutions, got %d", len(solutions))
	}

	// Check that different strategies exist
	strategies := make(map[string]bool)
	for _, s := range solutions {
		strategies[s.ID] = true
	}

	expectedStrategies := []string{"conservative", "balanced", "aggressive", "cost-optimized", "performance", "current"}
	for _, strategy := range expectedStrategies {
		if !strategies[strategy] {
			t.Errorf("Missing strategy: %s", strategy)
		}
	}

	// Each solution should have all objectives
	for _, s := range solutions {
		if len(s.Objectives) < 5 {
			t.Errorf("Solution %s missing objectives, has %d", s.ID, len(s.Objectives))
		}
	}
}

func TestOptimizer_SelectBestForProfile(t *testing.T) {
	opt := NewOptimizer()

	solutions := opt.GenerateSolutionSet(
		"default", "myapp",
		1000, 1024*1024*1024,
		500, 512*1024*1024,
		800, 768*1024*1024,
		700, 700*1024*1024,
		900, 900*1024*1024,
		85.0,
	)

	result := opt.Optimize(solutions)

	// Production should prioritize reliability
	prodChoice := opt.SelectBestForProfile(result, "production")
	if prodChoice == nil {
		t.Fatal("Production choice should not be nil")
		return
	}

	// Development should prioritize cost
	devChoice := opt.SelectBestForProfile(result, "development")
	if devChoice == nil {
		t.Fatal("Development choice should not be nil")
		return
	}

	// They may or may not be the same depending on the frontier
	t.Logf("Production choice: %s", prodChoice.ID)
	t.Logf("Development choice: %s", devChoice.ID)
}

// === Recommendation Helper Tests ===

func TestRecommendationHelper_GenerateRecommendation(t *testing.T) {
	helper := NewRecommendationHelper()

	metrics := &WorkloadMetrics{
		Namespace:     "default",
		WorkloadName:  "test-app",
		CurrentCPU:    1000,
		CurrentMemory: 1024 * 1024 * 1024, // 1Gi
		AvgCPU:        400,
		AvgMemory:     400 * 1024 * 1024,
		PeakCPU:       800,
		PeakMemory:    800 * 1024 * 1024,
		P95CPU:        700,
		P95Memory:     700 * 1024 * 1024,
		P99CPU:        850,
		P99Memory:     850 * 1024 * 1024,
		Confidence:    85.0,
		SampleCount:   100,
	}

	rec, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("GenerateRecommendation failed: %v", err)
	}

	if rec == nil {
		t.Fatal("Recommendation should not be nil")
		return
	}

	if rec.BestSolution == nil {
		t.Error("BestSolution should not be nil")
	}

	if len(rec.ParetoFrontier) == 0 {
		t.Error("ParetoFrontier should not be empty")
	}

	if rec.Summary == "" {
		t.Error("Summary should not be empty")
	}

	t.Logf("Recommendation: %s", rec.Summary)
}

func TestRecommendationHelper_CompareStrategies(t *testing.T) {
	helper := NewRecommendationHelper()

	metrics := &WorkloadMetrics{
		Namespace:     "default",
		WorkloadName:  "test-app",
		CurrentCPU:    1000,
		CurrentMemory: 1024 * 1024 * 1024,
		AvgCPU:        500,
		AvgMemory:     512 * 1024 * 1024,
		PeakCPU:       800,
		PeakMemory:    768 * 1024 * 1024,
		P95CPU:        700,
		P95Memory:     700 * 1024 * 1024,
		P99CPU:        850,
		P99Memory:     850 * 1024 * 1024,
		Confidence:    90.0,
		SampleCount:   200,
	}

	rec, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("GenerateRecommendation failed: %v", err)
	}

	comparisons := helper.CompareStrategies(rec)

	if len(comparisons) == 0 {
		t.Error("Should have strategy comparisons")
	}

	for _, comp := range comparisons {
		if comp.Strategy == "" {
			t.Error("Strategy name should not be empty")
		}
		t.Logf("Strategy %s: CPU=%dm, Memory=%dMi, Score=%.2f",
			comp.Strategy, comp.CPURequest, comp.MemoryRequest/(1024*1024), comp.OverallScore)
	}
}

func TestRecommendationHelper_AnalyzeTradeOffs(t *testing.T) {
	helper := NewRecommendationHelper()

	metrics := &WorkloadMetrics{
		Namespace:     "default",
		WorkloadName:  "test-app",
		CurrentCPU:    1000,
		CurrentMemory: 1024 * 1024 * 1024,
		AvgCPU:        500,
		AvgMemory:     512 * 1024 * 1024,
		PeakCPU:       800,
		PeakMemory:    768 * 1024 * 1024,
		P95CPU:        700,
		P95Memory:     700 * 1024 * 1024,
		P99CPU:        850,
		P99Memory:     850 * 1024 * 1024,
		Confidence:    90.0,
		SampleCount:   200,
	}

	rec, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("GenerateRecommendation failed: %v", err)
	}

	analysis := helper.AnalyzeTradeOffs(rec)

	if analysis == "" {
		t.Error("Trade-off analysis should not be empty")
	}

	t.Logf("Trade-off analysis:\n%s", analysis)
}

func TestRecommendationHelper_GetRecommendationForProfile(t *testing.T) {
	helper := NewRecommendationHelper()

	metrics := &WorkloadMetrics{
		Namespace:     "default",
		WorkloadName:  "test-app",
		CurrentCPU:    1000,
		CurrentMemory: 1024 * 1024 * 1024,
		AvgCPU:        500,
		AvgMemory:     512 * 1024 * 1024,
		PeakCPU:       800,
		PeakMemory:    768 * 1024 * 1024,
		P95CPU:        700,
		P95Memory:     700 * 1024 * 1024,
		P99CPU:        850,
		P99Memory:     850 * 1024 * 1024,
		Confidence:    85.0,
		SampleCount:   100,
	}

	rec, err := helper.GenerateRecommendation(metrics)
	if err != nil {
		t.Fatalf("Failed to generate recommendation: %v", err)
	}

	profiles := []string{"production", "development", "performance", "staging"}
	for _, profile := range profiles {
		sol := helper.GetRecommendationForProfile(rec, profile)
		if sol == nil {
			t.Errorf("Should get recommendation for profile %s", profile)
			continue
		}
		t.Logf("Profile %s: %s (CPU=%dm)", profile, sol.ID, sol.CPURequest)
	}
}

func TestRecommendationHelper_SetObjectiveWeights(t *testing.T) {
	helper := NewRecommendationHelper()

	// Set custom weights
	helper.SetObjectiveWeights(0.5, 0.1, 0.2, 0.1, 0.1)

	if helper.optimizer.CostWeight != 0.5 {
		t.Errorf("Expected cost weight 0.5, got %f", helper.optimizer.CostWeight)
	}
	if helper.optimizer.PerformanceWeight != 0.1 {
		t.Errorf("Expected performance weight 0.1, got %f", helper.optimizer.PerformanceWeight)
	}
}

// === Edge Cases ===

func TestOptimizer_EmptySolutions(t *testing.T) {
	opt := NewOptimizer()

	result := opt.Optimize([]*Solution{})

	if result == nil {
		t.Fatal("Should return result even with empty solutions")
		return
	}

	if len(result.ParetoFrontier) != 0 {
		t.Error("Pareto frontier should be empty")
	}

	if result.BestSolution != nil {
		t.Error("Best solution should be nil for empty input")
	}
}

func TestOptimizer_SingleSolution(t *testing.T) {
	opt := NewOptimizer()

	sol := createTestSolution("only", 5.0, 80.0)
	result := opt.Optimize([]*Solution{sol})

	if len(result.ParetoFrontier) != 1 {
		t.Error("Single solution should be on frontier")
	}

	if result.BestSolution != sol {
		t.Error("Best solution should be the only solution")
	}

	if sol.ParetoRank != 0 {
		t.Error("Single solution should have rank 0")
	}
}

func TestRecommendationHelper_NilMetrics(t *testing.T) {
	helper := NewRecommendationHelper()

	_, err := helper.GenerateRecommendation(nil)
	if err == nil {
		t.Error("Should return error for nil metrics")
	}
}

// === Benchmarks ===

func BenchmarkOptimizer_Optimize(b *testing.B) {
	opt := NewOptimizer()

	solutions := make([]*Solution, 20)
	for i := 0; i < 20; i++ {
		solutions[i] = createTestSolution(
			"sol",
			float64(i)*0.5+1,
			float64(20-i)*5,
		)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		opt.Optimize(solutions)
	}
}

func BenchmarkRecommendationHelper_GenerateRecommendation(b *testing.B) {
	helper := NewRecommendationHelper()

	metrics := &WorkloadMetrics{
		Namespace:     "default",
		WorkloadName:  "test-app",
		CurrentCPU:    1000,
		CurrentMemory: 1024 * 1024 * 1024,
		AvgCPU:        500,
		AvgMemory:     512 * 1024 * 1024,
		PeakCPU:       800,
		PeakMemory:    768 * 1024 * 1024,
		P95CPU:        700,
		P95Memory:     700 * 1024 * 1024,
		P99CPU:        850,
		P99Memory:     850 * 1024 * 1024,
		Confidence:    85.0,
		SampleCount:   100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = helper.GenerateRecommendation(metrics)
	}
}

// Helper function to create test solutions
func createTestSolution(id string, cost, performance float64) *Solution {
	sol := NewSolution(id, "default", "app")
	sol.AddObjective(ObjectiveCost, "Cost", cost, 0.5, true)
	sol.AddObjective(ObjectivePerformance, "Perf", performance, 0.5, false)
	return sol
}
