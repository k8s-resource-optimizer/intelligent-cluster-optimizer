package notifications

import (
	"testing"
	"time"
)

func TestEvent_GetSeverityColor(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		want     string
	}{
		{
			name:     "info severity returns green",
			severity: SeverityInfo,
			want:     "#36a64f",
		},
		{
			name:     "warning severity returns orange",
			severity: SeverityWarning,
			want:     "#ff9900",
		},
		{
			name:     "critical severity returns red",
			severity: SeverityCritical,
			want:     "#ff0000",
		},
		{
			name:     "unknown severity returns gray",
			severity: Severity("unknown"),
			want:     "#808080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &Event{Severity: tt.severity}
			if got := event.GetSeverityColor(); got != tt.want {
				t.Errorf("GetSeverityColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvent_GetTitle(t *testing.T) {
	tests := []struct {
		name  string
		event *Event
		want  string
	}{
		{
			name: "recommendation applied with info severity",
			event: &Event{
				Type:     EventRecommendationApplied,
				Severity: SeverityInfo,
			},
			want: "RecommendationApplied - info",
		},
		{
			name: "OOM detected with critical severity",
			event: &Event{
				Type:     EventOOMDetected,
				Severity: SeverityCritical,
			},
			want: "OOMDetected - critical",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.event.GetTitle(); got != tt.want {
				t.Errorf("GetTitle() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEventTypes(t *testing.T) {
	// Verify all event type constants are defined
	eventTypes := []EventType{
		EventRecommendationApplied,
		EventOOMDetected,
		EventMemoryLeakDetected,
		EventCircuitBreakerTriggered,
		EventSLAViolation,
		EventRollbackExecuted,
	}

	if len(eventTypes) != 6 {
		t.Errorf("Expected 6 event types, got %d", len(eventTypes))
	}
}

func TestSeverityLevels(t *testing.T) {
	// Verify all severity levels are defined
	severities := []Severity{
		SeverityInfo,
		SeverityWarning,
		SeverityCritical,
	}

	if len(severities) != 3 {
		t.Errorf("Expected 3 severity levels, got %d", len(severities))
	}
}

func TestEvent_Metadata(t *testing.T) {
	event := &Event{
		Type:      EventRecommendationApplied,
		Severity:  SeverityInfo,
		Namespace: "default",
		Pod:       "test-pod",
		Container: "test-container",
		Workload:  "test-deployment",
		Message:   "Test message",
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"cpu_before":    "100m",
			"cpu_after":     "200m",
			"memory_before": "256Mi",
			"memory_after":  "512Mi",
		},
	}

	if event.Metadata["cpu_before"] != "100m" {
		t.Errorf("Expected cpu_before=100m, got %v", event.Metadata["cpu_before"])
	}

	if event.Metadata["memory_after"] != "512Mi" {
		t.Errorf("Expected memory_after=512Mi, got %v", event.Metadata["memory_after"])
	}
}
