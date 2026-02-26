package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SlackNotifier sends notifications to Slack via webhook
type SlackNotifier struct {
	webhookURL string
	client     *http.Client
}

// NewSlackNotifier creates a new Slack notifier
func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the name of the notifier
func (s *SlackNotifier) Name() string {
	return "slack"
}

// slackMessage represents a Slack message payload
type slackMessage struct {
	Text        string             `json:"text,omitempty"`
	Attachments []slackAttachment  `json:"attachments,omitempty"`
}

// slackAttachment represents a Slack attachment
type slackAttachment struct {
	Color     string        `json:"color,omitempty"`
	Title     string        `json:"title,omitempty"`
	Text      string        `json:"text,omitempty"`
	Fields    []slackField  `json:"fields,omitempty"`
	Timestamp int64         `json:"ts,omitempty"`
}

// slackField represents a field in a Slack attachment
type slackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// Send sends a notification to Slack
func (s *SlackNotifier) Send(ctx context.Context, event *Event) error {
	if s.webhookURL == "" {
		return fmt.Errorf("slack webhook URL is not configured")
	}

	// Build Slack message
	msg := s.buildSlackMessage(event)

	// Marshal to JSON
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal slack message: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send slack notification: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack API returned non-200 status: %d", resp.StatusCode)
	}

	return nil
}

// buildSlackMessage builds a Slack message from an event
func (s *SlackNotifier) buildSlackMessage(event *Event) *slackMessage {
	fields := []slackField{
		{
			Title: "Namespace",
			Value: event.Namespace,
			Short: true,
		},
	}

	if event.Workload != "" {
		fields = append(fields, slackField{
			Title: "Workload",
			Value: event.Workload,
			Short: true,
		})
	}

	if event.Pod != "" {
		fields = append(fields, slackField{
			Title: "Pod",
			Value: event.Pod,
			Short: true,
		})
	}

	if event.Container != "" {
		fields = append(fields, slackField{
			Title: "Container",
			Value: event.Container,
			Short: true,
		})
	}

	// Add metadata fields
	for key, value := range event.Metadata {
		fields = append(fields, slackField{
			Title: key,
			Value: fmt.Sprintf("%v", value),
			Short: false,
		})
	}

	return &slackMessage{
		Text: fmt.Sprintf("🔔 Intelligent Cluster Optimizer Alert"),
		Attachments: []slackAttachment{
			{
				Color:     event.GetSeverityColor(),
				Title:     event.GetTitle(),
				Text:      event.Message,
				Fields:    fields,
				Timestamp: event.Timestamp.Unix(),
			},
		},
	}
}
