package trends

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/storage"
)

func TestExtractWorkloadName(t *testing.T) {
	tests := []struct {
		podName  string
		expected string
	}{
		{
			podName:  "nginx-deployment-5d59f5d8c8-abc123",
			expected: "nginx-deployment",
		},
		{
			podName:  "api-server-67b8c9-xyz",
			expected: "api-server",
		},
		{
			podName:  "simple-pod",
			expected: "simple-pod",
		},
		{
			podName:  "my-app-v2-stable-678abc-pod1",
			expected: "my-app-v2-stable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.podName, func(t *testing.T) {
			result := extractWorkloadName(tt.podName)
			if result != tt.expected {
				t.Errorf("extractWorkloadName(%q) = %q, want %q", tt.podName, result, tt.expected)
			}
		})
	}
}

func TestExtractResourceTimeSeries(t *testing.T) {
	// Create test metrics
	metrics := []models.PodMetric{
		{
			PodName:   "test-pod",
			Namespace: "default",
			Timestamp: time.Now(),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      100,
					UsageMemory:   512,
					LimitCPU:      1000,
					LimitMemory:   2048,
				},
			},
		},
		{
			PodName:   "test-pod",
			Namespace: "default",
			Timestamp: time.Now().Add(time.Hour),
			Containers: []models.ContainerMetric{
				{
					ContainerName: "app",
					UsageCPU:      150,
					UsageMemory:   600,
					LimitCPU:      1000,
					LimitMemory:   2048,
				},
			},
		},
	}

	t.Run("CPU extraction", func(t *testing.T) {
		data, timestamps, limit := extractResourceTimeSeries(metrics, "cpu")

		if len(data) != 2 {
			t.Errorf("Expected 2 data points, got %d", len(data))
		}
		if len(timestamps) != 2 {
			t.Errorf("Expected 2 timestamps, got %d", len(timestamps))
		}
		if data[0] != 100 || data[1] != 150 {
			t.Errorf("Unexpected CPU data: %v", data)
		}
		if limit != 1000 {
			t.Errorf("Expected limit 1000, got %f", limit)
		}
	})

	t.Run("Memory extraction", func(t *testing.T) {
		data, _, limit := extractResourceTimeSeries(metrics, "memory")

		if len(data) != 2 {
			t.Errorf("Expected 2 data points, got %d", len(data))
		}
		if data[0] != 512 || data[1] != 600 {
			t.Errorf("Unexpected memory data: %v", data)
		}
		if limit != 2048 {
			t.Errorf("Expected limit 2048, got %f", limit)
		}
	})
}

func TestAssessDataQuality(t *testing.T) {
	tests := []struct {
		name            string
		dataPoints      int
		trendStrength   float64
		expectedQuality string
		minConfidence   float64
		maxConfidence   float64
	}{
		{
			name:            "excellent - 30 days with strong trend",
			dataPoints:      720,
			trendStrength:   0.9,
			expectedQuality: "excellent",
			minConfidence:   85,
			maxConfidence:   95,
		},
		{
			name:            "good - 14 days with moderate trend",
			dataPoints:      336,
			trendStrength:   0.6,
			expectedQuality: "good",
			minConfidence:   70,
			maxConfidence:   85,
		},
		{
			name:            "fair - 7 days with weak trend",
			dataPoints:      168,
			trendStrength:   0.3,
			expectedQuality: "fair",
			minConfidence:   55,
			maxConfidence:   75,
		},
		{
			name:            "poor - minimal data",
			dataPoints:      48,
			trendStrength:   0.2,
			expectedQuality: "poor",
			minConfidence:   40,
			maxConfidence:   65,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]float64, tt.dataPoints)
			quality, confidence := assessDataQuality(data, tt.dataPoints, tt.trendStrength)

			if quality != tt.expectedQuality {
				t.Errorf("Quality = %q, want %q", quality, tt.expectedQuality)
			}

			if confidence < tt.minConfidence || confidence > tt.maxConfidence {
				t.Errorf("Confidence = %.2f, want between %.2f and %.2f",
					confidence, tt.minConfidence, tt.maxConfidence)
			}

			t.Logf("Quality: %s, Confidence: %.2f%%", quality, confidence)
		})
	}
}

func TestGroupMetricsByWorkload(t *testing.T) {
	metrics := []models.PodMetric{
		{PodName: "nginx-abc-123", Namespace: "default"},
		{PodName: "nginx-abc-456", Namespace: "default"},
		{PodName: "api-server-xyz-789", Namespace: "default"},
		{PodName: "redis-master-abc", Namespace: "default"},
	}

	workloads := groupMetricsByWorkload(metrics)

	expectedWorkloads := []string{"nginx", "api-server", "redis-master"}
	if len(workloads) != len(expectedWorkloads) {
		t.Errorf("Expected %d workloads, got %d", len(expectedWorkloads), len(workloads))
	}

	for _, expected := range expectedWorkloads {
		found := false
		for workload := range workloads {
			if workload == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to find workload %q", expected)
		}
	}
}

func TestTrendAnalyzerCreation(t *testing.T) {
	store := storage.NewStorage()

	t.Run("with default config", func(t *testing.T) {
		analyzer := NewTrendAnalyzer(store, nil)
		if analyzer == nil {
			t.Fatal("NewTrendAnalyzer returned nil")
		}
		if analyzer.config == nil {
			t.Error("Expected default config to be set")
		}
		if analyzer.storage == nil {
			t.Error("Expected storage to be set")
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		config := &AnalyzerConfig{
			MinDataPoints: 100,
		}
		analyzer := NewTrendAnalyzer(store, config)
		if analyzer.config.MinDataPoints != 100 {
			t.Errorf("Expected MinDataPoints = 100, got %d", analyzer.config.MinDataPoints)
		}
	})
}
