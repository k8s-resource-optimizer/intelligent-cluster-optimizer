package trends

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"sort"
	"time"
)

// ExportJSON exports the trend report as JSON
func (r *TrendReport) ExportJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}

// ExportCSV exports the trend report as CSV
func (r *TrendReport) ExportCSV(w io.Writer) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	header := []string{
		"Namespace", "Workload", "Resource", "Growth Pattern", "Monthly Growth %",
		"Risk Level", "Risk Score", "Time To Limit", "Current Utilization %",
		"Confidence %", "Data Quality", "Recommended Action",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write rows for each workload (CPU and Memory)
	for _, trend := range r.WorkloadTrends {
		// CPU row
		cpuRow := []string{
			trend.Namespace,
			trend.Workload,
			"CPU",
			string(trend.CPUAnalysis.GrowthPattern),
			fmt.Sprintf("%.2f", trend.CPUAnalysis.GrowthRate.Monthly),
			string(trend.CPUAnalysis.CapacityStatus.RiskLevel),
			fmt.Sprintf("%.2f", trend.CPUAnalysis.CapacityStatus.RiskScore),
			formatTimeToLimit(trend.CPUAnalysis.CapacityStatus.TimeToLimit),
			fmt.Sprintf("%.2f", trend.CPUAnalysis.CapacityStatus.CurrentUtilization),
			fmt.Sprintf("%.2f", trend.CPUAnalysis.Confidence),
			trend.CPUAnalysis.DataQuality,
			string(trend.CPUAnalysis.CapacityStatus.RecommendedAction),
		}
		if err := writer.Write(cpuRow); err != nil {
			return fmt.Errorf("failed to write CPU row: %w", err)
		}

		// Memory row
		memRow := []string{
			trend.Namespace,
			trend.Workload,
			"Memory",
			string(trend.MemoryAnalysis.GrowthPattern),
			fmt.Sprintf("%.2f", trend.MemoryAnalysis.GrowthRate.Monthly),
			string(trend.MemoryAnalysis.CapacityStatus.RiskLevel),
			fmt.Sprintf("%.2f", trend.MemoryAnalysis.CapacityStatus.RiskScore),
			formatTimeToLimit(trend.MemoryAnalysis.CapacityStatus.TimeToLimit),
			fmt.Sprintf("%.2f", trend.MemoryAnalysis.CapacityStatus.CurrentUtilization),
			fmt.Sprintf("%.2f", trend.MemoryAnalysis.Confidence),
			trend.MemoryAnalysis.DataQuality,
			string(trend.MemoryAnalysis.CapacityStatus.RecommendedAction),
		}
		if err := writer.Write(memRow); err != nil {
			return fmt.Errorf("failed to write memory row: %w", err)
		}
	}

	return nil
}

// ExportHTML exports the trend report as interactive HTML
func (r *TrendReport) ExportHTML(w io.Writer) error {
	tmpl, err := template.New("trend_report").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Prepare data for template
	data := prepareTemplateData(r)

	return tmpl.Execute(w, data)
}

// formatTimeToLimit formats the time-to-limit duration for display
func formatTimeToLimit(ttl *time.Duration) string {
	if ttl == nil {
		return "N/A"
	}

	days := ttl.Hours() / 24
	if days < 1 {
		return fmt.Sprintf("%.1f hours", ttl.Hours())
	} else if days < 7 {
		return fmt.Sprintf("%.1f days", days)
	} else if days < 30 {
		return fmt.Sprintf("%.1f weeks", days/7)
	} else {
		return fmt.Sprintf("%.1f months", days/30)
	}
}

