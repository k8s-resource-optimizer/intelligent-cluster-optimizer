package notifications

import (
	"context"
	"fmt"
	"time"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ManagerBuilder builds a notification Manager from OptimizerConfig
type ManagerBuilder struct {
	logger *zap.Logger
	client client.Client
}

// NewManagerBuilder creates a new ManagerBuilder
func NewManagerBuilder(logger *zap.Logger, client client.Client) *ManagerBuilder {
	return &ManagerBuilder{
		logger: logger,
		client: client,
	}
}

// BuildFromConfig builds a notification Manager from an OptimizerConfig
// Returns nil if notifications are not enabled
func (b *ManagerBuilder) BuildFromConfig(ctx context.Context, config *optimizerv1alpha1.OptimizerConfig) (*Manager, error) {
	if config.Spec.Notifications == nil || !config.Spec.Notifications.Enabled {
		return nil, nil
	}

	notifConfig := config.Spec.Notifications
	manager := NewManager(b.logger)

	// Add Slack notifier if configured
	if notifConfig.Slack != nil && notifConfig.Slack.WebhookURL != "" {
		slackNotifier := NewSlackNotifier(notifConfig.Slack.WebhookURL)
		manager.AddNotifier(slackNotifier)
		b.logger.Info("added Slack notifier", zap.String("config", config.Name))
	}

	// Add Email notifier if configured
	if notifConfig.Email != nil && notifConfig.Email.SMTPHost != "" {
		emailConfig, err := b.buildEmailConfig(ctx, config, notifConfig.Email)
		if err != nil {
			return nil, fmt.Errorf("failed to build email config: %w", err)
		}
		emailNotifier := NewEmailNotifier(*emailConfig)
		manager.AddNotifier(emailNotifier)
		b.logger.Info("added email notifier",
			zap.String("config", config.Name),
			zap.String("smtp_host", emailConfig.SMTPHost),
			zap.Int("smtp_port", emailConfig.SMTPPort))
	}

	// Add Webhook notifiers if configured
	for i, webhookCfg := range notifConfig.Webhooks {
		timeout, err := b.parseTimeout(webhookCfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout for webhook[%d]: %w", i, err)
		}

		webhookNotifier := NewWebhookNotifier(WebhookConfig{
			URL:     webhookCfg.URL,
			Headers: webhookCfg.Headers,
			Timeout: timeout,
		})
		manager.AddNotifier(webhookNotifier)
		b.logger.Info("added webhook notifier",
			zap.String("config", config.Name),
			zap.String("name", webhookCfg.Name),
			zap.String("url", webhookCfg.URL))
	}

	if manager.GetNotifierCount() == 0 {
		b.logger.Warn("notifications enabled but no notifiers configured",
			zap.String("config", config.Name))
		return nil, nil
	}

	b.logger.Info("notification manager built successfully",
		zap.String("config", config.Name),
		zap.Int("notifier_count", manager.GetNotifierCount()))

	return manager, nil
}

// buildEmailConfig builds EmailConfig with password retrieved from secret if configured
func (b *ManagerBuilder) buildEmailConfig(ctx context.Context, config *optimizerv1alpha1.OptimizerConfig, emailCfg *optimizerv1alpha1.EmailConfig) (*EmailConfig, error) {
	cfg := EmailConfig{
		SMTPHost:     emailCfg.SMTPHost,
		SMTPPort:     emailCfg.SMTPPort,
		Username:     emailCfg.Username,
		From:         emailCfg.From,
		To:           emailCfg.To,
		UseTLS:       emailCfg.UseTLS,
		InsecureSkip: emailCfg.InsecureSkipVerify,
	}

	// Default SMTP port if not specified
	if cfg.SMTPPort == 0 {
		cfg.SMTPPort = 587
	}

	// Retrieve password from secret if configured
	if emailCfg.PasswordSecretRef != nil {
		password, err := b.getPasswordFromSecret(ctx, config.Namespace, emailCfg.PasswordSecretRef)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve password from secret: %w", err)
		}
		cfg.Password = password
	}

	return &cfg, nil
}

// getPasswordFromSecret retrieves password from a Kubernetes secret
func (b *ManagerBuilder) getPasswordFromSecret(ctx context.Context, configNamespace string, secretRef *optimizerv1alpha1.SecretReference) (string, error) {
	// Determine secret namespace
	namespace := secretRef.Namespace
	if namespace == "" {
		namespace = configNamespace
	}

	// Get secret
	var secret corev1.Secret
	err := b.client.Get(ctx, types.NamespacedName{
		Name:      secretRef.Name,
		Namespace: namespace,
	}, &secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretRef.Name, err)
	}

	// Extract password
	password, ok := secret.Data["password"]
	if !ok {
		return "", fmt.Errorf("secret %s/%s does not contain 'password' key", namespace, secretRef.Name)
	}

	return string(password), nil
}

// parseTimeout parses timeout string, returns default if empty
func (b *ManagerBuilder) parseTimeout(timeoutStr string) (time.Duration, error) {
	if timeoutStr == "" {
		return 10 * time.Second, nil // Default timeout
	}

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return 0, err
	}

	if timeout <= 0 {
		return 0, fmt.Errorf("timeout must be positive, got %s", timeoutStr)
	}

	return timeout, nil
}

// ShouldNotifyEvent checks if an event type should trigger notifications
// Returns true if the event should be notified based on the config
func ShouldNotifyEvent(config *optimizerv1alpha1.OptimizerConfig, eventType EventType) bool {
	if config.Spec.Notifications == nil || !config.Spec.Notifications.Enabled {
		return false
	}

	// If no specific events configured, notify all events
	if len(config.Spec.Notifications.Events) == 0 {
		return true
	}

	// Check if event type is in the configured list
	for _, configuredEvent := range config.Spec.Notifications.Events {
		if string(configuredEvent) == string(eventType) {
			return true
		}
	}

	return false
}
