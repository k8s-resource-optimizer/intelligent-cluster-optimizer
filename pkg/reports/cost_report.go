package reports

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"time"

	"intelligent-cluster-optimizer/pkg/recommendation"
)

// CostReport represents a comprehensive cost analysis report
type CostReport struct {
	ClusterName      string          `json:"cluster_name"`
	GeneratedAt      time.Time       `json:"generated_at"`
	TotalMonthlyCost float64         `json:"total_monthly_cost"`
	PotentialSavings float64         `json:"potential_savings"`
	WastePercentage  float64         `json:"waste_percentage"`
	TopWasters       []WorkloadWaste `json:"top_wasters"`
	Summary          ReportSummary   `json:"summary"`
}

// WorkloadWaste represents a single workload's cost waste analysis
type WorkloadWaste struct {
	Namespace     string  `json:"namespace"`
	WorkloadType  string  `json:"workload_type"`
	WorkloadName  string  `json:"workload_name"`
	ContainerName string  `json:"container_name"`
	CurrentCost   float64 `json:"current_cost"`
	OptimizedCost float64 `json:"optimized_cost"`
	Savings       float64 `json:"savings"`
	WasteReason   string  `json:"waste_reason"`
	Confidence    float64 `json:"confidence"`
}

// ReportSummary provides high-level statistics
type ReportSummary struct {
	TotalWorkloads        int     `json:"total_workloads"`
	OverProvisioned       int     `json:"over_provisioned"`
	UnderProvisioned      int     `json:"under_provisioned"`
	OptimallyProvisioned  int     `json:"optimally_provisioned"`
	AverageSavingsPercent float64 `json:"average_savings_percent"`
}

// GenerateCostReport creates a comprehensive cost report from workload recommendations
func GenerateCostReport(clusterName string, recommendations []recommendation.WorkloadRecommendation) *CostReport {
	report := &CostReport{
		ClusterName: clusterName,
		GeneratedAt: time.Now(),
		TopWasters:  []WorkloadWaste{},
	}

	var allWasters []WorkloadWaste
	totalCurrentCost := 0.0
	totalOptimizedCost := 0.0

	// Process each workload recommendation
	for _, rec := range recommendations {
		// Process each container in the workload
		for _, container := range rec.Containers {
			// Calculate current and optimized costs (monthly)
			currentCost := 0.0
			optimizedCost := 0.0

			if container.EstimatedSavings != nil {
				currentCost = container.EstimatedSavings.CurrentCost.TotalPerMonth
				optimizedCost = container.EstimatedSavings.RecommendedCost.TotalPerMonth
			}

			// Calculate savings
			savings := currentCost - optimizedCost

			// Determine waste reason
			wasteReason := determineWasteReason(
				container.CurrentCPU,
				container.CurrentMemory,
				container.RecommendedCPU,
				container.RecommendedMemory,
				container.HasOOMHistory,
			)

			// Create WorkloadWaste entry
			waster := WorkloadWaste{
				Namespace:     rec.Namespace,
				WorkloadType:  rec.WorkloadKind,
				WorkloadName:  rec.WorkloadName,
				ContainerName: container.ContainerName,
				CurrentCost:   currentCost,
				OptimizedCost: optimizedCost,
				Savings:       savings,
				WasteReason:   wasteReason,
				Confidence:    container.Confidence,
			}

			// Only include if there are meaningful savings or issues
			if waster.Savings > 0.01 || waster.WasteReason != "optimal" {
				allWasters = append(allWasters, waster)
			}

			totalCurrentCost += currentCost
			totalOptimizedCost += optimizedCost
		}
	}

	// Sort by savings (highest first) and take top 20
	sort.Slice(allWasters, func(i, j int) bool {
		return allWasters[i].Savings > allWasters[j].Savings
	})

	if len(allWasters) > 20 {
		report.TopWasters = allWasters[:20]
	} else {
		report.TopWasters = allWasters
	}

	// Calculate totals
	report.TotalMonthlyCost = totalCurrentCost
	report.PotentialSavings = totalCurrentCost - totalOptimizedCost
	if totalCurrentCost > 0 {
		report.WastePercentage = (report.PotentialSavings / totalCurrentCost) * 100
	}

	// Generate summary
	report.Summary = generateSummary(allWasters)

	return report
}

// ExportJSON exports the report as JSON
func (r *CostReport) ExportJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

