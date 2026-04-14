package profile

import (
	"fmt"
	"time"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
)

// Resolver resolves profile settings from OptimizerConfig
type Resolver struct {
	manager *ProfileManager
}

// NewResolver creates a new profile resolver
func NewResolver() *Resolver {
	return &Resolver{
		manager: NewProfileManager(),
	}
}

// ResolvedSettings contains the final resolved settings for an optimizer config
type ResolvedSettings struct {
	// ProfileName is the name of the profile used (or "custom" if none)
	ProfileName string

	// Strategy is the resolved optimization strategy
	Strategy string

	// CPUPercentile is the resolved CPU percentile
	CPUPercentile int

	// MemoryPercentile is the resolved memory percentile
	MemoryPercentile int

	// SafetyMargin is the resolved safety margin
	SafetyMargin float64

	// MinSamples is the minimum samples required
	MinSamples int

	// HistoryDuration is the history duration to use
	HistoryDuration time.Duration

	// MinConfidence is the minimum confidence score required
	MinConfidence float64

	// ApplyDelay is how long to wait before applying
	ApplyDelay time.Duration

	// MaxChangePercent is the maximum change percentage allowed
	MaxChangePercent float64

	// RequireApproval indicates if manual approval is needed
	RequireApproval bool

	// DryRun indicates if running in dry-run mode
	DryRun bool

	// RollbackOnError indicates if rollback is enabled
	RollbackOnError bool

	// CircuitBreakerEnabled indicates if circuit breaker is active
	CircuitBreakerEnabled bool

	// CircuitBreakerThreshold is the error threshold for circuit breaker
	CircuitBreakerThreshold int

	// Resource limits
	MinCPUMillicores   int64
	MinMemoryMegabytes int64
	MaxCPUMillicores   int64
	MaxMemoryMegabytes int64
}

// Resolve resolves the effective settings from an OptimizerConfig
func (r *Resolver) Resolve(config *optimizerv1alpha1.OptimizerConfig) (*ResolvedSettings, error) {
	spec := &config.Spec

	// If a profile is specified, use it as base
	if spec.Profile != "" && spec.Profile != optimizerv1alpha1.ProfileCustom {
		return r.resolveFromProfile(spec)
	}

	// Otherwise, use explicit settings from spec
	return r.resolveFromSpec(spec)
}

// resolveFromProfile resolves settings from a named profile
func (r *Resolver) resolveFromProfile(spec *optimizerv1alpha1.OptimizerConfigSpec) (*ResolvedSettings, error) {
	profileName := string(spec.Profile)
	profile, err := r.manager.GetProfile(profileName)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile %s: %w", profileName, err)
	}

	settings := profile.Settings

	// Apply overrides if specified
	resolved := &ResolvedSettings{
		ProfileName:             profileName,
		Strategy:                settings.Strategy,
		CPUPercentile:           settings.CPUPercentile,
		MemoryPercentile:        settings.MemoryPercentile,
		SafetyMargin:            settings.SafetyMargin,
		MinSamples:              settings.MinSamples,
		HistoryDuration:         settings.HistoryDuration,
		MinConfidence:           settings.MinConfidence,
		ApplyDelay:              settings.ApplyDelay,
		MaxChangePercent:        settings.MaxChangePercent,
		RequireApproval:         settings.RequireApproval,
		DryRun:                  settings.DryRunByDefault,
		RollbackOnError:         settings.RollbackOnError,
		CircuitBreakerEnabled:   settings.CircuitBreakerEnabled,
		CircuitBreakerThreshold: settings.CircuitBreakerThreshold,
		MinCPUMillicores:        settings.MinCPUMillicores,
		MinMemoryMegabytes:      settings.MinMemoryMegabytes,
		MaxCPUMillicores:        settings.MaxCPUMillicores,
		MaxMemoryMegabytes:      settings.MaxMemoryMegabytes,
	}

	// Apply profile overrides
	if spec.ProfileOverrides != nil {
		r.applyOverrides(resolved, spec.ProfileOverrides)
	}

	// Explicit spec values can also override profile defaults
	r.applySpecOverrides(resolved, spec)

	return resolved, nil
}

// resolveFromSpec resolves settings from explicit spec values (no profile)
func (r *Resolver) resolveFromSpec(spec *optimizerv1alpha1.OptimizerConfigSpec) (*ResolvedSettings, error) {
	// Start with balanced defaults
	resolved := &ResolvedSettings{
		ProfileName:             "custom",
		Strategy:                string(spec.Strategy),
		CPUPercentile:           95,
		MemoryPercentile:        95,
		SafetyMargin:            1.2,
		MinSamples:              10,
		HistoryDuration:         2 * time.Hour,
		MinConfidence:           50.0,
		ApplyDelay:              0,
		MaxChangePercent:        0, // No limit
		RequireApproval:         false,
		DryRun:                  spec.DryRun,
		RollbackOnError:         true,
		CircuitBreakerEnabled:   true,
		CircuitBreakerThreshold: 5,
		MinCPUMillicores:        10,
		MinMemoryMegabytes:      32,
		MaxCPUMillicores:        16000,
		MaxMemoryMegabytes:      64 * 1024,
	}

	// If strategy is empty, default to balanced
	if resolved.Strategy == "" {
		resolved.Strategy = "balanced"
	}

	// Apply explicit spec values
	r.applySpecOverrides(resolved, spec)

	return resolved, nil
}

