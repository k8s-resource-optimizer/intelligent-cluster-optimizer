package reports

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/cost"
	"intelligent-cluster-optimizer/pkg/recommendation"
)

func TestGenerateCostReport(t *testing.T) {
	// Create sample workload recommendations
	recommendations := []recommendation.WorkloadRecommendation{
		{
			Namespace:    "test-namespace",
			WorkloadKind: "Deployment",
			WorkloadName: "nginx",
			Containers: []recommendation.ContainerRecommendation{
				{
					ContainerName:     "nginx",
					CurrentCPU:        1000,              // 1000m (1 core)
					CurrentMemory:     512 * 1024 * 1024, // 512Mi
					RecommendedCPU:    300,               // 300m (recommended)
					RecommendedMemory: 256 * 1024 * 1024, // 256Mi
					Confidence:        0.92,
					EstimatedSavings: &cost.SavingsEstimate{
						CurrentCost: cost.ResourceCost{
							TotalPerMonth: 100.00,
						},
						RecommendedCost: cost.ResourceCost{
							TotalPerMonth: 60.00,
						},
					},
					HasOOMHistory: false,
				},
			},
		},
		{
			Namespace:    "test-namespace",
			WorkloadKind: "StatefulSet",
			WorkloadName: "redis",
			Containers: []recommendation.ContainerRecommendation{
				{
					ContainerName:     "redis",
					CurrentCPU:        500,
					CurrentMemory:     1024 * 1024 * 1024,
					RecommendedCPU:    600, // Under-provisioned
					RecommendedMemory: 1536 * 1024 * 1024,
					Confidence:        0.88,
					EstimatedSavings: &cost.SavingsEstimate{
						CurrentCost: cost.ResourceCost{
							TotalPerMonth: 50.00,
						},
						RecommendedCost: cost.ResourceCost{
							TotalPerMonth: 65.00, // Higher cost (need more resources)
						},
					},
					HasOOMHistory: true, // OOM risk
				},
			},
		},
	}

	// Generate report
	report := GenerateCostReport("test-cluster", recommendations)

	// Verify report structure
	if report == nil {
		t.Fatal("Report is nil")
		return
	}

	if report.ClusterName != "test-cluster" {
		t.Errorf("Expected cluster name 'test-cluster', got '%s'", report.ClusterName)
	}

	if report.TotalMonthlyCost <= 0 {
		t.Errorf("Expected positive total monthly cost, got %.2f", report.TotalMonthlyCost)
	}

	// Should have wasters
	if len(report.TopWasters) == 0 {
		t.Error("Expected at least one top waster")
	}

	// Verify first waster details (nginx - has savings)
	if len(report.TopWasters) > 0 {
		waster := report.TopWasters[0]
		if waster.Namespace != "test-namespace" {
			t.Errorf("Expected namespace 'test-namespace', got '%s'", waster.Namespace)
		}
		if waster.WorkloadType != "Deployment" {
			t.Errorf("Expected workload type 'Deployment', got '%s'", waster.WorkloadType)
		}
		if waster.WorkloadName != "nginx" {
			t.Errorf("Expected workload name 'nginx', got '%s'", waster.WorkloadName)
		}
		if waster.ContainerName != "nginx" {
			t.Errorf("Expected container name 'nginx', got '%s'", waster.ContainerName)
		}
		if waster.Savings <= 0 {
			t.Errorf("Expected positive savings, got %.2f", waster.Savings)
		}
	}

	// Verify summary
	if report.Summary.TotalWorkloads == 0 {
		t.Error("Expected non-zero total workloads in summary")
	}
}