// prepareTemplateData prepares data for HTML template
func prepareTemplateData(report *TrendReport) map[string]interface{} {
	// Sort workload trends by highest risk
	sortedTrends := make([]WorkloadTrend, len(report.WorkloadTrends))
	copy(sortedTrends, report.WorkloadTrends)

	sort.Slice(sortedTrends, func(i, j int) bool {
		maxRiskI := maxRiskScore(sortedTrends[i])
		maxRiskJ := maxRiskScore(sortedTrends[j])
		return maxRiskI > maxRiskJ
	})

	// Extract capacity warnings (high and critical risks)
	var warnings []map[string]interface{}
	for _, trend := range sortedTrends {
		if trend.CPUAnalysis.CapacityStatus.RiskLevel == RiskCritical ||
			trend.CPUAnalysis.CapacityStatus.RiskLevel == RiskHigh {
			warnings = append(warnings, map[string]interface{}{
				"workload": trend.Workload,
				"resource": "CPU",
				"risk":     trend.CPUAnalysis.CapacityStatus.RiskLevel,
				"message":  trend.CPUAnalysis.CapacityStatus.Message,
			})
		}

		if trend.MemoryAnalysis.CapacityStatus.RiskLevel == RiskCritical ||
			trend.MemoryAnalysis.CapacityStatus.RiskLevel == RiskHigh {
			warnings = append(warnings, map[string]interface{}{
				"workload": trend.Workload,
				"resource": "Memory",
				"risk":     trend.MemoryAnalysis.CapacityStatus.RiskLevel,
				"message":  trend.MemoryAnalysis.CapacityStatus.Message,
			})
		}
	}

	// Prepare chart data for top workloads
	var chartWorkloads []string
	var chartCPUGrowth []float64
	var chartMemGrowth []float64
	var chartCPURisk []float64
	var chartMemRisk []float64

	limit := 10
	if len(sortedTrends) < limit {
		limit = len(sortedTrends)
	}

	for i := 0; i < limit; i++ {
		chartWorkloads = append(chartWorkloads, sortedTrends[i].Workload)
		chartCPUGrowth = append(chartCPUGrowth, sortedTrends[i].CPUAnalysis.GrowthRate.Monthly)
		chartMemGrowth = append(chartMemGrowth, sortedTrends[i].MemoryAnalysis.GrowthRate.Monthly)
		chartCPURisk = append(chartCPURisk, sortedTrends[i].CPUAnalysis.CapacityStatus.RiskScore)
		chartMemRisk = append(chartMemRisk, sortedTrends[i].MemoryAnalysis.CapacityStatus.RiskScore)
	}

	// JSON-encode chart slices and wrap in template.JS so html/template injects
	// them as valid JavaScript arrays inside <script> blocks. Without this, Go
	// would render []string{"a","b"} as "[a b]" — invalid JS that silently
	// breaks the charts.
	workloadsJSON, _ := json.Marshal(chartWorkloads)
	cpuGrowthJSON, _ := json.Marshal(chartCPUGrowth)
	memGrowthJSON, _ := json.Marshal(chartMemGrowth)
	cpuRiskJSON, _ := json.Marshal(chartCPURisk)
	memRiskJSON, _ := json.Marshal(chartMemRisk)

	return map[string]interface{}{
		"Report":       report,
		"SortedTrends": sortedTrends,
		"Warnings":     warnings,
		// #nosec G203 -- values are JSON-encoded internal data (float64 scores and
		// Kubernetes workload names), not user-controlled input. template.JS is
		// required to inject valid JS arrays into <script> blocks.
		"ChartWorkloads": template.JS(workloadsJSON), // #nosec G203
		"ChartCPUGrowth": template.JS(cpuGrowthJSON), // #nosec G203
		"ChartMemGrowth": template.JS(memGrowthJSON), // #nosec G203
		"ChartCPURisk":   template.JS(cpuRiskJSON),   // #nosec G203
		"ChartMemRisk":   template.JS(memRiskJSON),   // #nosec G203
		"HasWarnings":    len(warnings) > 0,
	}
}

// maxRiskScore returns the maximum risk score for a workload
func maxRiskScore(trend WorkloadTrend) float64 {
	cpuRisk := trend.CPUAnalysis.CapacityStatus.RiskScore
	memRisk := trend.MemoryAnalysis.CapacityStatus.RiskScore
	if cpuRisk > memRisk {
		return cpuRisk
	}
	return memRisk
}

