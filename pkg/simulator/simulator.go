package simulator

import (
	"fmt"
	"time"

	"intelligent-cluster-optimizer/pkg/recommendation"
)

// Scenario defines a what-if simulation scenario
type Scenario struct {
	Name        string              // Scenario name
	Workloads   []string            // Workload names to include (empty = all)
	Strategy    Strategy            // Optimization strategy
	TimeHorizon time.Duration       // Simulation duration (7d, 30d, 90d)
	Assumptions ScenarioAssumptions // Additional assumptions
}

// Strategy defines optimization aggressiveness
type Strategy string

const (
	StrategyAggressive   Strategy = "aggressive"   // Maximize cost savings, higher risk
	StrategyBalanced     Strategy = "balanced"     // Balance cost and safety
	StrategyConservative Strategy = "conservative" // Prioritize safety, minimal changes
)

// ScenarioAssumptions contains simulation assumptions
type ScenarioAssumptions struct {
	TrafficGrowth         float64 // Expected traffic growth % (e.g., 0.1 = 10% increase)
	AllowDowntime         bool    // Whether brief downtime is acceptable
	SLAViolationTolerance float64 // Acceptable SLA violation probability (0.0-1.0)
}

// SimulationResult contains the simulation outcome
type SimulationResult struct {
	Scenario          Scenario
	TotalWorkloads    int
	AffectedWorkloads int
	EstimatedSavings  SavingsProjection
	RiskScore         float64 // 0-100, higher = riskier
	RiskLevel         string  // "low", "medium", "high", "critical"
	SLAImpactProb     float64 // Probability of SLA violation (0.0-1.0)
	ResourceChanges   []ResourceDelta
	Confidence        float64 // Overall confidence in simulation (0-100)
	Warnings          []string
	Recommendations   []string
}

// SavingsProjection shows projected savings over time
type SavingsProjection struct {
	Daily    float64
	Weekly   float64
	Monthly  float64
	Yearly   float64
	Currency string
}

// ResourceDelta shows before/after resource changes
type ResourceDelta struct {
	Workload          string
	Container         string
	CurrentCPU        int64
	RecommendedCPU    int64
	CPUChange         string // e.g., "-40%"
	CurrentMemory     int64
	RecommendedMemory int64
	MemoryChange      string // e.g., "-25%"
	MonthlySavings    float64
	RiskScore         float64
}

// Simulator runs what-if simulations
type Simulator struct {
	// No state needed for now
}

// NewSimulator creates a new simulator
func NewSimulator() *Simulator {
	return &Simulator{}
}

// Simulate runs a what-if simulation
func Simulate(scenario Scenario, recommendations []recommendation.WorkloadRecommendation) (*SimulationResult, error) {
	if len(recommendations) == 0 {
		return nil, fmt.Errorf("no recommendations provided")
	}

	result := &SimulationResult{
		Scenario:        scenario,
		TotalWorkloads:  len(recommendations),
		ResourceChanges: []ResourceDelta{},
		Warnings:        []string{},
		Recommendations: []string{},
	}

	// Filter recommendations based on scenario workloads
	filteredRecs := filterRecommendations(recommendations, scenario.Workloads)
	result.AffectedWorkloads = len(filteredRecs)

	// Apply strategy adjustments
	adjustedRecs := applyStrategy(filteredRecs, scenario.Strategy)

	// Calculate resource changes and savings
	var totalDailySavings float64
	var totalRisk float64
	var highRiskCount int

	for _, rec := range adjustedRecs {
		for _, container := range rec.Containers {
			// Calculate changes
			delta := calculateDelta(rec, container)

			// Apply traffic growth assumptions
			if scenario.Assumptions.TrafficGrowth > 0 {
				delta = applyTrafficGrowth(delta, scenario.Assumptions.TrafficGrowth)
			}

			result.ResourceChanges = append(result.ResourceChanges, delta)

			// Accumulate savings
			if container.EstimatedSavings != nil {
				totalDailySavings += container.EstimatedSavings.SavingsPerDay
			}

			// Accumulate risk
			totalRisk += delta.RiskScore
			if delta.RiskScore > 60 {
				highRiskCount++
			}
		}
	}

	// Calculate projected savings
	result.EstimatedSavings = SavingsProjection{
		Daily:    totalDailySavings,
		Weekly:   totalDailySavings * 7,
		Monthly:  totalDailySavings * 30,
		Yearly:   totalDailySavings * 365,
		Currency: "USD",
	}

	// Calculate overall risk score
	if len(result.ResourceChanges) > 0 {
		result.RiskScore = totalRisk / float64(len(result.ResourceChanges))
	}
	result.RiskLevel = calculateRiskLevel(result.RiskScore)

	// Calculate SLA impact probability
	result.SLAImpactProb = calculateSLAImpactProb(result.ResourceChanges, scenario.Assumptions)

	// Calculate confidence
	result.Confidence = calculateSimulationConfidence(adjustedRecs)

	// Generate warnings
	result.Warnings = generateWarnings(result, scenario)

	// Generate recommendations
	result.Recommendations = generateRecommendations(result, scenario)

	return result, nil
}

