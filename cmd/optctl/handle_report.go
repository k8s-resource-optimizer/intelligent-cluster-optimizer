package main

import (
	"context"
	"fmt"
	"os"

	"intelligent-cluster-optimizer/pkg/recommendation"
	"intelligent-cluster-optimizer/pkg/reports"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

func handleReport(kubeClient *kubernetes.Clientset, subcommand, namespace string) error {
	if subcommand != "cost" {
		return fmt.Errorf("unknown report subcommand: %s (only 'cost' is supported)", subcommand)
	}

	if namespace == "" {
		namespace = "default"
	}

	ctx := context.Background()

	// Get all pods in namespace
	pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found in namespace %s", namespace)
	}

	klog.Infof("Found %d pods in namespace %s", len(pods.Items), namespace)

	// TODO: In a real implementation, you would collect actual metrics here
	// For now, we'll return an error with instructions
	return fmt.Errorf(`
Cost report generation requires metric collection integration.

To generate a cost report:
1. Ensure the optimizer controller is running and collecting metrics
2. Use the controller's API to generate recommendations
3. Then use those recommendations to create the report

Example integration:
	// Collect metrics from storage
	// Generate recommendations using recommendation.Engine
	// Create report using reports.GenerateCostReport()

For now, use 'optctl cost <namespace>' for basic cost calculations.
`)
}

// GenerateReportFromRecommendations is a helper to generate reports from recommendations
func GenerateReportFromRecommendations(clusterName string, recs []recommendation.WorkloadRecommendation, outputFormat string) error {
	// Generate the report
	report := reports.GenerateCostReport(clusterName, recs)

	// Export based on format
	switch outputFormat {
	case "json":
		if err := report.ExportJSON(os.Stdout); err != nil {
			return fmt.Errorf("failed to export JSON: %w", err)
		}
	case "csv":
		if err := report.ExportCSV(os.Stdout); err != nil {
			return fmt.Errorf("failed to export CSV: %w", err)
		}
	case "html":
		if err := report.ExportHTML(os.Stdout); err != nil {
			return fmt.Errorf("failed to export HTML: %w", err)
		}
	default:
		return fmt.Errorf("unsupported format: %s (use json, csv, or html)", outputFormat)
	}

	return nil
}
