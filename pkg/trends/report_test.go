package trends

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExportJSON(t *testing.T) {
	report := createTestReport()

	var buf bytes.Buffer
	err := report.ExportJSON(&buf)
	if err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}

	// Verify it's valid JSON
	var decoded TrendReport
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	// Verify basic fields
	if decoded.Namespace != "test-namespace" {
		t.Errorf("Namespace = %q, want %q", decoded.Namespace, "test-namespace")
	}
	if decoded.TotalWorkloads != 2 {
		t.Errorf("TotalWorkloads = %d, want 2", decoded.TotalWorkloads)
	}

	t.Logf("JSON output length: %d bytes", buf.Len())
}

func TestExportCSV(t *testing.T) {
	report := createTestReport()

	var buf bytes.Buffer
	err := report.ExportCSV(&buf)
	if err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}

	output := buf.String()
	lines := strings.Split(output, "\n")

	// Check header
	if !strings.Contains(lines[0], "Namespace") {
		t.Error("CSV should contain header with 'Namespace'")
	}
	if !strings.Contains(lines[0], "Growth Pattern") {
		t.Error("CSV should contain header with 'Growth Pattern'")
	}

	// Check that we have data rows (header + 2 workloads * 2 resources = 5 lines minimum)
	if len(lines) < 5 {
		t.Errorf("Expected at least 5 lines, got %d", len(lines))
	}

	// Verify content
	csvContent := buf.String()
	if !strings.Contains(csvContent, "test-app") {
		t.Error("CSV should contain workload name 'test-app'")
	}
	if !strings.Contains(csvContent, "CPU") {
		t.Error("CSV should contain resource type 'CPU'")
	}
	if !strings.Contains(csvContent, "Memory") {
		t.Error("CSV should contain resource type 'Memory'")
	}

	t.Logf("CSV output:\n%s", output)
}

func TestExportHTML(t *testing.T) {
	report := createTestReport()

	var buf bytes.Buffer
	err := report.ExportHTML(&buf)
	if err != nil {
		t.Fatalf("ExportHTML failed: %v", err)
	}

	output := buf.String()

	// Verify HTML structure
	requiredElements := []string{
		"<!DOCTYPE html>",
		"<html",
		"<head>",
		"<body>",
		"Historical Trend Analysis Report",
		"test-namespace",
		"Chart.js",
		"canvas",
	}

	for _, elem := range requiredElements {
		if !strings.Contains(output, elem) {
			t.Errorf("HTML should contain %q", elem)
		}
	}

	// Verify data is present
	if !strings.Contains(output, "test-app") {
		t.Error("HTML should contain workload name")
	}

	// Verify charts are included
	if !strings.Contains(output, "growthChart") {
		t.Error("HTML should contain growth chart")
	}
	if !strings.Contains(output, "riskChart") {
		t.Error("HTML should contain risk chart")
	}

	t.Logf("HTML output length: %d bytes", buf.Len())
}