// filterRecommendations filters by workload names
func filterRecommendations(recs []recommendation.WorkloadRecommendation, workloads []string) []recommendation.WorkloadRecommendation {
	if len(workloads) == 0 {
		return recs // Include all
	}

	filtered := []recommendation.WorkloadRecommendation{}
	workloadMap := make(map[string]bool)
	for _, w := range workloads {
		workloadMap[w] = true
	}

	for _, rec := range recs {
		if workloadMap[rec.WorkloadName] {
			filtered = append(filtered, rec)
		}
	}

	return filtered
}

// applyStrategy adjusts recommendations based on strategy
func applyStrategy(recs []recommendation.WorkloadRecommendation, strategy Strategy) []recommendation.WorkloadRecommendation {
	adjusted := make([]recommendation.WorkloadRecommendation, len(recs))
	copy(adjusted, recs)

	for i := range adjusted {
		for j := range adjusted[i].Containers {
			container := &adjusted[i].Containers[j]

			switch strategy {
			case StrategyAggressive:
				// More aggressive cuts (if reducing), reduce safety margin
				if container.RecommendedCPU < container.CurrentCPU {
					reduction := float64(container.CurrentCPU-container.RecommendedCPU) * 1.2
					container.RecommendedCPU = container.CurrentCPU - int64(reduction)
					if container.RecommendedCPU < 10 {
						container.RecommendedCPU = 10 // Minimum 10m
					}
				}
				if container.RecommendedMemory < container.CurrentMemory {
					reduction := float64(container.CurrentMemory-container.RecommendedMemory) * 1.2
					container.RecommendedMemory = container.CurrentMemory - int64(reduction)
					if container.RecommendedMemory < 10*1024*1024 {
						container.RecommendedMemory = 10 * 1024 * 1024 // Minimum 10Mi
					}
				}

			case StrategyConservative:
				// Less aggressive cuts, increase safety margin
				if container.RecommendedCPU < container.CurrentCPU {
					reduction := float64(container.CurrentCPU-container.RecommendedCPU) * 0.5
					container.RecommendedCPU = container.CurrentCPU - int64(reduction)
				}
				if container.RecommendedMemory < container.CurrentMemory {
					reduction := float64(container.CurrentMemory-container.RecommendedMemory) * 0.5
					container.RecommendedMemory = container.CurrentMemory - int64(reduction)
				}

			case StrategyBalanced:
				// Use recommendations as-is
			}
		}
	}

	return adjusted
}

// calculateDelta calculates resource changes
func calculateDelta(rec recommendation.WorkloadRecommendation, container recommendation.ContainerRecommendation) ResourceDelta {
	cpuChange := calculatePercentChange(container.CurrentCPU, container.RecommendedCPU)
	memoryChange := calculatePercentChange(container.CurrentMemory, container.RecommendedMemory)

	monthlySavings := 0.0
	if container.EstimatedSavings != nil {
		monthlySavings = container.EstimatedSavings.SavingsPerMonth
	}

	// Calculate risk based on magnitude of change
	riskScore := calculateChangeRisk(cpuChange, memoryChange, container.Confidence)

	return ResourceDelta{
		Workload:          fmt.Sprintf("%s/%s", rec.WorkloadKind, rec.WorkloadName),
		Container:         container.ContainerName,
		CurrentCPU:        container.CurrentCPU,
		RecommendedCPU:    container.RecommendedCPU,
		CPUChange:         formatPercentChange(cpuChange),
		CurrentMemory:     container.CurrentMemory,
		RecommendedMemory: container.RecommendedMemory,
		MemoryChange:      formatPercentChange(memoryChange),
		MonthlySavings:    monthlySavings,
		RiskScore:         riskScore,
	}
}

// calculatePercentChange calculates percentage change
func calculatePercentChange(current, recommended int64) float64 {
	if current == 0 {
		return 0
	}
	return float64(current-recommended) / float64(current) * 100
}

// formatPercentChange formats percentage change with sign
func formatPercentChange(change float64) string {
	if change > 0 {
		return fmt.Sprintf("-%.1f%%", change)
	} else if change < 0 {
		return fmt.Sprintf("+%.1f%%", -change)
	}
	return "0%"
}

