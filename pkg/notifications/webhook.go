package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookNotifier sends notifications to a generic HTTP webhook
type WebhookNotifier struct {
	url     string
	headers map[string]string
	client  *http.Client
}

// WebhookConfig contains HTTP webhook configuration
type WebhookConfig struct {
	URL     string
	Headers map[string]string
	Timeout time.Duration
}

// NewWebhookNotifier creates a new generic webhook notifier
func NewWebhookNotifier(config WebhookConfig) *WebhookNotifier {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &WebhookNotifier{
		url:     config.URL,
		headers: config.Headers,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the name of the notifier
func (w *WebhookNotifier) Name() string {
	return "webhook"
}

// webhookPayload represents the JSON payload sent to the webhook
type webhookPayload struct {
	Event *Event `json:"event"`
}

// Send sends a notification to the webhook
func (w *WebhookNotifier) Send(ctx context.Context, event *Event) error {
	if w.url == "" {
		return fmt.Errorf("webhook URL is not configured")
	}

	// Build payload
	payload := webhookPayload{
		Event: event,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", w.url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	// Set default Content-Type
	req.Header.Set("Content-Type", "application/json")

	// Set custom headers
	for key, value := range w.headers {
		req.Header.Set(key, value)
	}

	// Send request
	// #nosec G704 - Webhook URL is configured by cluster admin, not user input
	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook notification: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-2xx status: %d", resp.StatusCode)
	}

	return nil
}
