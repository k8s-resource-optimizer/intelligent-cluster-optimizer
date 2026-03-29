package controller

// Unit tests for extractCPUHistory and SetMLForecaster.
// Uses package controller (not controller_test) to access the package-private extractCPUHistory.

import (
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"intelligent-cluster-optimizer/pkg/forecaster"
	"intelligent-cluster-optimizer/pkg/models"
)

// ── extractCPUHistory ──────────────────────────────────────────────────────────

// Test 2a: empty input returns nil
func TestExtractCPUHistory_Empty(t *testing.T) {
	result := extractCPUHistory(nil)
	if result != nil {
		t.Errorf("expected nil for empty input, got %v", result)
	}
	result = extractCPUHistory([]models.PodMetric{})
	if result != nil {
		t.Errorf("expected nil for empty slice, got %v", result)
	}
}

// Test 2b: containers with zero LimitCPU and zero RequestCPU are skipped entirely
func TestExtractCPUHistory_ZeroCapacitySkipped(t *testing.T) {
	metrics := []models.PodMetric{
		{
			PodName:   "pod-a",
			Namespace: "default",
			Timestamp: time.Now(),
			Containers: []models.ContainerMetric{
				{ContainerName: "app", UsageCPU: 100, LimitCPU: 0, RequestCPU: 0},
			},
		},
	}
	result := extractCPUHistory(metrics)
	if len(result) != 0 {
		t.Errorf("expected zero-capacity pod to be skipped, got %v", result)
	}
}

// Test 2c: falls back to RequestCPU when LimitCPU is 0
func TestExtractCPUHistory_FallsBackToRequest(t *testing.T) {
	metrics := []models.PodMetric{
		{
			PodName:   "pod-a",
			Namespace: "default",
			Timestamp: time.Now(),
			Containers: []models.ContainerMetric{
				// LimitCPU=0 → should use RequestCPU=200 as capacity
				{ContainerName: "app", UsageCPU: 100, LimitCPU: 0, RequestCPU: 200},
			},
		},
	}
	result := extractCPUHistory(metrics)
	if len(result) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(result))
	}
	expected := 100.0 / 200.0 // 0.5
	if result[0] != expected {
		t.Errorf("expected %.4f, got %.4f", expected, result[0])
	}
}

// Test 2d: usage exceeding limit is clamped to 1.0
func TestExtractCPUHistory_ClampedToOne(t *testing.T) {
	metrics := []models.PodMetric{
		{
			PodName:   "pod-a",
			Namespace: "default",
			Timestamp: time.Now(),
			Containers: []models.ContainerMetric{
				{ContainerName: "app", UsageCPU: 500, LimitCPU: 200, RequestCPU: 200},
			},
		},
	}
	result := extractCPUHistory(metrics)
	if len(result) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(result))
	}
	if result[0] != 1.0 {
		t.Errorf("expected clamped value 1.0, got %.4f", result[0])
	}
}

// Test 2e: metrics are sorted by timestamp before processing
func TestExtractCPUHistory_TimestampOrdering(t *testing.T) {
	now := time.Now()
	// Provide in reverse order — output should reflect the sorted (ascending) order
	metrics := []models.PodMetric{
		{
			PodName: "pod-c", Timestamp: now.Add(2 * time.Minute),
			Containers: []models.ContainerMetric{{UsageCPU: 300, LimitCPU: 1000}},
		},
		{
			PodName: "pod-a", Timestamp: now,
			Containers: []models.ContainerMetric{{UsageCPU: 100, LimitCPU: 1000}},
		},
		{
			PodName: "pod-b", Timestamp: now.Add(time.Minute),
			Containers: []models.ContainerMetric{{UsageCPU: 200, LimitCPU: 1000}},
		},
	}
	result := extractCPUHistory(metrics)
	if len(result) != 3 {
		t.Fatalf("expected 3 data points, got %d", len(result))
	}
	// After sorting: 100/1000, 200/1000, 300/1000
	expected := []float64{0.1, 0.2, 0.3}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("index %d: expected %.4f, got %.4f (ordering is wrong)", i, v, result[i])
		}
	}
}

// Test 2f: normal utilisation (0 < usage < limit) is correctly normalised
func TestExtractCPUHistory_NormalUtilisation(t *testing.T) {
	metrics := []models.PodMetric{
		{
			PodName: "pod-a", Timestamp: time.Now(),
			Containers: []models.ContainerMetric{
				{UsageCPU: 250, LimitCPU: 1000},
				{UsageCPU: 150, LimitCPU: 500},
			},
		},
	}
	// total usage = 400, total capacity = 1500 → 400/1500 ≈ 0.2667
	result := extractCPUHistory(metrics)
	if len(result) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(result))
	}
	want := 400.0 / 1500.0
	if result[0] != want {
		t.Errorf("expected %.6f, got %.6f", want, result[0])
	}
}

// Test 2g: mixed pods — some zero-capacity, some valid — only valid ones included
func TestExtractCPUHistory_MixedPods(t *testing.T) {
	now := time.Now()
	metrics := []models.PodMetric{
		{
			PodName: "valid", Timestamp: now,
			Containers: []models.ContainerMetric{{UsageCPU: 100, LimitCPU: 400}},
		},
		{
			PodName: "zero-cap", Timestamp: now.Add(time.Second),
			Containers: []models.ContainerMetric{{UsageCPU: 50, LimitCPU: 0, RequestCPU: 0}},
		},
	}
	result := extractCPUHistory(metrics)
	if len(result) != 1 {
		t.Errorf("expected 1 valid point (zero-cap skipped), got %d", len(result))
	}
}

// ── SetMLForecaster ────────────────────────────────────────────────────────────

// Test 2h: SetMLForecaster stores the injected forecaster and scaler
func TestSetMLForecaster_Stored(t *testing.T) {
	logger := zaptest.NewLogger(t)
	r      := NewReconciler(fakeKube(), nil)
	fc     := forecaster.NewHoltWintersForecaster(logger)
	scaler := forecaster.NewHorizontalScaler(fakeKube(), logger)

	r.SetMLForecaster(fc, scaler)

	if r.mlForecaster == nil {
		t.Error("mlForecaster should not be nil after SetMLForecaster")
	}
	if r.mlScaler == nil {
		t.Error("mlScaler should not be nil after SetMLForecaster")
	}
}

// Test 2i: SetMLForecaster with nil clears the forecaster (allows disabling at runtime)
func TestSetMLForecaster_NilClears(t *testing.T) {
	logger := zaptest.NewLogger(t)
	r      := NewReconciler(fakeKube(), nil)
	fc     := forecaster.NewHoltWintersForecaster(logger)

	r.SetMLForecaster(fc, nil)
	r.SetMLForecaster(nil, nil)

	if r.mlForecaster != nil {
		t.Error("mlForecaster should be nil after clearing")
	}
}
