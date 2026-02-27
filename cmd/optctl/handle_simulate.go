package main

import (
	"encoding/json"
	"fmt"
	"os"

	"intelligent-cluster-optimizer/pkg/recommendation"
	"intelligent-cluster-optimizer/pkg/simulator"

	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

func handleSimulate(kubeClient *kubernetes.Clientset, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: optctl simulate [namespace] [--strategy=<aggressive|balanced|conservative>] [--days=<7|30|90>] [--growth=<0.1>]")
	}

	namespace := args[0]

	// Parse flags
	strategy := simulator.StrategyBalanced
	if strategyFlag != "" {
		strategy = simulator.Strategy(strategyFlag)
	}

	days := 30
	if daysFlag > 0 {
		days = daysFlag
	}

	growthRate := 0.0
	if growthFlag != "" {
		if _, err := fmt.Sscanf(growthFlag, "%f", &growthRate); err != nil {
			klog.Warningf("Invalid growth rate format: %v, using 0.0", err)
			growthRate = 0.0
		}
	}

	klog.Infof("Running simulation for namespace: %s", namespace)
	klog.Infof("Strategy: %s, Time horizon: %d days, Growth rate: %.1f%%", strategy, days, growthRate*100)

	// For now, return an instructional error since we need the full controller integration
	return fmt.Errorf(`
Simulation requires recommendation data from the optimizer controller.

To run a simulation:
1. Ensure the optimizer controller is running and has collected metrics
2. Use the controller API to generate recommendations
3. Pass recommendations to simulator.Simulate()

Example integration:
	scenario := simulator.Scenario{
		Name:        "Production Test",
		Strategy:    simulator.Strategy%s,
		TimeHorizon: %d * 24 * time.Hour,
		Assumptions: simulator.ScenarioAssumptions{
			TrafficGrowth: %.2f,
		},
	}

	result, err := simulator.Simulate(scenario, recommendations)
	// Handle result...

For now, use 'optctl report cost' for cost analysis.
`, strategy, days, growthRate)
}

// SimulateFromRecommendations is a helper to run simulation with recommendations
func SimulateFromRecommendations(
	scenario simulator.Scenario,
	recs []recommendation.WorkloadRecommendation,
	outputFormat string,
) error {
	// Run simulation
	result, err := simulator.Simulate(scenario, recs)
	if err != nil {
		return fmt.Errorf("simulation failed: %w", err)
	}

	// Output based on format
	switch outputFormat {
	case "json":
		return outputSimulationJSON(result)
	case "text":
		return outputSimulationText(result)
	default:
		return fmt.Errorf("unsupported format: %s", outputFormat)
	}
}

func outputSimulationJSON(result *simulator.SimulationResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func outputSimulationText(result *simulator.SimulationResult) error {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                        WHAT-IF SIMULATION RESULTS                             ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Printf("Scenario:         %s\n", result.Scenario.Name)
	fmt.Printf("Strategy:         %s\n", result.Scenario.Strategy)
	fmt.Printf("Time Horizon:     %s\n", result.Scenario.TimeHorizon)
	fmt.Println()

	fmt.Println("┌─ IMPACT SUMMARY ─────────────────────────────────────────────────────────────┐")
	fmt.Printf("│  Total Workloads:     %d                                                       \n", result.TotalWorkloads)
	fmt.Printf("│  Affected Workloads:  %d                                                       \n", result.AffectedWorkloads)
	fmt.Printf("│  Confidence:          %.1f%%                                                   \n", result.Confidence)
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	fmt.Println("┌─ ESTIMATED SAVINGS ──────────────────────────────────────────────────────────┐")
	fmt.Printf("│  Daily:      $%-8.2f                                                          \n", result.EstimatedSavings.Daily)
	fmt.Printf("│  Weekly:     $%-8.2f                                                          \n", result.EstimatedSavings.Weekly)
	fmt.Printf("│  Monthly:    $%-8.2f                                                          \n", result.EstimatedSavings.Monthly)
	fmt.Printf("│  Yearly:     $%-8.2f                                                          \n", result.EstimatedSavings.Yearly)
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	fmt.Println("┌─ RISK ASSESSMENT ────────────────────────────────────────────────────────────┐")
	fmt.Printf("│  Risk Score:          %.1f%% (%s)                                              \n", result.RiskScore, result.RiskLevel)
	fmt.Printf("│  SLA Impact Prob:     %.1f%%                                                   \n", result.SLAImpactProb*100)
	fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	if len(result.Warnings) > 0 {
		fmt.Println("⚠️  WARNINGS:")
		for _, warning := range result.Warnings {
			fmt.Printf("  - %s\n", warning)
		}
		fmt.Println()
	}

	if len(result.Recommendations) > 0 {
		fmt.Println("💡 RECOMMENDATIONS:")
		for _, rec := range result.Recommendations {
			fmt.Printf("  - %s\n", rec)
		}
		fmt.Println()
	}

	if len(result.ResourceChanges) > 0 {
		fmt.Println("┌─ TOP RESOURCE CHANGES ───────────────────────────────────────────────────────┐")
		fmt.Printf("│  %-25s %-15s %-10s %-10s │\n", "WORKLOAD", "CONTAINER", "CPU", "MEMORY")
		fmt.Println("├──────────────────────────────────────────────────────────────────────────────┤")

		// Show top 10 changes by savings
		count := len(result.ResourceChanges)
		if count > 10 {
			count = 10
		}

		for i := 0; i < count; i++ {
			change := result.ResourceChanges[i]
			fmt.Printf("│  %-25s %-15s %-10s %-10s │\n",
				truncate(change.Workload, 25),
				truncate(change.Container, 15),
				change.CPUChange,
				change.MemoryChange,
			)
		}
		fmt.Println("└──────────────────────────────────────────────────────────────────────────────┘")
	}

	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