// HTML template for trend report
const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Trend Analysis Report - {{.Report.Namespace}}</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            padding: 20px;
            color: #333;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
            background: white;
            border-radius: 12px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
            overflow: hidden;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 40px;
            text-align: center;
        }
        .header h1 {
            font-size: 2.5em;
            margin-bottom: 10px;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.2);
        }
        .header .subtitle {
            font-size: 1.2em;
            opacity: 0.9;
        }
        .summary {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            padding: 40px;
            background: #f8f9fa;
        }
        .summary-card {
            background: white;
            padding: 25px;
            border-radius: 10px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
            text-align: center;
            transition: transform 0.2s;
        }
        .summary-card:hover {
            transform: translateY(-5px);
            box-shadow: 0 4px 12px rgba(0,0,0,0.15);
        }
        .summary-card .value {
            font-size: 2.5em;
            font-weight: bold;
            margin: 10px 0;
        }
        .summary-card .label {
            color: #666;
            font-size: 0.9em;
            text-transform: uppercase;
            letter-spacing: 1px;
        }
        .critical { color: #dc3545; }
        .high { color: #fd7e14; }
        .medium { color: #ffc107; }
        .low { color: #28a745; }
        .none { color: #6c757d; }

        .warnings {
            padding: 40px;
            background: #fff3cd;
            border-top: 4px solid #ffc107;
        }
        .warnings h2 {
            color: #856404;
            margin-bottom: 20px;
            font-size: 1.8em;
        }
        .warning-item {
            background: white;
            padding: 20px;
            margin-bottom: 15px;
            border-radius: 8px;
            border-left: 5px solid;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .warning-item.critical { border-left-color: #dc3545; }
        .warning-item.high { border-left-color: #fd7e14; }

        .charts {
            padding: 40px;
            background: white;
        }
        .charts h2 {
            margin-bottom: 30px;
            font-size: 1.8em;
            color: #333;
        }
        .chart-container {
            position: relative;
            margin-bottom: 40px;
            height: 400px;
        }

        .workloads {
            padding: 40px;
            background: #f8f9fa;
        }
        .workloads h2 {
            margin-bottom: 30px;
            font-size: 1.8em;
            color: #333;
        }
        .workload-card {
            background: white;
            padding: 30px;
            margin-bottom: 20px;
            border-radius: 10px;
            box-shadow: 0 2px 8px rgba(0,0,0,0.1);
        }
        .workload-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
            padding-bottom: 15px;
            border-bottom: 2px solid #e9ecef;
        }
        .workload-name {
            font-size: 1.5em;
            font-weight: bold;
            color: #333;
        }
        .resource-grid {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 30px;
        }
        .resource-section {
            padding: 20px;
            background: #f8f9fa;
            border-radius: 8px;
        }
        .resource-section h4 {
            margin-bottom: 15px;
            color: #667eea;
            font-size: 1.2em;
        }
        .metric-row {
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            border-bottom: 1px solid #e9ecef;
        }
        .metric-label {
            color: #666;
            font-size: 0.9em;
        }
        .metric-value {
            font-weight: 600;
            color: #333;
        }
        .badge {
            display: inline-block;
            padding: 5px 12px;
            border-radius: 20px;
            font-size: 0.85em;
            font-weight: 600;
            text-transform: uppercase;
        }
        .badge.critical { background: #dc3545; color: white; }
        .badge.high { background: #fd7e14; color: white; }
        .badge.medium { background: #ffc107; color: #333; }
        .badge.low { background: #28a745; color: white; }
        .badge.none { background: #6c757d; color: white; }

        .footer {
            padding: 30px 40px;
            background: #343a40;
            color: white;
            text-align: center;
        }
        .footer p {
            margin: 5px 0;
            opacity: 0.8;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>📈 Historical Trend Analysis Report</h1>
            <div class="subtitle">Namespace: {{.Report.Namespace}}</div>
            <div class="subtitle">Generated: {{.Report.GeneratedAt.Format "2006-01-02 15:04:05"}}</div>
        </div>

        <div class="summary">
            <div class="summary-card">
                <div class="label">Total Workloads</div>
                <div class="value">{{.Report.TotalWorkloads}}</div>
            </div>
            <div class="summary-card">
                <div class="label">Critical Risks</div>
                <div class="value critical">{{.Report.CriticalRisks}}</div>
            </div>
            <div class="summary-card">
                <div class="label">High Risks</div>
                <div class="value high">{{.Report.HighRisks}}</div>
            </div>
            <div class="summary-card">
                <div class="label">Medium Risks</div>
                <div class="value medium">{{.Report.MediumRisks}}</div>
            </div>
        </div>

        {{if .HasWarnings}}
        <div class="warnings">
            <h2>⚠️ Capacity Warnings</h2>
            {{range .Warnings}}
            <div class="warning-item {{.risk}}">
                <strong>{{.workload}}</strong> - {{.resource}}
                <br>
                <span class="badge {{.risk}}">{{.risk}}</span>
                {{.message}}
            </div>
            {{end}}
        </div>
        {{end}}

        <div class="charts">
            <h2>Growth Rate Analysis</h2>
            <div class="chart-container">
                <canvas id="growthChart"></canvas>
            </div>

            <h2>Capacity Risk Analysis</h2>
            <div class="chart-container">
                <canvas id="riskChart"></canvas>
            </div>
        </div>

        <div class="workloads">
            <h2>Workload Details</h2>
            {{range .SortedTrends}}
            <div class="workload-card">
                <div class="workload-header">
                    <div class="workload-name">{{.Workload}}</div>
                </div>
                <div class="resource-grid">
                    <div class="resource-section">
                        <h4>💻 CPU Analysis</h4>
                        <div class="metric-row">
                            <span class="metric-label">Growth Pattern:</span>
                            <span class="metric-value">{{.CPUAnalysis.GrowthPattern}}</span>
                        </div>
                        <div class="metric-row">
                            <span class="metric-label">Monthly Growth:</span>
                            <span class="metric-value">{{printf "%.2f" .CPUAnalysis.GrowthRate.Monthly}}%</span>
                        </div>
                        <div class="metric-row">
                            <span class="metric-label">Risk Level:</span>
                            <span class="badge {{.CPUAnalysis.CapacityStatus.RiskLevel}}">{{.CPUAnalysis.CapacityStatus.RiskLevel}}</span>
                        </div>
                        <div class="metric-row">
                            <span class="metric-label">Current Utilization:</span>
                            <span class="metric-value">{{printf "%.1f" .CPUAnalysis.CapacityStatus.CurrentUtilization}}%</span>
                        </div>
                        <div class="metric-row">
                            <span class="metric-label">Confidence:</span>
                            <span class="metric-value">{{printf "%.1f" .CPUAnalysis.Confidence}}% ({{.CPUAnalysis.DataQuality}})</span>
                        </div>
                    </div>

                    <div class="resource-section">
                        <h4>💾 Memory Analysis</h4>
                        <div class="metric-row">
                            <span class="metric-label">Growth Pattern:</span>
                            <span class="metric-value">{{.MemoryAnalysis.GrowthPattern}}</span>
                        </div>
                        <div class="metric-row">
                            <span class="metric-label">Monthly Growth:</span>
                            <span class="metric-value">{{printf "%.2f" .MemoryAnalysis.GrowthRate.Monthly}}%</span>
                        </div>
                        <div class="metric-row">
                            <span class="metric-label">Risk Level:</span>
                            <span class="badge {{.MemoryAnalysis.CapacityStatus.RiskLevel}}">{{.MemoryAnalysis.CapacityStatus.RiskLevel}}</span>
                        </div>
                        <div class="metric-row">
                            <span class="metric-label">Current Utilization:</span>
                            <span class="metric-value">{{printf "%.1f" .MemoryAnalysis.CapacityStatus.CurrentUtilization}}%</span>
                        </div>
                        <div class="metric-row">
                            <span class="metric-label">Confidence:</span>
                            <span class="metric-value">{{printf "%.1f" .MemoryAnalysis.Confidence}}% ({{.MemoryAnalysis.DataQuality}})</span>
                        </div>
                    </div>
                </div>
            </div>
            {{end}}
        </div>

        <div class="footer">
            <p><strong>Intelligent Cluster Optimizer</strong></p>
            <p>Historical Trend Analysis with Capacity Planning</p>
            <p>Lookback Period: {{.Report.LookbackPeriod}}</p>
            <p>Charts powered by Chart.js</p>
        </div>
    </div>

    <script>
        // Growth Rate Chart
        const growthCtx = document.getElementById('growthChart').getContext('2d');
        new Chart(growthCtx, {
            type: 'bar',
            data: {
                labels: {{.ChartWorkloads}},
                datasets: [
                    {
                        label: 'CPU Growth (%/month)',
                        data: {{.ChartCPUGrowth}},
                        backgroundColor: 'rgba(102, 126, 234, 0.7)',
                        borderColor: 'rgba(102, 126, 234, 1)',
                        borderWidth: 2
                    },
                    {
                        label: 'Memory Growth (%/month)',
                        data: {{.ChartMemGrowth}},
                        backgroundColor: 'rgba(118, 75, 162, 0.7)',
                        borderColor: 'rgba(118, 75, 162, 1)',
                        borderWidth: 2
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                scales: {
                    y: {
                        beginAtZero: true,
                        title: {
                            display: true,
                            text: 'Growth Rate (%/month)'
                        }
                    }
                },
                plugins: {
                    title: {
                        display: true,
                        text: 'Top 10 Workloads by Growth Rate',
                        font: { size: 16 }
                    },
                    legend: {
                        display: true,
                        position: 'top'
                    }
                }
            }
        });

        // Risk Score Chart
        const riskCtx = document.getElementById('riskChart').getContext('2d');
        new Chart(riskCtx, {
            type: 'bar',
            data: {
                labels: {{.ChartWorkloads}},
                datasets: [
                    {
                        label: 'CPU Risk Score',
                        data: {{.ChartCPURisk}},
                        backgroundColor: 'rgba(220, 53, 69, 0.7)',
                        borderColor: 'rgba(220, 53, 69, 1)',
                        borderWidth: 2
                    },
                    {
                        label: 'Memory Risk Score',
                        data: {{.ChartMemRisk}},
                        backgroundColor: 'rgba(253, 126, 20, 0.7)',
                        borderColor: 'rgba(253, 126, 20, 1)',
                        borderWidth: 2
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                scales: {
                    y: {
                        beginAtZero: true,
                        max: 100,
                        title: {
                            display: true,
                            text: 'Risk Score (0-100)'
                        }
                    }
                },
                plugins: {
                    title: {
                        display: true,
                        text: 'Top 10 Workloads by Capacity Risk',
                        font: { size: 16 }
                    },
                    legend: {
                        display: true,
                        position: 'top'
                    }
                }
            }
        });
    </script>
</body>
</html>`