// calculateChangeRisk calculates risk score based on change magnitude
func calculateChangeRisk(cpuChange, memoryChange, confidence float64) float64 {
	// Higher changes = higher risk
	// Lower confidence = higher risk

	cpuRisk := cpuChange * 0.5                 // Max 50 points
	memoryRisk := memoryChange * 0.5           // Max 50 points
	confidenceRisk := (100 - confidence) * 0.3 // Max 30 points

	totalRisk := cpuRisk + memoryRisk + confidenceRisk

	// Cap at 100
	if totalRisk > 100 {
		totalRisk = 100
	}
	if totalRisk < 0 {
		totalRisk = 0
	}

	return totalRisk
}

// calculateRiskLevel determines risk level from score
func calculateRiskLevel(score float64) string {
	switch {
	case score >= 75:
		return "critical"
	case score >= 50:
		return "high"
	case score >= 25:
		return "medium"
	default:
		return "low"
	}
}

// calculateSLAImpactProb estimates SLA violation probability
func calculateSLAImpactProb(deltas []ResourceDelta, assumptions ScenarioAssumptions) float64 {
	if len(deltas) == 0 {
		return 0
	}

	var totalProb float64
	for _, delta := range deltas {
		// Higher risk = higher SLA impact probability
		prob := delta.RiskScore / 100.0 * 0.3 // Max 30% per workload
		totalProb += prob
	}

	// Average probability
	avgProb := totalProb / float64(len(deltas))

	// Adjust for traffic growth
	if assumptions.TrafficGrowth > 0 {
		avgProb *= (1 + assumptions.TrafficGrowth)
	}

	// Cap at 100%
	if avgProb > 1.0 {
		avgProb = 1.0
	}

	return avgProb
}

// calculateSimulationConfidence calculates overall confidence
func calculateSimulationConfidence(recs []recommendation.WorkloadRecommendation) float64 {
	if len(recs) == 0 {
		return 0
	}

	var totalConfidence float64
	var count int

	for _, rec := range recs {
		for _, container := range rec.Containers {
			totalConfidence += container.Confidence
			count++
		}
	}

	if count == 0 {
		return 0
	}

	return totalConfidence / float64(count)
}

// applyTrafficGrowth adjusts delta for expected traffic growth
func applyTrafficGrowth(delta ResourceDelta, growthRate float64) ResourceDelta {
	// Reduce savings by growth rate (higher traffic = less savings potential)
	delta.MonthlySavings *= (1 - growthRate*0.5)

	// Increase risk (growth makes reductions riskier)
	delta.RiskScore *= (1 + growthRate)
	if delta.RiskScore > 100 {
		delta.RiskScore = 100
	}

	return delta
}

// generateWarnings generates warnings based on simulation results
func generateWarnings(result *SimulationResult, scenario Scenario) []string {
	warnings := []string{}

	if result.RiskScore > 60 {
		warnings = append(warnings, fmt.Sprintf("High risk score (%.1f%%) - changes may impact stability", result.RiskScore))
	}

	if result.SLAImpactProb > 0.1 {
		warnings = append(warnings, fmt.Sprintf("%.1f%% probability of SLA impact - consider phased rollout", result.SLAImpactProb*100))
	}

	if result.Confidence < 70 {
		warnings = append(warnings, fmt.Sprintf("Low confidence (%.1f%%) - recommendations based on limited data", result.Confidence))
	}

	highRiskChanges := 0
	for _, delta := range result.ResourceChanges {
		if delta.RiskScore > 70 {
			highRiskChanges++
		}
	}
	if highRiskChanges > 0 {
		warnings = append(warnings, fmt.Sprintf("%d workload(s) have high-risk changes - review individually", highRiskChanges))
	}

	return warnings
}

// generateRecommendations generates actionable recommendations
func generateRecommendations(result *SimulationResult, scenario Scenario) []string {
	recs := []string{}

	if result.RiskScore > 50 {
		recs = append(recs, "Consider using 'conservative' strategy to reduce risk")
	}

	if result.SLAImpactProb > 0.15 {
		recs = append(recs, "Apply changes during low-traffic maintenance window")
		recs = append(recs, "Implement phased rollout (10% → 50% → 100%)")
	}

	if result.EstimatedSavings.Monthly > 1000 {
		recs = append(recs, fmt.Sprintf("Significant savings potential: $%.2f/month", result.EstimatedSavings.Monthly))
	}

	if len(result.Warnings) == 0 && result.RiskScore < 30 {
		recs = append(recs, "Low risk - safe to proceed with changes")
	}

	return recs
}