// applyOverrides applies ProfileOverrides to resolved settings
func (r *Resolver) applyOverrides(resolved *ResolvedSettings, overrides *optimizerv1alpha1.ProfileOverrides) {
	if overrides.MinConfidence != nil {
		resolved.MinConfidence = *overrides.MinConfidence
	}
	if overrides.MaxChangePercent != nil {
		resolved.MaxChangePercent = *overrides.MaxChangePercent
	}
	if overrides.RequireApproval != nil {
		resolved.RequireApproval = *overrides.RequireApproval
	}
	if overrides.ApplyDelay != "" {
		if d, err := time.ParseDuration(overrides.ApplyDelay); err == nil {
			resolved.ApplyDelay = d
		}
	}
	if overrides.DryRun != nil {
		resolved.DryRun = *overrides.DryRun
	}
}

// applySpecOverrides applies explicit spec values to resolved settings
func (r *Resolver) applySpecOverrides(resolved *ResolvedSettings, spec *optimizerv1alpha1.OptimizerConfigSpec) {
	// Recommendation config overrides
	if spec.Recommendations != nil {
		if spec.Recommendations.CPUPercentile > 0 {
			resolved.CPUPercentile = spec.Recommendations.CPUPercentile
		}
		if spec.Recommendations.MemoryPercentile > 0 {
			resolved.MemoryPercentile = spec.Recommendations.MemoryPercentile
		}
		if spec.Recommendations.SafetyMargin > 0 {
			resolved.SafetyMargin = spec.Recommendations.SafetyMargin
		}
		if spec.Recommendations.MinSamples > 0 {
			resolved.MinSamples = spec.Recommendations.MinSamples
		}
		if spec.Recommendations.HistoryDuration != "" {
			if d, err := time.ParseDuration(spec.Recommendations.HistoryDuration); err == nil {
				resolved.HistoryDuration = d
			}
		}
	}

	// Circuit breaker config overrides
	if spec.CircuitBreaker != nil {
		resolved.CircuitBreakerEnabled = spec.CircuitBreaker.Enabled
		if spec.CircuitBreaker.ErrorThreshold > 0 {
			resolved.CircuitBreakerThreshold = spec.CircuitBreaker.ErrorThreshold
		}
	}

	// Resource thresholds
	if spec.ResourceThresholds != nil {
		if spec.ResourceThresholds.CPU != nil {
			if min := parseCPUToMillicores(spec.ResourceThresholds.CPU.Min); min > 0 {
				resolved.MinCPUMillicores = min
			}
			if max := parseCPUToMillicores(spec.ResourceThresholds.CPU.Max); max > 0 {
				resolved.MaxCPUMillicores = max
			}
		}
		if spec.ResourceThresholds.Memory != nil {
			if min := parseMemoryToMegabytes(spec.ResourceThresholds.Memory.Min); min > 0 {
				resolved.MinMemoryMegabytes = min
			}
			if max := parseMemoryToMegabytes(spec.ResourceThresholds.Memory.Max); max > 0 {
				resolved.MaxMemoryMegabytes = max
			}
		}
	}

	// DryRun from spec always takes precedence if explicitly set
	resolved.DryRun = spec.DryRun
}

// parseCPUToMillicores parses CPU string to millicores
func parseCPUToMillicores(cpu string) int64 {
	if cpu == "" {
		return 0
	}
	var value int64
	if _, err := fmt.Sscanf(cpu, "%dm", &value); err == nil {
		return value
	}
	// Try float first for values like "0.5" or "1.5"
	var floatValue float64
	if _, err := fmt.Sscanf(cpu, "%f", &floatValue); err == nil {
		return int64(floatValue * 1000)
	}
	if _, err := fmt.Sscanf(cpu, "%d", &value); err == nil {
		return value * 1000
	}
	return 0
}

// parseMemoryToMegabytes parses memory string to megabytes
func parseMemoryToMegabytes(memory string) int64 {
	if memory == "" {
		return 0
	}
	var value int64
	if _, err := fmt.Sscanf(memory, "%dGi", &value); err == nil {
		return value * 1024
	}
	if _, err := fmt.Sscanf(memory, "%dMi", &value); err == nil {
		return value
	}
	if _, err := fmt.Sscanf(memory, "%dG", &value); err == nil {
		return value * 1000
	}
	if _, err := fmt.Sscanf(memory, "%dM", &value); err == nil {
		return value
	}
	return 0
}

// GetEffectiveStrategy returns the strategy that should be used
func (r *ResolvedSettings) GetEffectiveStrategy() optimizerv1alpha1.OptimizationStrategy {
	switch r.Strategy {
	case "aggressive":
		return optimizerv1alpha1.StrategyAggressive
	case "conservative":
		return optimizerv1alpha1.StrategyConservative
	default:
		return optimizerv1alpha1.StrategyBalanced
	}
}

// ShouldApplyRecommendation checks if a recommendation should be applied based on settings
func (r *ResolvedSettings) ShouldApplyRecommendation(confidence float64, changePercent float64) (bool, string) {
	if r.DryRun {
		return false, "dry-run mode enabled"
	}

	if confidence < r.MinConfidence {
		return false, fmt.Sprintf("confidence %.1f%% below minimum %.1f%%", confidence, r.MinConfidence)
	}

	if r.MaxChangePercent > 0 && changePercent > r.MaxChangePercent {
		return false, fmt.Sprintf("change %.1f%% exceeds maximum %.1f%%", changePercent, r.MaxChangePercent)
	}

	if r.RequireApproval {
		return false, "manual approval required"
	}

	return true, ""
}

// Summary returns a human-readable summary of the resolved settings
func (r *ResolvedSettings) Summary() string {
	return fmt.Sprintf("Profile=%s, Strategy=%s, P%d/P%d, Margin=%.0f%%, MinConf=%.0f%%, DryRun=%v",
		r.ProfileName,
		r.Strategy,
		r.CPUPercentile,
		r.MemoryPercentile,
		(r.SafetyMargin-1)*100,
		r.MinConfidence,
		r.DryRun)
}
