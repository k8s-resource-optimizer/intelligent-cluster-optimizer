package notifications

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlackNotifier_Send(t *testing.T) {
	tests := []struct {
		name           string
		event          *Event
		serverResponse int
		wantErr        bool
		validatePayload func(t *testing.T, payload *slackMessage)
	}{
		{
			name: "successful notification",
			event: &Event{
				Type:      EventRecommendationApplied,
				Severity:  SeverityInfo,
				Namespace: "default",
				Workload:  "nginx-deployment",
				Pod:       "nginx-pod-123",
				Container: "nginx",
				Message:   "Successfully applied resource recommendations",
				Timestamp: time.Now(),
				Metadata: map[string]interface{}{
					"cpu_change":    "+50m",
					"memory_change": "+128Mi",
				},
			},
			serverResponse: http.StatusOK,
			wantErr:        false,
			validatePayload: func(t *testing.T, payload *slackMessage) {
				if payload.Text == "" {
					t.Error("Expected non-empty text")
				}
				if len(payload.Attachments) != 1 {
					t.Fatalf("Expected 1 attachment, got %d", len(payload.Attachments))
				}
				att := payload.Attachments[0]
				if att.Color != "#36a64f" {
					t.Errorf("Expected green color for info, got %s", att.Color)
				}
				if att.Title != "RecommendationApplied - info" {
					t.Errorf("Expected correct title, got %s", att.Title)
				}
				if att.Text != "Successfully applied resource recommendations" {
					t.Errorf("Expected correct message, got %s", att.Text)
				}
			},
		},
		{
			name: "critical OOM event",
			event: &Event{
				Type:      EventOOMDetected,
				Severity:  SeverityCritical,
				Namespace: "production",
				Workload:  "api-deployment",
				Pod:       "api-pod-456",
				Container: "api",
				Message:   "OOMKilled detected in container",
				Timestamp: time.Now(),
			},
			serverResponse: http.StatusOK,
			wantErr:        false,
			validatePayload: func(t *testing.T, payload *slackMessage) {
				att := payload.Attachments[0]
				if att.Color != "#ff0000" {
					t.Errorf("Expected red color for critical, got %s", att.Color)
				}
			},
		},
		{
			name: "server error",
			event: &Event{
				Type:      EventRecommendationApplied,
				Severity:  SeverityInfo,
				Namespace: "default",
				Message:   "Test",
				Timestamp: time.Now(),
			},
			serverResponse: http.StatusInternalServerError,
			wantErr:        true,
			validatePayload: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			var receivedPayload *slackMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Validate request
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
				}

				// Read and parse payload
				body, _ := io.ReadAll(r.Body)
				var msg slackMessage
				if err := json.Unmarshal(body, &msg); err != nil {
					t.Errorf("Failed to unmarshal payload: %v", err)
				}
				receivedPayload = &msg

				w.WriteHeader(tt.serverResponse)
			}))
			defer server.Close()

			// Create notifier
			notifier := NewSlackNotifier(server.URL)

			// Send notification
			ctx := context.Background()
			err := notifier.Send(ctx, tt.event)

			// Check error
			if (err != nil) != tt.wantErr {
				t.Errorf("Send() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Validate payload if function provided
			if tt.validatePayload != nil && receivedPayload != nil {
				tt.validatePayload(t, receivedPayload)
			}
		})
	}
}

func TestSlackNotifier_EmptyWebhookURL(t *testing.T) {
	notifier := NewSlackNotifier("")
	event := &Event{
		Type:      EventRecommendationApplied,
		Severity:  SeverityInfo,
		Namespace: "default",
		Message:   "Test",
		Timestamp: time.Now(),
	}

	err := notifier.Send(context.Background(), event)
	if err == nil {
		t.Error("Expected error for empty webhook URL")
	}
}

func TestSlackNotifier_ContextCancellation(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewSlackNotifier(server.URL)
	event := &Event{
		Type:      EventRecommendationApplied,
		Severity:  SeverityInfo,
		Namespace: "default",
		Message:   "Test",
		Timestamp: time.Now(),
	}

	// Create context that will be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := notifier.Send(ctx, event)
	if err == nil {
		t.Error("Expected error due to context cancellation")
	}
}

func TestSlackNotifier_Name(t *testing.T) {
	notifier := NewSlackNotifier("https://example.com")
	if notifier.Name() != "slack" {
		t.Errorf("Expected name 'slack', got %s", notifier.Name())
	}
}

func TestSlackNotifier_BuildMessage(t *testing.T) {
	notifier := NewSlackNotifier("https://example.com")

	event := &Event{
		Type:      EventMemoryLeakDetected,
		Severity:  SeverityWarning,
		Namespace: "production",
		Workload:  "backend-deployment",
		Pod:       "backend-pod-789",
		Container: "backend",
		Message:   "Memory leak detected: 50MB/hour growth rate",
		Timestamp: time.Unix(1642000000, 0),
		Metadata: map[string]interface{}{
			"growth_rate": "50MB/hour",
			"r2_score":    0.95,
		},
	}

	msg := notifier.buildSlackMessage(event)

	if msg.Text != "🔔 Intelligent Cluster Optimizer Alert" {
		t.Errorf("Expected correct text, got %s", msg.Text)
	}

	if len(msg.Attachments) != 1 {
		t.Fatalf("Expected 1 attachment, got %d", len(msg.Attachments))
	}

	att := msg.Attachments[0]
	if att.Color != "#ff9900" {
		t.Errorf("Expected orange color for warning, got %s", att.Color)
	}
	if att.Title != "MemoryLeakDetected - warning" {
		t.Errorf("Expected correct title, got %s", att.Title)
	}
	if att.Timestamp != 1642000000 {
		t.Errorf("Expected timestamp 1642000000, got %d", att.Timestamp)
	}

	// Check fields
	expectedFields := map[string]bool{
		"Namespace": true,
		"Workload":  true,
		"Pod":       true,
		"Container": true,
	}

	for _, field := range att.Fields {
		if expectedFields[field.Title] {
			delete(expectedFields, field.Title)
		}
	}

	if len(expectedFields) > 0 {
		t.Errorf("Missing expected fields: %v", expectedFields)
	}
}
