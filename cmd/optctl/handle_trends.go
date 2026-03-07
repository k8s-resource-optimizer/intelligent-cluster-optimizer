package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"intelligent-cluster-optimizer/pkg/storage"
	"intelligent-cluster-optimizer/pkg/trends"

	"k8s.io/klog/v2"
)

// handleTrends processes trend analysis commands
func handleTrends(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: optctl trends <namespace> [workload]\n  Flags: --format=json|csv|html, --days=N (lookback days, default 30), --metrics-file=path")
	}

	namespace := args[0]
	workload := ""
	if len(args) > 1 {
		workload = args[1]
	}

	// Determine lookback period
	lookbackDays := 30
	if daysFlag > 0 {
		lookbackDays = daysFlag
	}
	lookback := time.Duration(lookbackDays) * 24 * time.Hour

	// Load metrics from storage file
	store := storage.NewStorage()
	if err := store.LoadFromFile(metricsFile); err != nil {
		return fmt.Errorf("failed to load metrics from %s: %w\n\nEnsure the optimizer controller is running and saving metrics.", metricsFile, err)
	}

	count := store.GetMetricCount()
	if count == 0 {
		return fmt.Errorf("no metrics found in %s\n\nTo collect metrics:\n  1. Run the optimizer controller (it saves metrics automatically)\n  2. Or specify a different file with --metrics-file=path\n  3. Default metrics file: %s", metricsFile, metricsFile)
	}

	klog.Infof("Loaded %d metric entries from %s (lookback: %d days)", count, metricsFile, lookbackDays)

	if workload != "" {
		return GenerateWorkloadTrendFromStorage(store, namespace, workload, lookback, format)
	}
	return GenerateTrendReportFromStorage(store, namespace, lookback, format)
}

// GenerateTrendReportFromStorage generates and exports a trend report for a namespace
func GenerateTrendReportFromStorage(store *storage.InMemoryStorage, namespace string, lookback time.Duration, outputFormat string) error {
	config := trends.DefaultAnalyzerConfig()
	analyzer := trends.NewTrendAnalyzer(store, config)

	klog.Infof("Analyzing trends for namespace %s (lookback: %v)", namespace, lookback)

	report, err := analyzer.AnalyzeNamespace(namespace, lookback)
	if err != nil {
		return fmt.Errorf("failed to analyze namespace: %w", err)
	}

	klog.Infof("Analysis complete: %d workloads, %d critical risks, %d high risks",
		report.TotalWorkloads, report.CriticalRisks, report.HighRisks)

	switch outputFormat {
	case "json":
		return report.ExportJSON(os.Stdout)
	case "csv":
		return report.ExportCSV(os.Stdout)
	case "html":
		return report.ExportHTML(os.Stdout)
	case "text", "":
		return printTrendReportText(report)
	default:
		return fmt.Errorf("unsupported format: %s (use json, csv, html, or text)", outputFormat)
	}
}

// GenerateWorkloadTrendFromStorage generates trend analysis for a single workload
func GenerateWorkloadTrendFromStorage(store *storage.InMemoryStorage, namespace, workload string, lookback time.Duration, outputFormat string) error {
	config := trends.DefaultAnalyzerConfig()
	analyzer := trends.NewTrendAnalyzer(store, config)

	klog.Infof("Analyzing trend for workload %s/%s (lookback: %v)", namespace, workload, lookback)

	trend, err := analyzer.AnalyzeWorkload(namespace, workload, lookback)
	if err != nil {
		return fmt.Errorf("failed to analyze workload: %w", err)
	}

	switch outputFormat {
	case "json":
		report := &trends.TrendReport{
			GeneratedAt:    time.Now(),
			Namespace:      namespace,
			LookbackPeriod: lookback,
			WorkloadTrends: []trends.WorkloadTrend{*trend},
			TotalWorkloads: 1,
		}
		countRisks(report, trend)
		return report.ExportJSON(os.Stdout)
	case "csv":
		report := &trends.TrendReport{WorkloadTrends: []trends.WorkloadTrend{*trend}}
		return report.ExportCSV(os.Stdout)
	case "html":
		return fmt.Errorf("HTML format is not supported for single workload; use namespace-level report instead")
	case "text", "":
		return printWorkloadTrendText(trend)
	default:
		return fmt.Errorf("unsupported format: %s (use json, csv, or text)", outputFormat)
	}
}