func TestExportJSON(t *testing.T) {
	report := &CostReport{
		ClusterName:      "test-cluster",
		GeneratedAt:      time.Date(2026, 2, 26, 12, 0, 0, 0, time.UTC),
		TotalMonthlyCost: 1000.00,
		PotentialSavings: 250.00,
		WastePercentage:  25.0,
		TopWasters: []WorkloadWaste{
			{
				Namespace:     "prod",
				WorkloadType:  "Deployment",
				WorkloadName:  "api",
				ContainerName: "app",
				CurrentCost:   100.00,
				OptimizedCost: 60.00,
				Savings:       40.00,
				WasteReason:   "over-provisioned",
				Confidence:    0.92,
			},
		},
		Summary: ReportSummary{
			TotalWorkloads:        10,
			OverProvisioned:       6,
			UnderProvisioned:      1,
			OptimallyProvisioned:  3,
			AverageSavingsPercent: 25.0,
		},
	}

	var buf bytes.Buffer
	err := report.ExportJSON(&buf)
	if err != nil {
		t.Fatalf("ExportJSON failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed CostReport
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON parsing failed: %v", err)
	}

	// Verify key fields
	if parsed.ClusterName != "test-cluster" {
		t.Errorf("Expected cluster name 'test-cluster', got '%s'", parsed.ClusterName)
	}
	if parsed.PotentialSavings != 250.00 {
		t.Errorf("Expected savings 250.00, got %.2f", parsed.PotentialSavings)
	}
}

func TestExportCSV(t *testing.T) {
	report := &CostReport{
		ClusterName: "test-cluster",
		GeneratedAt: time.Now(),
		TopWasters: []WorkloadWaste{
			{
				Namespace:     "prod",
				WorkloadType:  "Deployment",
				WorkloadName:  "api",
				ContainerName: "app",
				CurrentCost:   100.00,
				OptimizedCost: 60.00,
				Savings:       40.00,
				WasteReason:   "over-provisioned",
				Confidence:    0.92,
			},
		},
	}

	var buf bytes.Buffer
	err := report.ExportCSV(&buf)
	if err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}

	output := buf.String()

	// Verify header
	if !strings.Contains(output, "Namespace") {
		t.Error("CSV missing header")
	}

	// Verify data row
	if !strings.Contains(output, "prod") {
		t.Error("CSV missing data")
	}
	if !strings.Contains(output, "$100.00") {
		t.Error("CSV missing current cost")
	}
	if !strings.Contains(output, "$40.00") {
		t.Error("CSV missing savings")
	}
}

func TestExportHTML(t *testing.T) {
	report := &CostReport{
		ClusterName:      "test-cluster",
		GeneratedAt:      time.Now(),
		TotalMonthlyCost: 1000.00,
		PotentialSavings: 250.00,
		WastePercentage:  25.0,
		TopWasters: []WorkloadWaste{
			{
				Namespace:     "prod",
				WorkloadType:  "Deployment",
				WorkloadName:  "api",
				ContainerName: "app",
				CurrentCost:   100.00,
				OptimizedCost: 60.00,
				Savings:       40.00,
				WasteReason:   "over-provisioned",
				Confidence:    0.92,
			},
		},
		Summary: ReportSummary{
			TotalWorkloads:        10,
			OverProvisioned:       6,
			UnderProvisioned:      1,
			OptimallyProvisioned:  3,
			AverageSavingsPercent: 25.0,
		},
	}

	var buf bytes.Buffer
	err := report.ExportHTML(&buf)
	if err != nil {
		t.Fatalf("ExportHTML failed: %v", err)
	}

	output := buf.String()

	// Verify HTML structure
	if !strings.Contains(output, "<!DOCTYPE html>") {
		t.Error("HTML missing doctype")
	}
	if !strings.Contains(output, "Cost Optimization Report") {
		t.Error("HTML missing title")
	}
	if !strings.Contains(output, "$1000.00") {
		t.Error("HTML missing total cost")
	}
	if !strings.Contains(output, "$250.00") {
		t.Error("HTML missing savings")
	}
	if !strings.Contains(output, "prod") {
		t.Error("HTML missing workload data")
	}
}

func TestSortByPotentialSavings(t *testing.T) {
	report := &CostReport{
		TopWasters: []WorkloadWaste{
			{WorkloadName: "low", Savings: 10.00},
			{WorkloadName: "high", Savings: 100.00},
			{WorkloadName: "medium", Savings: 50.00},
		},
	}

	report.sortByPotentialSavings()

	// Verify sorted in descending order
	if report.TopWasters[0].WorkloadName != "high" {
		t.Errorf("Expected 'high' first, got '%s'", report.TopWasters[0].WorkloadName)
	}
	if report.TopWasters[1].WorkloadName != "medium" {
		t.Errorf("Expected 'medium' second, got '%s'", report.TopWasters[1].WorkloadName)
	}
	if report.TopWasters[2].WorkloadName != "low" {
		t.Errorf("Expected 'low' third, got '%s'", report.TopWasters[2].WorkloadName)
	}
}
