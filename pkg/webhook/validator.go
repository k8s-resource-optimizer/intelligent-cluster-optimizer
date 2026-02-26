package webhook

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"

	"github.com/robfig/cron/v3"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ValidatorInterface defines the interface for OptimizerConfig validation
type ValidatorInterface interface {
	ValidateCreate(config *optimizerv1alpha1.OptimizerConfig) error
	ValidateUpdate(oldConfig, newConfig *optimizerv1alpha1.OptimizerConfig) error
	ValidateDelete(config *optimizerv1alpha1.OptimizerConfig) error
}

// OptimizerConfigValidator validates OptimizerConfig resources
type OptimizerConfigValidator struct {
	cronParser cron.Parser
}

// NewValidator creates a new OptimizerConfig validator
func NewValidator() ValidatorInterface {
	return &OptimizerConfigValidator{
		cronParser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// ValidateCreate validates a new OptimizerConfig
func (v *OptimizerConfigValidator) ValidateCreate(config *optimizerv1alpha1.OptimizerConfig) error {
	if err := v.validateTargetNamespaces(config); err != nil {
		return err
	}

	if err := v.validateProfile(config); err != nil {
		return err
	}

	if err := v.validateMaintenanceWindows(config); err != nil {
		return err
	}

	if err := v.validateResourceThresholds(config); err != nil {
		return err
	}

	if err := v.validateRecommendationConfig(config); err != nil {
		return err
	}

	if err := v.validateUpdateStrategy(config); err != nil {
		return err
	}

	if err := v.validateCircuitBreaker(config); err != nil {
		return err
	}

	if err := v.validateGitOpsExport(config); err != nil {
		return err
	}

	if err := v.validateExcludeWorkloads(config); err != nil {
		return err
	}

	if err := v.validateNotifications(config); err != nil {
		return err
	}

	return nil
}

// ValidateUpdate validates an OptimizerConfig update
func (v *OptimizerConfigValidator) ValidateUpdate(oldConfig, newConfig *optimizerv1alpha1.OptimizerConfig) error {
	// Run all create validations on new config
	if err := v.ValidateCreate(newConfig); err != nil {
		return err
	}

	// Additional update-specific validations can go here
	// For example, prevent certain fields from being modified after creation

	return nil
}

// ValidateDelete validates an OptimizerConfig deletion
func (v *OptimizerConfigValidator) ValidateDelete(config *optimizerv1alpha1.OptimizerConfig) error {
	// Deletion validations can go here
	// For example, check if there are dependent resources

	return nil
}

// validateTargetNamespaces ensures at least one namespace is specified
func (v *OptimizerConfigValidator) validateTargetNamespaces(config *optimizerv1alpha1.OptimizerConfig) error {
	if len(config.Spec.TargetNamespaces) == 0 {
		return fmt.Errorf("targetNamespaces must contain at least one namespace")
	}

	// Validate each namespace name format
	for _, ns := range config.Spec.TargetNamespaces {
		if ns == "" {
			return fmt.Errorf("targetNamespaces cannot contain empty strings")
		}
		// Kubernetes namespace naming rules: lowercase alphanumeric and hyphens
		if !regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`).MatchString(ns) {
			return fmt.Errorf("invalid namespace name '%s': must be lowercase alphanumeric with hyphens", ns)
		}
	}

	return nil
}

// validateProfile validates profile and profile overrides
func (v *OptimizerConfigValidator) validateProfile(config *optimizerv1alpha1.OptimizerConfig) error {
	if config.Spec.ProfileOverrides != nil {
		// Validate MinConfidence
		if config.Spec.ProfileOverrides.MinConfidence != nil {
			confidence := *config.Spec.ProfileOverrides.MinConfidence
			if confidence < 0 || confidence > 100 {
				return fmt.Errorf("profileOverrides.minConfidence must be between 0 and 100, got %.2f", confidence)
			}
		}

		// Validate MaxChangePercent
		if config.Spec.ProfileOverrides.MaxChangePercent != nil {
			maxChange := *config.Spec.ProfileOverrides.MaxChangePercent
			if maxChange < 0 || maxChange > 100 {
				return fmt.Errorf("profileOverrides.maxChangePercent must be between 0 and 100, got %.2f", maxChange)
			}
		}

		// Validate ApplyDelay duration format
		if config.Spec.ProfileOverrides.ApplyDelay != "" {
			if _, err := time.ParseDuration(config.Spec.ProfileOverrides.ApplyDelay); err != nil {
				return fmt.Errorf("profileOverrides.applyDelay has invalid duration format: %w", err)
			}
		}
	}

	return nil
}

// validateMaintenanceWindows validates maintenance window configurations
func (v *OptimizerConfigValidator) validateMaintenanceWindows(config *optimizerv1alpha1.OptimizerConfig) error {
	for i, window := range config.Spec.MaintenanceWindows {
		// Validate cron schedule
		if window.Schedule == "" {
			return fmt.Errorf("maintenanceWindows[%d].schedule cannot be empty", i)
		}

		_, err := v.cronParser.Parse(window.Schedule)
		if err != nil {
			return fmt.Errorf("maintenanceWindows[%d].schedule has invalid cron expression '%s': %w", i, window.Schedule, err)
		}

		// Validate duration format
		if window.Duration == "" {
			return fmt.Errorf("maintenanceWindows[%d].duration cannot be empty", i)
		}

		duration, err := time.ParseDuration(window.Duration)
		if err != nil {
			return fmt.Errorf("maintenanceWindows[%d].duration has invalid format '%s': %w", i, window.Duration, err)
		}

		if duration <= 0 {
			return fmt.Errorf("maintenanceWindows[%d].duration must be positive, got %s", i, window.Duration)
		}

		// Validate timezone if specified
		if window.Timezone != "" && window.Timezone != "UTC" {
			_, err := time.LoadLocation(window.Timezone)
			if err != nil {
				return fmt.Errorf("maintenanceWindows[%d].timezone '%s' is invalid: %w", i, window.Timezone, err)
			}
		}
	}

	return nil
}

// validateResourceThresholds validates resource threshold configurations
func (v *OptimizerConfigValidator) validateResourceThresholds(config *optimizerv1alpha1.OptimizerConfig) error {
	if config.Spec.ResourceThresholds == nil {
		return nil
	}

	// Validate CPU thresholds
	if config.Spec.ResourceThresholds.CPU != nil {
		if err := v.validateResourceLimit("cpu", config.Spec.ResourceThresholds.CPU); err != nil {
			return err
		}
	}

	// Validate Memory thresholds
	if config.Spec.ResourceThresholds.Memory != nil {
		if err := v.validateResourceLimit("memory", config.Spec.ResourceThresholds.Memory); err != nil {
			return err
		}
	}

	return nil
}

// validateResourceLimit validates a single resource limit (CPU or Memory)
func (v *OptimizerConfigValidator) validateResourceLimit(resourceType string, limit *optimizerv1alpha1.ResourceLimit) error {
	// Parse min value
	minQty, err := resource.ParseQuantity(limit.Min)
	if err != nil {
		return fmt.Errorf("resourceThresholds.%s.min has invalid format '%s': %w", resourceType, limit.Min, err)
	}

	// Parse max value
	maxQty, err := resource.ParseQuantity(limit.Max)
	if err != nil {
		return fmt.Errorf("resourceThresholds.%s.max has invalid format '%s': %w", resourceType, limit.Max, err)
	}

	// Ensure min < max
	if minQty.Cmp(maxQty) >= 0 {
		return fmt.Errorf("resourceThresholds.%s.min (%s) must be less than max (%s)", resourceType, limit.Min, limit.Max)
	}

	// Ensure values are positive
	if minQty.Sign() <= 0 {
		return fmt.Errorf("resourceThresholds.%s.min must be positive, got %s", resourceType, limit.Min)
	}

	return nil
}

// validateRecommendationConfig validates recommendation configuration
func (v *OptimizerConfigValidator) validateRecommendationConfig(config *optimizerv1alpha1.OptimizerConfig) error {
	if config.Spec.Recommendations == nil {
		return nil
	}

	rec := config.Spec.Recommendations

	// Validate CPUPercentile
	if rec.CPUPercentile != 0 {
		if rec.CPUPercentile < 50 || rec.CPUPercentile > 99 {
			return fmt.Errorf("recommendations.cpuPercentile must be between 50 and 99, got %d", rec.CPUPercentile)
		}
	}

	// Validate MemoryPercentile
	if rec.MemoryPercentile != 0 {
		if rec.MemoryPercentile < 50 || rec.MemoryPercentile > 99 {
			return fmt.Errorf("recommendations.memoryPercentile must be between 50 and 99, got %d", rec.MemoryPercentile)
		}
	}

	// Validate MinSamples
	if rec.MinSamples != 0 {
		if rec.MinSamples < 10 {
			return fmt.Errorf("recommendations.minSamples must be at least 10, got %d", rec.MinSamples)
		}
	}

	// Validate SafetyMargin
	if rec.SafetyMargin != 0 {
		if rec.SafetyMargin < 1.0 || rec.SafetyMargin > 3.0 {
			return fmt.Errorf("recommendations.safetyMargin must be between 1.0 and 3.0, got %.2f", rec.SafetyMargin)
		}
	}

	// Validate HistoryDuration
	if rec.HistoryDuration != "" {
		duration, err := time.ParseDuration(rec.HistoryDuration)
		if err != nil {
			return fmt.Errorf("recommendations.historyDuration has invalid format '%s': %w", rec.HistoryDuration, err)
		}
		if duration <= 0 {
			return fmt.Errorf("recommendations.historyDuration must be positive, got %s", rec.HistoryDuration)
		}
	}

	return nil
}

// validateUpdateStrategy validates update strategy configuration
func (v *OptimizerConfigValidator) validateUpdateStrategy(config *optimizerv1alpha1.OptimizerConfig) error {
	if config.Spec.UpdateStrategy == nil || config.Spec.UpdateStrategy.RollingUpdate == nil {
		return nil
	}

	rolling := config.Spec.UpdateStrategy.RollingUpdate

	// Validate MaxUnavailable
	if rolling.MaxUnavailable != "" {
		if err := v.validateIntOrPercentage("updateStrategy.rollingUpdate.maxUnavailable", rolling.MaxUnavailable); err != nil {
			return err
		}
	}

	// Validate MaxSurge
	if rolling.MaxSurge != "" {
		if err := v.validateIntOrPercentage("updateStrategy.rollingUpdate.maxSurge", rolling.MaxSurge); err != nil {
			return err
		}
	}

	return nil
}

// validateIntOrPercentage validates a value that can be an integer or percentage
func (v *OptimizerConfigValidator) validateIntOrPercentage(field, value string) error {
	// Check if it's a percentage
	if strings.HasSuffix(value, "%") {
		percentStr := strings.TrimSuffix(value, "%")
		percent, err := strconv.Atoi(percentStr)
		if err != nil {
			return fmt.Errorf("%s has invalid percentage format '%s': %w", field, value, err)
		}
		if percent < 0 || percent > 100 {
			return fmt.Errorf("%s percentage must be between 0 and 100, got %d", field, percent)
		}
		return nil
	}

	// Check if it's an integer
	intVal, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%s must be an integer or percentage, got '%s': %w", field, value, err)
	}
	if intVal < 0 {
		return fmt.Errorf("%s must be non-negative, got %d", field, intVal)
	}

	return nil
}

// validateCircuitBreaker validates circuit breaker configuration
func (v *OptimizerConfigValidator) validateCircuitBreaker(config *optimizerv1alpha1.OptimizerConfig) error {
	if config.Spec.CircuitBreaker == nil {
		return nil
	}

	cb := config.Spec.CircuitBreaker

	// Validate ErrorThreshold
	if cb.ErrorThreshold != 0 {
		if cb.ErrorThreshold < 1 || cb.ErrorThreshold > 20 {
			return fmt.Errorf("circuitBreaker.errorThreshold must be between 1 and 20, got %d", cb.ErrorThreshold)
		}
	}

	// Validate SuccessThreshold
	if cb.SuccessThreshold != 0 {
		if cb.SuccessThreshold < 1 || cb.SuccessThreshold > 10 {
			return fmt.Errorf("circuitBreaker.successThreshold must be between 1 and 10, got %d", cb.SuccessThreshold)
		}
	}

	// Validate Timeout
	if cb.Timeout != "" {
		timeout, err := time.ParseDuration(cb.Timeout)
		if err != nil {
			return fmt.Errorf("circuitBreaker.timeout has invalid format '%s': %w", cb.Timeout, err)
		}
		if timeout <= 0 {
			return fmt.Errorf("circuitBreaker.timeout must be positive, got %s", cb.Timeout)
		}
	}

	return nil
}

// validateGitOpsExport validates GitOps export configuration
func (v *OptimizerConfigValidator) validateGitOpsExport(config *optimizerv1alpha1.OptimizerConfig) error {
	if config.Spec.GitOpsExport == nil || !config.Spec.GitOpsExport.Enabled {
		return nil
	}

	gitops := config.Spec.GitOpsExport

	// Validate OutputPath is not empty
	if gitops.OutputPath == "" {
		return fmt.Errorf("gitOpsExport.outputPath cannot be empty when enabled")
	}

	// If AutoCommit is enabled, validate git repository config
	if gitops.AutoCommit {
		if gitops.GitRepository == nil {
			return fmt.Errorf("gitOpsExport.gitRepository is required when autoCommit is enabled")
		}

		repo := gitops.GitRepository

		// Validate URL is not empty and has valid format
		if repo.URL == "" {
			return fmt.Errorf("gitOpsExport.gitRepository.url cannot be empty")
		}

		// Basic URL validation (git@github.com:user/repo.git or https://github.com/user/repo.git)
		validURL := regexp.MustCompile(`^(https?://|git@)[\w\-.]+(:\d+)?(/|:)[\w\-./]+\.git$`)
		if !validURL.MatchString(repo.URL) {
			return fmt.Errorf("gitOpsExport.gitRepository.url has invalid format: %s", repo.URL)
		}

		// Validate CreatePR requirements
		if repo.CreatePR {
			if repo.PRTitle == "" {
				return fmt.Errorf("gitOpsExport.gitRepository.prTitle is required when createPR is enabled")
			}
		}
	}

	return nil
}

// validateExcludeWorkloads validates exclude workload regex patterns
func (v *OptimizerConfigValidator) validateExcludeWorkloads(config *optimizerv1alpha1.OptimizerConfig) error {
	for i, pattern := range config.Spec.ExcludeWorkloads {
		if pattern == "" {
			return fmt.Errorf("excludeWorkloads[%d] cannot be empty", i)
		}

		// Validate regex compiles
		_, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("excludeWorkloads[%d] has invalid regex pattern '%s': %w", i, pattern, err)
		}
	}

	return nil
}

// validateNotifications validates notification configuration
func (v *OptimizerConfigValidator) validateNotifications(config *optimizerv1alpha1.OptimizerConfig) error {
	if config.Spec.Notifications == nil || !config.Spec.Notifications.Enabled {
		return nil
	}

	notif := config.Spec.Notifications

	// Validate at least one notifier is configured
	hasNotifier := false
	if notif.Slack != nil && notif.Slack.WebhookURL != "" {
		hasNotifier = true
		// Validate Slack webhook URL format
		if !strings.HasPrefix(notif.Slack.WebhookURL, "https://hooks.slack.com/") {
			return fmt.Errorf("notifications.slack.webhookURL must be a valid Slack webhook URL")
		}
	}

	if notif.Email != nil {
		hasNotifier = true
		// Validate SMTP host
		if notif.Email.SMTPHost == "" {
			return fmt.Errorf("notifications.email.smtpHost cannot be empty")
		}

		// Validate SMTP port
		if notif.Email.SMTPPort != 0 {
			if notif.Email.SMTPPort < 1 || notif.Email.SMTPPort > 65535 {
				return fmt.Errorf("notifications.email.smtpPort must be between 1 and 65535, got %d", notif.Email.SMTPPort)
			}
		}

		// Validate From address
		if notif.Email.From == "" {
			return fmt.Errorf("notifications.email.from cannot be empty")
		}

		// Basic email format validation
		emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
		if !emailRegex.MatchString(notif.Email.From) {
			return fmt.Errorf("notifications.email.from has invalid email format: %s", notif.Email.From)
		}

		// Validate To addresses
		if len(notif.Email.To) == 0 {
			return fmt.Errorf("notifications.email.to must contain at least one email address")
		}

		for i, to := range notif.Email.To {
			if !emailRegex.MatchString(to) {
				return fmt.Errorf("notifications.email.to[%d] has invalid email format: %s", i, to)
			}
		}
	}

	if len(notif.Webhooks) > 0 {
		hasNotifier = true
		for i, webhook := range notif.Webhooks {
			// Validate name
			if webhook.Name == "" {
				return fmt.Errorf("notifications.webhooks[%d].name cannot be empty", i)
			}

			// Validate URL
			if webhook.URL == "" {
				return fmt.Errorf("notifications.webhooks[%d].url cannot be empty", i)
			}

			// Basic URL validation
			urlRegex := regexp.MustCompile(`^https?://[\w\-.]+(:\d+)?(/.*)?$`)
			if !urlRegex.MatchString(webhook.URL) {
				return fmt.Errorf("notifications.webhooks[%d].url has invalid format: %s", i, webhook.URL)
			}

			// Validate timeout format if specified
			if webhook.Timeout != "" {
				timeout, err := time.ParseDuration(webhook.Timeout)
				if err != nil {
					return fmt.Errorf("notifications.webhooks[%d].timeout has invalid duration format: %w", i, err)
				}
				if timeout <= 0 {
					return fmt.Errorf("notifications.webhooks[%d].timeout must be positive, got %s", i, webhook.Timeout)
				}
			}
		}
	}

	if !hasNotifier {
		return fmt.Errorf("notifications.enabled is true but no notifiers (slack, email, webhooks) are configured")
	}

	return nil
}
