package notifications

import "time"

// EventType represents the type of notification event
type EventType string

const (
	// EventRecommendationApplied is triggered when optimizer applies a resource change
	EventRecommendationApplied EventType = "RecommendationApplied"

	// EventOOMDetected is triggered when OOM kill history found for a container
	EventOOMDetected EventType = "OOMDetected"

	// EventMemoryLeakDetected is triggered when leakdetector flags a container
	EventMemoryLeakDetected EventType = "MemoryLeakDetected"

	// EventCircuitBreakerTriggered is triggered when safety circuit breaker blocks a change
	EventCircuitBreakerTriggered EventType = "CircuitBreakerTriggered"

	// EventSLAViolation is triggered when SLA monitor detects a breach
	EventSLAViolation EventType = "SLAViolation"

	// EventRollbackExecuted is triggered when a rollback is performed
	EventRollbackExecuted EventType = "RollbackExecuted"
)

// Severity represents the severity level of a notification event
type Severity string

const (
	// SeverityInfo represents informational events
	SeverityInfo Severity = "info"

	// SeverityWarning represents warning events
	SeverityWarning Severity = "warning"

	// SeverityCritical represents critical events
	SeverityCritical Severity = "critical"
)

// Event represents a notification event
type Event struct {
	// Type is the event type
	Type EventType `json:"type"`

	// Severity is the severity level
	Severity Severity `json:"severity"`

	// Namespace is the Kubernetes namespace
	Namespace string `json:"namespace"`

	// Pod is the pod name (optional)
	Pod string `json:"pod,omitempty"`

	// Container is the container name (optional)
	Container string `json:"container,omitempty"`

	// Workload is the workload name (deployment, statefulset, etc.)
	Workload string `json:"workload,omitempty"`

	// Message is the human-readable message
	Message string `json:"message"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// Metadata contains additional event-specific data
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// GetSeverityColor returns the color code for Slack attachments based on severity
func (e *Event) GetSeverityColor() string {
	switch e.Severity {
	case SeverityInfo:
		return "#36a64f" // green
	case SeverityWarning:
		return "#ff9900" // orange
	case SeverityCritical:
		return "#ff0000" // red
	default:
		return "#808080" // gray
	}
}

// GetTitle returns a formatted title for the event
func (e *Event) GetTitle() string {
	return string(e.Type) + " - " + string(e.Severity)
}
