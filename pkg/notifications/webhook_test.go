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

func TestWebhookNotifier_Send(t *testing.T) {
	tests := []struct {
		name            string
		config          WebhookConfig
		event           *Event
		serverResponse  int
		wantErr         bool
		validatePayload func(t *testing.T, payload *webhookPayload)
		validateHeaders func(t *testing.T, headers http.Header)
	}{
		{
			name: "successful webhook notification",
			config: WebhookConfig{
				Headers: map[string]string{
					"Authorization":   "Bearer token123",
					"X-Custom-Header": "custom-value",
				},
			},
			event: &Event{
				Type:      EventCircuitBreakerTriggered,
				Severity:  SeverityCritical,
				Namespace: "production",
				Workload:  "api-deployment",
				Message:   "Circuit breaker opened due to 5 consecutive errors",
				Timestamp: time.Now(),
				Metadata: map[string]interface{}{
					"consecutive_errors": 5,
					"error_threshold":    5,
				},
			},
			serverResponse: http.StatusOK,
			wantErr:        false,
			validatePayload: func(t *testing.T, payload *webhookPayload) {
				if payload.Event == nil {
					t.Fatal("Expected non-nil event in payload")
				}
				if payload.Event.Type != EventCircuitBreakerTriggered {
					t.Errorf("Expected event type %s, got %s", EventCircuitBreakerTriggered, payload.Event.Type)
				}
				if payload.Event.Severity != SeverityCritical {
					t.Errorf("Expected severity %s, got %s", SeverityCritical, payload.Event.Severity)
				}
			},
			validateHeaders: func(t *testing.T, headers http.Header) {
				if headers.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type: application/json, got %s", headers.Get("Content-Type"))
				}
				if headers.Get("Authorization") != "Bearer token123" {
					t.Errorf("Expected Authorization header, got %s", headers.Get("Authorization"))
				}
				if headers.Get("X-Custom-Header") != "custom-value" {
					t.Errorf("Expected X-Custom-Header, got %s", headers.Get("X-Custom-Header"))
				}
			},
		},
		{
			name:   "webhook returns error status",
			config: WebhookConfig{},
			event: &Event{
				Type:      EventSLAViolation,
				Severity:  SeverityWarning,
				Namespace: "default",
				Message:   "SLA violation detected",
				Timestamp: time.Now(),
			},
			serverResponse:  http.StatusBadRequest,
			wantErr:         true,
			validatePayload: nil,
			validateHeaders: nil,
		},
		{
			name: "webhook with custom timeout",
			config: WebhookConfig{
				Timeout: 5 * time.Second,
			},
			event: &Event{
				Type:      EventRecommendationApplied,
				Severity:  SeverityInfo,
				Namespace: "default",
				Message:   "Test",
				Timestamp: time.Now(),
			},
			serverResponse:  http.StatusAccepted,
			wantErr:         false,
			validatePayload: nil,
			validateHeaders: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			var receivedPayload *webhookPayload
			var receivedHeaders http.Header
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Validate request
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}

				receivedHeaders = r.Header.Clone()

				// Read and parse payload
				body, _ := io.ReadAll(r.Body)
				var payload webhookPayload
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Errorf("Failed to unmarshal payload: %v", err)
				}
				receivedPayload = &payload

				w.WriteHeader(tt.serverResponse)
			}))
			defer server.Close()

			// Set URL in config
			tt.config.URL = server.URL

			// Create notifier
			notifier := NewWebhookNotifier(tt.config)

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

			// Validate headers if function provided
			if tt.validateHeaders != nil {
				tt.validateHeaders(t, receivedHeaders)
			}
		})
	}
}

func TestWebhookNotifier_EmptyURL(t *testing.T) {
	config := WebhookConfig{
		URL: "",
	}
	notifier := NewWebhookNotifier(config)

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

func TestWebhookNotifier_ContextCancellation(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := WebhookConfig{
		URL:     server.URL,
		Timeout: 5 * time.Second,
	}
	notifier := NewWebhookNotifier(config)

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

func TestWebhookNotifier_Name(t *testing.T) {
	config := WebhookConfig{
		URL: "https://example.com/webhook",
	}
	notifier := NewWebhookNotifier(config)

	if notifier.Name() != "webhook" {
		t.Errorf("Expected name 'webhook', got %s", notifier.Name())
	}
}

func TestWebhookNotifier_DefaultTimeout(t *testing.T) {
	config := WebhookConfig{
		URL: "https://example.com/webhook",
		// Timeout not specified - should default to 10s
	}
	notifier := NewWebhookNotifier(config)

	if notifier.client.Timeout != 10*time.Second {
		t.Errorf("Expected default timeout of 10s, got %v", notifier.client.Timeout)
	}
}

func TestWebhookNotifier_PayloadStructure(t *testing.T) {
	var receivedPayload webhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := WebhookConfig{
		URL: server.URL,
	}
	notifier := NewWebhookNotifier(config)

	event := &Event{
		Type:      EventRollbackExecuted,
		Severity:  SeverityWarning,
		Namespace: "production",
		Workload:  "frontend-deployment",
		Pod:       "frontend-pod-123",
		Container: "frontend",
		Message:   "Rollback executed due to SLA violation",
		Timestamp: time.Unix(1642000000, 0),
		Metadata: map[string]interface{}{
			"reason":           "SLA violation",
			"previous_version": "v1.2.3",
			"rollback_version": "v1.2.2",
		},
	}

	err := notifier.Send(context.Background(), event)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Validate payload structure
	if receivedPayload.Event == nil {
		t.Fatal("Expected non-nil event in payload")
	}

	if receivedPayload.Event.Namespace != "production" {
		t.Errorf("Expected namespace 'production', got %s", receivedPayload.Event.Namespace)
	}

	if receivedPayload.Event.Workload != "frontend-deployment" {
		t.Errorf("Expected workload 'frontend-deployment', got %s", receivedPayload.Event.Workload)
	}

	if receivedPayload.Event.Metadata["reason"] != "SLA violation" {
		t.Errorf("Expected reason metadata, got %v", receivedPayload.Event.Metadata["reason"])
	}
}
