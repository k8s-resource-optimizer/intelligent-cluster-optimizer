package trends

import (
	"testing"
	"time"
)

func TestPredictTimeToExhaustion(t *testing.T) {
	config := DefaultAnalyzerConfig()

	tests := []struct {
		name              string
		data              []float64
		limit             float64
		expectedRiskLevel RiskLevel
		expectTimeToLimit bool
	}{
		{
			name:              "stable under limit",
			data:              generateConstant(100, 50),
			limit:             100,
			expectedRiskLevel: RiskNone,
			expectTimeToLimit: false,
		},
		{
			name:              "approaching limit",
			data:              generateLinear(100, 50, 0.3), // Reaches ~80% utilisation, trending toward limit
			limit:             100,
			expectedRiskLevel: RiskMedium,
			expectTimeToLimit: true,
		},
		{
			name:              "at critical capacity",
			data:              generateConstant(100, 96),
			limit:             100,
			expectedRiskLevel: RiskCritical,
			expectTimeToLimit: true,
		},
		{
			name:              "over limit",
			data:              generateConstant(100, 105),
			limit:             100,
			expectedRiskLevel: RiskCritical,
			expectTimeToLimit: false, // Already over
		},
		{
			name:              "rapid growth",
			data:              generateExponential(50, 10, 1.02),
			limit:             30,
			expectedRiskLevel: RiskHigh,
			expectTimeToLimit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamps := generateTimestamps(len(tt.data), time.Hour)
			status := PredictTimeToExhaustion(tt.data, timestamps, tt.limit, config)

			if status == nil {
				t.Fatal("PredictTimeToExhaustion() returned nil")
			}

			if status.RiskLevel != tt.expectedRiskLevel {
				t.Errorf("RiskLevel = %v, want %v", status.RiskLevel, tt.expectedRiskLevel)
				t.Logf("Risk score: %.2f", status.RiskScore)
				t.Logf("Utilization: %.2f%%", status.CurrentUtilization)
			}

			if tt.expectTimeToLimit && status.TimeToLimit == nil {
				t.Error("Expected TimeToLimit to be set, but got nil")
			} else if !tt.expectTimeToLimit && status.TimeToLimit != nil && tt.expectedRiskLevel != RiskCritical {
				// Allow TimeToLimit for critical cases even if not explicitly expecting it
				t.Logf("Warning: Got TimeToLimit when not expected: %v", *status.TimeToLimit)
			}

			if status.Message == "" {
				t.Error("Expected non-empty message")
			}

			t.Logf("Status: %s", status.Message)
		})
	}
}

func TestCalculateRiskScore(t *testing.T) {
	tests := []struct {
		name         string
		utilization  float64
		timeToLimit  *time.Duration
		acceleration float64
		minScore     float64
		maxScore     float64
	}{
		{
			name:         "low risk",
			utilization:  30,
			timeToLimit:  nil,
			acceleration: 0,
			minScore:     0,
			maxScore:     20,
		},
		{
			name:         "high utilization",
			utilization:  90,
			timeToLimit:  nil,
			acceleration: 0,
			minScore:     30,
			maxScore:     50,
		},
		{
			name:         "imminent exhaustion",
			utilization:  80,
			timeToLimit:  durationPtr(24 * time.Hour), // 1 day
			acceleration: 0,
			minScore:     60,
			maxScore:     90,
		},
		{
			name:         "critical with acceleration",
			utilization:  85,
			timeToLimit:  durationPtr(12 * time.Hour),
			acceleration: 2.0,
			minScore:     80,
			maxScore:     100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := CalculateRiskScore(tt.utilization, tt.timeToLimit, tt.acceleration, nil)

			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("CalculateRiskScore() = %.2f, want between %.2f and %.2f",
					score, tt.minScore, tt.maxScore)
			}

			t.Logf("Risk score: %.2f", score)
		})
	}
}

func TestDetermineRiskLevel(t *testing.T) {
	tests := []struct {
		score    float64
		expected RiskLevel
	}{
		{0, RiskNone},
		{15, RiskNone},
		{25, RiskLow},
		{45, RiskMedium},
		{65, RiskHigh},
		{85, RiskCritical},
		{100, RiskCritical},
	}

	for _, tt := range tests {
		t.Run(string(tt.expected), func(t *testing.T) {
			result := determineRiskLevel(tt.score)
			if result != tt.expected {
				t.Errorf("determineRiskLevel(%.0f) = %v, want %v", tt.score, result, tt.expected)
			}
		})
	}
}

func TestRecommendCapacityAction(t *testing.T) {
	tests := []struct {
		name        string
		riskLevel   RiskLevel
		timeToLimit *time.Duration
		expected    CapacityAction
	}{
		{
			name:        "none - monitor",
			riskLevel:   RiskNone,
			timeToLimit: nil,
			expected:    ActionMonitor,
		},
		{
			name:        "low - monitor",
			riskLevel:   RiskLow,
			timeToLimit: durationPtr(30 * 24 * time.Hour),
			expected:    ActionMonitor,
		},
		{
			name:        "medium - plan",
			riskLevel:   RiskMedium,
			timeToLimit: durationPtr(14 * 24 * time.Hour),
			expected:    ActionPlan,
		},
		{
			name:        "high soon - immediate",
			riskLevel:   RiskHigh,
			timeToLimit: durationPtr(3 * 24 * time.Hour),
			expected:    ActionImmediate,
		},
		{
			name:        "high later - plan",
			riskLevel:   RiskHigh,
			timeToLimit: durationPtr(20 * 24 * time.Hour),
			expected:    ActionPlan,
		},
		{
			name:        "critical - immediate",
			riskLevel:   RiskCritical,
			timeToLimit: durationPtr(0),
			expected:    ActionImmediate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RecommendCapacityAction(tt.riskLevel, tt.timeToLimit)
			if result != tt.expected {
				t.Errorf("RecommendCapacityAction() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateCapacityMessage(t *testing.T) {
	tests := []struct {
		name          string
		utilization   float64
		timeToLimit   *time.Duration
		riskLevel     RiskLevel
		shouldContain []string
	}{
		{
			name:          "stable",
			utilization:   50,
			timeToLimit:   nil,
			riskLevel:     RiskNone,
			shouldContain: []string{"50.0%", "stable"},
		},
		{
			name:          "approaching in hours",
			utilization:   85,
			timeToLimit:   durationPtr(6 * time.Hour),
			riskLevel:     RiskCritical,
			shouldContain: []string{"85.0%", "hours", "critical"},
		},
		{
			name:          "approaching in days",
			utilization:   70,
			timeToLimit:   durationPtr(5 * 24 * time.Hour),
			riskLevel:     RiskMedium,
			shouldContain: []string{"70.0%", "days", "medium"},
		},
		{
			name:          "approaching in weeks",
			utilization:   60,
			timeToLimit:   durationPtr(14 * 24 * time.Hour),
			riskLevel:     RiskLow,
			shouldContain: []string{"60.0%", "weeks", "low"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := generateCapacityMessage(tt.utilization, tt.timeToLimit, tt.riskLevel)

			if msg == "" {
				t.Error("Expected non-empty message")
			}

			for _, substr := range tt.shouldContain {
				if !contains(msg, substr) {
					t.Errorf("Message should contain %q, got: %s", substr, msg)
				}
			}

			t.Logf("Message: %s", msg)
		})
	}
}

// Helper functions

func durationPtr(d time.Duration) *time.Duration {
	return &d
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