// countRisks tallies risk levels from a workload trend into the report
func countRisks(report *trends.TrendReport, trend *trends.WorkloadTrend) {
	if trend.CPUAnalysis.CapacityStatus.RiskLevel == trends.RiskCritical ||
		trend.MemoryAnalysis.CapacityStatus.RiskLevel == trends.RiskCritical {
		report.CriticalRisks = 1
	} else if trend.CPUAnalysis.CapacityStatus.RiskLevel == trends.RiskHigh ||
		trend.MemoryAnalysis.CapacityStatus.RiskLevel == trends.RiskHigh {
		report.HighRisks = 1
	} else if trend.CPUAnalysis.CapacityStatus.RiskLevel == trends.RiskMedium ||
		trend.MemoryAnalysis.CapacityStatus.RiskLevel == trends.RiskMedium {
		report.MediumRisks = 1
	}
}

// printTrendReportText prints a human-readable trend report to stdout
func printTrendReportText(report *trends.TrendReport) error {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    HISTORICAL TREND ANALYSIS REPORT                          ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════════╝")
	fmt.Printf("  Namespace:  %s\n", report.Namespace)
	fmt.Printf("  Generated:  %s\n", report.GeneratedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Lookback:   %s\n", formatLookback(report.LookbackPeriod))
	fmt.Println()

	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  SUMMARY                                                                    │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│  Total Workloads: %-4d  Critical: %-4d  High: %-4d  Medium: %-4d            │\n",
		report.TotalWorkloads, report.CriticalRisks, report.HighRisks, report.MediumRisks)
	if report.FastestGrowing != nil {
		fmt.Printf("│  Fastest Growing: %-55s │\n", report.FastestGrowing.Workload)
	}
	if report.HighestCapacityRisk != nil {
		fmt.Printf("│  Highest Risk:    %-55s │\n", report.HighestCapacityRisk.Workload)
	}
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	if report.TotalWorkloads == 0 {
		fmt.Println("  No workloads with sufficient data found.")
		return nil
	}

	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  WORKLOAD TRENDS                                                            │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  WORKLOAD\tRESOURCE\tPATTERN\tGROWTH/MO\tRISK\tUTIL%\tCONFIDENCE")
	fmt.Fprintln(w, "  "+strings.Repeat("-", 73))

	for _, t := range report.WorkloadTrends {
		// CPU row
		ttl := formatTTL(t.CPUAnalysis.CapacityStatus.TimeToLimit)
		fmt.Fprintf(w, "  %s\tCPU\t%s\t%+.1f%%\t%s\t%.0f%%\t%.0f%% (%s) %s\n",
			t.Workload,
			t.CPUAnalysis.GrowthPattern,
			t.CPUAnalysis.GrowthRate.Monthly,
			riskLabel(t.CPUAnalysis.CapacityStatus.RiskLevel),
			t.CPUAnalysis.CapacityStatus.CurrentUtilization,
			t.CPUAnalysis.Confidence,
			t.CPUAnalysis.DataQuality,
			ttl,
		)
		// Memory row
		ttl = formatTTL(t.MemoryAnalysis.CapacityStatus.TimeToLimit)
		fmt.Fprintf(w, "  %s\tMemory\t%s\t%+.1f%%\t%s\t%.0f%%\t%.0f%% (%s) %s\n",
			t.Workload,
			t.MemoryAnalysis.GrowthPattern,
			t.MemoryAnalysis.GrowthRate.Monthly,
			riskLabel(t.MemoryAnalysis.CapacityStatus.RiskLevel),
			t.MemoryAnalysis.CapacityStatus.CurrentUtilization,
			t.MemoryAnalysis.Confidence,
			t.MemoryAnalysis.DataQuality,
			ttl,
		)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("  Tip: Use --format=html to generate an interactive chart report.")
	fmt.Println("       Use --format=csv to export data for spreadsheets.")
	return nil
}

// printWorkloadTrendText prints a human-readable single workload trend
func printWorkloadTrendText(t *trends.WorkloadTrend) error {
	fmt.Println()
	fmt.Printf("╔══════════════════════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║            TREND ANALYSIS: %-49s║\n", t.Namespace+"/"+t.Workload+" ")
	fmt.Printf("╚══════════════════════════════════════════════════════════════════════════════╝\n")
	fmt.Printf("  Analysis Time: %s\n", t.AnalysisTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Lookback:      %s\n\n", formatLookback(t.LookbackPeriod))

	printResourceAnalysis("CPU", &t.CPUAnalysis)
	fmt.Println()
	printResourceAnalysis("Memory", &t.MemoryAnalysis)
	fmt.Println()
	return nil
}

func printResourceAnalysis(name string, a *trends.TrendAnalysis) {
	fmt.Printf("  ┌─ %s ANALYSIS %s\n", name, strings.Repeat("─", 62-len(name)))
	fmt.Printf("  │  Growth Pattern:    %s\n", a.GrowthPattern)
	fmt.Printf("  │  Growth Rate:       Daily %+.2f%%  Weekly %+.2f%%  Monthly %+.2f%%  CAGR %+.2f%%\n",
		a.GrowthRate.Daily, a.GrowthRate.Weekly, a.GrowthRate.Monthly, a.GrowthRate.CAGR)
	fmt.Printf("  │  Acceleration:      %.4f\n", a.Acceleration)
	fmt.Printf("  │  Trend Strength:    %.2f\n", a.TrendStrength)
	fmt.Printf("  │  Data Points:       %d (%s quality, %.0f%% confidence)\n",
		a.DataPoints, a.DataQuality, a.Confidence)
	fmt.Printf("  ├─ Capacity Status\n")
	fmt.Printf("  │  Current Util:      %.1f%%\n", a.CapacityStatus.CurrentUtilization)
	fmt.Printf("  │  Risk Level:        %s (score: %.1f)\n",
		riskLabel(a.CapacityStatus.RiskLevel), a.CapacityStatus.RiskScore)
	fmt.Printf("  │  Recommended:       %s\n", a.CapacityStatus.RecommendedAction)
	if a.CapacityStatus.TimeToLimit != nil {
		fmt.Printf("  │  Time to Limit:     %s\n", formatDuration(*a.CapacityStatus.TimeToLimit))
	}
	fmt.Printf("  │  Message:           %s\n", a.CapacityStatus.Message)
	fmt.Printf("  ├─ Forecasts\n")
	fmt.Printf("  │  7-day:   %.2f  (%.2f – %.2f)\n",
		a.ShortTerm.Value, a.ShortTerm.LowerBound, a.ShortTerm.UpperBound)
	fmt.Printf("  │  30-day:  %.2f  (%.2f – %.2f)\n",
		a.MidTerm.Value, a.MidTerm.LowerBound, a.MidTerm.UpperBound)
	fmt.Printf("  │  90-day:  %.2f  (%.2f – %.2f)\n",
		a.LongTerm.Value, a.LongTerm.LowerBound, a.LongTerm.UpperBound)
	fmt.Printf("  └%s\n", strings.Repeat("─", 74))
}

// riskLabel returns a display string for a risk level
func riskLabel(r trends.RiskLevel) string {
	switch r {
	case trends.RiskCritical:
		return "CRITICAL"
	case trends.RiskHigh:
		return "HIGH    "
	case trends.RiskMedium:
		return "MEDIUM  "
	case trends.RiskLow:
		return "LOW     "
	default:
		return "NONE    "
	}
}

// formatTTL formats time-to-limit as a short string for table display
func formatTTL(ttl *time.Duration) string {
	if ttl == nil {
		return ""
	}
	days := ttl.Hours() / 24
	if days < 1 {
		return fmt.Sprintf("(%.0fh to limit)", ttl.Hours())
	} else if days < 30 {
		return fmt.Sprintf("(%.0fd to limit)", days)
	}
	return fmt.Sprintf("(%.0fmo to limit)", days/30)
}

// formatDuration formats a duration for human display
func formatDuration(d time.Duration) string {
	days := d.Hours() / 24
	if days < 1 {
		return fmt.Sprintf("%.1f hours", d.Hours())
	} else if days < 7 {
		return fmt.Sprintf("%.1f days", days)
	} else if days < 30 {
		return fmt.Sprintf("%.1f weeks", days/7)
	}
	return fmt.Sprintf("%.1f months", days/30)
}

// formatLookback formats a duration as a human-friendly lookback string
func formatLookback(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}