func TestFormatTimeToLimit(t *testing.T) {
	tests := []struct {
		name     string
		duration *time.Duration
		expected string
	}{
		{
			name:     "nil duration",
			duration: nil,
			expected: "N/A",
		},
		{
			name:     "hours",
			duration: durationPtr(6 * time.Hour),
			expected: "6.0 hours",
		},
		{
			name:     "days",
			duration: durationPtr(5 * 24 * time.Hour),
			expected: "5.0 days",
		},
		{
			name:     "weeks",
			duration: durationPtr(14 * 24 * time.Hour),
			expected: "2.0 weeks",
		},
		{
			name:     "months",
			duration: durationPtr(60 * 24 * time.Hour),
			expected: "2.0 months",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTimeToLimit(tt.duration)
			if result != tt.expected {
				t.Errorf("formatTimeToLimit() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMaxRiskScore(t *testing.T) {
	trend := WorkloadTrend{
		CPUAnalysis: TrendAnalysis{
			CapacityStatus: CapacityStatus{
				RiskScore: 75.0,
			},
		},
		MemoryAnalysis: TrendAnalysis{
			CapacityStatus: CapacityStatus{
				RiskScore: 60.0,
			},
		},
	}

	maxScore := maxRiskScore(trend)
	if maxScore != 75.0 {
		t.Errorf("maxRiskScore() = %.2f, want 75.0", maxScore)
	}

	// Test with memory higher
	trend.MemoryAnalysis.CapacityStatus.RiskScore = 85.0
	maxScore = maxRiskScore(trend)
	if maxScore != 85.0 {
		t.Errorf("maxRiskScore() = %.2f, want 85.0", maxScore)
	}
}

func TestPrepareTemplateData(t *testing.T) {
	report := createTestReport()

	data := prepareTemplateData(report)

	// Verify required fields
	if data["Report"] == nil {
		t.Error("Template data should contain Report")
	}
	if data["SortedTrends"] == nil {
		t.Error("Template data should contain SortedTrends")
	}
	if data["ChartWorkloads"] == nil {
		t.Error("Template data should contain ChartWorkloads")
	}

	// Verify chart data arrays
	chartWorkloads, ok := data["ChartWorkloads"].([]string)
	if !ok {
		t.Fatal("ChartWorkloads should be []string")
	}
	if len(chartWorkloads) == 0 {
		t.Error("ChartWorkloads should not be empty")
	}

	chartCPUGrowth, ok := data["ChartCPUGrowth"].([]float64)
	if !ok {
		t.Fatal("ChartCPUGrowth should be []float64")
	}
	if len(chartCPUGrowth) == 0 {
		t.Error("ChartCPUGrowth should not be empty")
	}

	t.Logf("Chart workloads: %v", chartWorkloads)
	t.Logf("Chart CPU growth: %v", chartCPUGrowth)
}

// Helper function to create a test report
func createTestReport() *TrendReport {
	now := time.Now()
	ttl := 7 * 24 * time.Hour

	return &TrendReport{
		GeneratedAt:    now,
		Namespace:      "test-namespace",
		LookbackPeriod: 30 * 24 * time.Hour,
		WorkloadTrends: []WorkloadTrend{
			{
				Namespace: "test-namespace",
				Workload:  "test-app",
				CPUAnalysis: TrendAnalysis{
					GrowthPattern: PatternLinear,
					GrowthRate: GrowthRate{
						Daily:   2.0,
						Weekly:  14.0,
						Monthly: 60.0,
					},
					CapacityStatus: CapacityStatus{
						CurrentUtilization: 75.0,
						TimeToLimit:        &ttl,
						RiskLevel:          RiskHigh,
						RiskScore:          65.0,
						RecommendedAction:  ActionPlan,
						Message:            "Test message",
					},
					Confidence:  85.0,
					DataQuality: "good",
					DataPoints:  100,
				},
				MemoryAnalysis: TrendAnalysis{
					GrowthPattern: PatternStable,
					GrowthRate: GrowthRate{
						Daily:   0.5,
						Weekly:  3.5,
						Monthly: 15.0,
					},
					CapacityStatus: CapacityStatus{
						CurrentUtilization: 50.0,
						TimeToLimit:        nil,
						RiskLevel:          RiskLow,
						RiskScore:          25.0,
						RecommendedAction:  ActionMonitor,
						Message:            "Stable usage",
					},
					Confidence:  82.0,
					DataQuality: "good",
					DataPoints:  100,
				},
				AnalysisTime:   now,
				LookbackPeriod: 30 * 24 * time.Hour,
			},
			{
				Namespace: "test-namespace",
				Workload:  "critical-app",
				CPUAnalysis: TrendAnalysis{
					GrowthPattern: PatternExponential,
					GrowthRate: GrowthRate{
						Daily:   5.0,
						Weekly:  35.0,
						Monthly: 150.0,
					},
					CapacityStatus: CapacityStatus{
						CurrentUtilization: 92.0,
						TimeToLimit:        durationPtr(24 * time.Hour),
						RiskLevel:          RiskCritical,
						RiskScore:          95.0,
						RecommendedAction:  ActionImmediate,
						Message:            "Critical - approaching limit",
					},
					Confidence:  88.0,
					DataQuality: "excellent",
					DataPoints:  150,
				},
				MemoryAnalysis: TrendAnalysis{
					GrowthPattern: PatternLinear,
					GrowthRate: GrowthRate{
						Daily:   3.0,
						Weekly:  21.0,
						Monthly: 90.0,
					},
					CapacityStatus: CapacityStatus{
						CurrentUtilization: 80.0,
						TimeToLimit:        durationPtr(14 * 24 * time.Hour),
						RiskLevel:          RiskHigh,
						RiskScore:          70.0,
						RecommendedAction:  ActionPlan,
						Message:            "High risk",
					},
					Confidence:  86.0,
					DataQuality: "good",
					DataPoints:  150,
				},
				AnalysisTime:   now,
				LookbackPeriod: 30 * 24 * time.Hour,
			},
		},
		TotalWorkloads: 2,
		CriticalRisks:  1,
		HighRisks:      1,
		MediumRisks:    0,
	}
}