// ExportCSV exports the report as CSV
func (r *CostReport) ExportCSV(w io.Writer) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	header := []string{"Namespace", "Workload", "Container", "Current Cost", "Optimized Cost", "Savings", "Waste Reason", "Confidence"}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write rows
	for _, waster := range r.TopWasters {
		row := []string{
			waster.Namespace,
			fmt.Sprintf("%s/%s", waster.WorkloadType, waster.WorkloadName),
			waster.ContainerName,
			fmt.Sprintf("$%.2f", waster.CurrentCost),
			fmt.Sprintf("$%.2f", waster.OptimizedCost),
			fmt.Sprintf("$%.2f", waster.Savings),
			waster.WasteReason,
			fmt.Sprintf("%.1f%%", waster.Confidence*100),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	return nil
}

// ExportHTML exports the report as HTML
func (r *CostReport) ExportHTML(w io.Writer) error {
	tmpl := `
<!DOCTYPE html>
<html>
<head>
    <title>Cost Optimization Report - {{.ClusterName}}</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .header { background: #2c3e50; color: white; padding: 20px; border-radius: 8px; }
        .summary { background: white; padding: 20px; margin: 20px 0; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 20px; margin: 20px 0; }
        .stat-card { background: white; padding: 15px; border-radius: 8px; text-align: center; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .stat-value { font-size: 32px; font-weight: bold; color: #27ae60; }
        .stat-label { color: #7f8c8d; margin-top: 5px; }
        table { width: 100%; border-collapse: collapse; background: white; border-radius: 8px; overflow: hidden; }
        th { background: #34495e; color: white; padding: 12px; text-align: left; }
        td { padding: 12px; border-bottom: 1px solid #ecf0f1; }
        tr:hover { background: #f8f9fa; }
        .savings { color: #27ae60; font-weight: bold; }
        .waste-high { color: #e74c3c; }
        .waste-medium { color: #f39c12; }
    </style>
</head>
<body>
    <div class="header">
        <h1>💰 Cost Optimization Report</h1>
        <p>Cluster: {{.ClusterName}} | Generated: {{.GeneratedAt.Format "2006-01-02 15:04:05"}}</p>
    </div>

    <div class="stats">
        <div class="stat-card">
            <div class="stat-value">${{printf "%.2f" .TotalMonthlyCost}}</div>
            <div class="stat-label">Current Monthly Cost</div>
        </div>
        <div class="stat-card">
            <div class="stat-value savings">${{printf "%.2f" .PotentialSavings}}</div>
            <div class="stat-label">Potential Savings</div>
        </div>
        <div class="stat-card">
            <div class="stat-value">{{printf "%.1f" .WastePercentage}}%</div>
            <div class="stat-label">Waste Percentage</div>
        </div>
    </div>

    <div class="summary">
        <h2>Summary</h2>
        <p>Total Workloads: {{.Summary.TotalWorkloads}}</p>
        <p>Over-Provisioned: {{.Summary.OverProvisioned}} | Under-Provisioned: {{.Summary.UnderProvisioned}} | Optimal: {{.Summary.OptimallyProvisioned}}</p>
    </div>

    <table>
        <thead>
            <tr>
                <th>Namespace</th>
                <th>Workload</th>
                <th>Container</th>
                <th>Current Cost</th>
                <th>Optimized Cost</th>
                <th>Savings</th>
                <th>Waste Reason</th>
                <th>Confidence</th>
            </tr>
        </thead>
        <tbody>
            {{range .TopWasters}}
            <tr>
                <td>{{.Namespace}}</td>
                <td>{{.WorkloadType}}/{{.WorkloadName}}</td>
                <td>{{.ContainerName}}</td>
                <td>${{printf "%.2f" .CurrentCost}}</td>
                <td>${{printf "%.2f" .OptimizedCost}}</td>
                <td class="savings">${{printf "%.2f" .Savings}}</td>
                <td>{{.WasteReason}}</td>
                <td>{{printf "%.0f" (mul .Confidence 100)}}%</td>
            </tr>
            {{end}}
        </tbody>
    </table>
</body>
</html>
`

	t, err := template.New("report").Funcs(template.FuncMap{
		"mul": func(a, b float64) float64 { return a * b },
	}).Parse(tmpl)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	return t.Execute(w, r)
}

// Helper function to sort wasters by savings
func (r *CostReport) sortByPotentialSavings() {
	sort.Slice(r.TopWasters, func(i, j int) bool {
		return r.TopWasters[i].Savings > r.TopWasters[j].Savings
	})
}

// determineWasteReason analyzes why resources are wasted
func determineWasteReason(currentCPU, currentMemory, optimizedCPU, optimizedMemory int64, hasOOMHistory bool) string {
	// Check for OOM issues first (critical)
	if hasOOMHistory {
		return "oom-risk"
	}

	// Avoid division by zero
	if currentCPU == 0 || currentMemory == 0 {
		return "unknown"
	}

	// Calculate reduction percentages
	cpuReduction := float64(currentCPU-optimizedCPU) / float64(currentCPU) * 100
	memoryReduction := float64(currentMemory-optimizedMemory) / float64(currentMemory) * 100

	// Check if resources are significantly over-provisioned
	if cpuReduction > 30 || memoryReduction > 30 {
		return "over-provisioned"
	}

	// Check if moderately over-provisioned
	if cpuReduction > 15 || memoryReduction > 15 {
		return "waste-detected"
	}

	// Check if under-provisioned (should increase)
	if cpuReduction < -10 || memoryReduction < -10 {
		return "under-provisioned"
	}

	// Check if nearly optimal
	if cpuReduction > -5 && cpuReduction < 5 && memoryReduction > -5 && memoryReduction < 5 {
		return "optimal"
	}

	return "minor-adjustment"
}

// generateSummary creates summary statistics from wasters
func generateSummary(wasters []WorkloadWaste) ReportSummary {
	summary := ReportSummary{
		TotalWorkloads: len(wasters),
	}

	var totalSavingsPercent float64
	for _, w := range wasters {
		switch w.WasteReason {
		case "over-provisioned":
			summary.OverProvisioned++
		case "under-provisioned":
			summary.UnderProvisioned++
		case "optimal":
			summary.OptimallyProvisioned++
		}

		// Calculate savings percentage
		if w.CurrentCost > 0 {
			savingsPercent := (w.Savings / w.CurrentCost) * 100
			totalSavingsPercent += savingsPercent
		}
	}

	// Calculate average
	if len(wasters) > 0 {
		summary.AverageSavingsPercent = totalSavingsPercent / float64(len(wasters))
	}

	return summary
}
