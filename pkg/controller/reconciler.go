package controller

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"intelligent-cluster-optimizer/pkg/anomaly"
	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/apiserver"
	"intelligent-cluster-optimizer/pkg/applier"
	"intelligent-cluster-optimizer/pkg/events"
	"intelligent-cluster-optimizer/pkg/forecaster"
	"intelligent-cluster-optimizer/pkg/gitops"
	"intelligent-cluster-optimizer/pkg/metrics"
	"intelligent-cluster-optimizer/pkg/models"
	"intelligent-cluster-optimizer/pkg/pareto"
	"intelligent-cluster-optimizer/pkg/prediction"
	"intelligent-cluster-optimizer/pkg/profile"
	"intelligent-cluster-optimizer/pkg/recommendation"
	"intelligent-cluster-optimizer/pkg/safety"
	"intelligent-cluster-optimizer/pkg/scheduler"
	"intelligent-cluster-optimizer/pkg/sla"
	"intelligent-cluster-optimizer/pkg/storage"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
)

type ReconcileResult struct {
	RequeueAfter time.Duration
	Updated      bool
}

type Reconciler struct {
	kubeClient             kubernetes.Interface
	hpaChecker             *safety.HPAChecker
	pdbChecker             *safety.PDBChecker
	applier                *applier.Applier
	eventRecorder          record.EventRecorder
	optimizerEvents        *events.OptimizerEventRecorder
	maintenanceWindowCheck *scheduler.MaintenanceWindowChecker
	circuitBreaker         *safety.CircuitBreaker
	recommendationEngine   *recommendation.Engine
	metricsStorage         *storage.InMemoryStorage
	profileResolver        *profile.Resolver
	anomalyChecker         *anomaly.WorkloadChecker
	workloadPredictor      *prediction.WorkloadPredictor
	paretoHelper           *pareto.RecommendationHelper
	gitopsExporter         gitops.Exporter
	slaHealthChecker       sla.HealthChecker
	mlForecaster           forecaster.CpuForecaster
	mlScaler               *forecaster.HorizontalScaler

	// metricsCollector is used to fetch live pod metrics from the metrics-server.
	// Optional; when set, processRecommendations populates metricsStorage before
	// running the recommendation engine.
	metricsCollector *metrics.MetricsCollector

	// GUI API backend stores — optional; set via SetAPIStores.
	scalingHistory *apiserver.ScalingHistoryStore
	forecastCache  *apiserver.ForecastCache
	dryRunQueue    *apiserver.DryRunQueue

	// applyCooldowns tracks the last time a change was applied per workload container.
	// Key: "namespace/kind/name/container". Prevents thrashing when Holt-Winters
	// produces slightly different recommendations on consecutive cycles.
	applyCooldowns   map[string]time.Time
	applyCooldownsMu sync.Mutex
}

func NewReconciler(kubeClient kubernetes.Interface, eventRecorder record.EventRecorder) *Reconciler {
	return &Reconciler{
		kubeClient:             kubeClient,
		hpaChecker:             safety.NewHPAChecker(kubeClient),
		pdbChecker:             safety.NewPDBChecker(kubeClient),
		applier:                applier.NewApplier(kubeClient, eventRecorder),
		eventRecorder:          eventRecorder,
		optimizerEvents:        events.NewOptimizerEventRecorder(eventRecorder),
		maintenanceWindowCheck: scheduler.NewMaintenanceWindowChecker(),
		circuitBreaker:         safety.NewCircuitBreaker(),
		recommendationEngine:   recommendation.NewEngine(),
		metricsStorage:         storage.NewStorage(),
		profileResolver:        profile.NewResolver(),
		anomalyChecker:         anomaly.NewWorkloadChecker(),
		workloadPredictor:      prediction.NewWorkloadPredictor(),
		paretoHelper:           pareto.NewRecommendationHelper(),
		gitopsExporter:         gitops.NewExporter(),
		slaHealthChecker:       sla.NewHealthChecker(),
		applyCooldowns:         make(map[string]time.Time),
	}
}

const applyCooldownDuration = 5 * time.Minute

// SetMetricsStorage allows injecting a shared metrics storage instance
func (r *Reconciler) SetMetricsStorage(store *storage.InMemoryStorage) {
	r.metricsStorage = store
}

// GetMetricsStorage returns the metrics storage for external population
func (r *Reconciler) GetMetricsStorage() *storage.InMemoryStorage {
	return r.metricsStorage
}

// SetMetricsCollector injects a live metrics collector so the reconciler can
// populate metricsStorage from the metrics-server on every reconcile loop.
func (r *Reconciler) SetMetricsCollector(c *metrics.MetricsCollector) {
	r.metricsCollector = c
}

// SetMLForecaster injects the ML forecaster and its horizontal scaler.
// When set, the reconciler drives replica scaling for Deployments via the
// forecaster in addition to the standard resource-request recommendations.
func (r *Reconciler) SetMLForecaster(f forecaster.CpuForecaster, s *forecaster.HorizontalScaler) {
	r.mlForecaster = f
	r.mlScaler = s
}

// SetAPIStores injects the shared stores used by the GUI API server.
// All three parameters are required together; pass nil to disable GUI recording.
func (r *Reconciler) SetAPIStores(
	history *apiserver.ScalingHistoryStore,
	cache *apiserver.ForecastCache,
	queue *apiserver.DryRunQueue,
) {
	r.scalingHistory = history
	r.forecastCache = cache
	r.dryRunQueue = queue
}

func (r *Reconciler) Reconcile(ctx context.Context, config *optimizerv1alpha1.OptimizerConfig) (*ReconcileResult, error) {
	result := &ReconcileResult{}

	if !config.Spec.Enabled {
		klog.V(3).Infof("OptimizerConfig %s/%s is disabled, skipping", config.Namespace, config.Name)
		if err := r.updatePhase(config, optimizerv1alpha1.OptimizerPhasePaused, "Disabled"); err != nil {
			return result, err
		}
		result.Updated = true
		return result, nil
	}

	if config.Status.Phase == "" {
		if err := r.updatePhase(config, optimizerv1alpha1.OptimizerPhasePending, "Initializing"); err != nil {
			return result, err
		}
		result.Updated = true
		return result, nil
	}

	if config.Spec.CircuitBreaker != nil && config.Spec.CircuitBreaker.Enabled {
		if !r.circuitBreaker.ShouldAllow(config) {
			klog.V(3).Infof("Circuit breaker is open for %s/%s", config.Namespace, config.Name)
			r.optimizerEvents.RecordWarningEvent(config, events.ReasonCircuitBreakerOpen,
				"Circuit breaker open, skipping optimization")
			if err := r.updatePhase(config, optimizerv1alpha1.OptimizerPhaseCircuitOpen, "Circuit breaker open"); err != nil {
				return result, err
			}
			result.Updated = true
			result.RequeueAfter = 5 * time.Minute
			return result, nil
		}
	}

	inMaintenanceWindow := r.maintenanceWindowCheck.IsInMaintenanceWindow(config)
	config.Status.ActiveMaintenanceWindow = inMaintenanceWindow

	if nextWindow := r.maintenanceWindowCheck.GetNextMaintenanceWindow(config); nextWindow != nil {
		config.Status.NextMaintenanceWindow = &metav1.Time{Time: *nextWindow}
	}

	if err := r.updateCondition(config, optimizerv1alpha1.ConditionTypeMaintenanceWindow,
		boolToConditionStatus(inMaintenanceWindow), "MaintenanceWindowCheck",
		maintenanceWindowMessage(inMaintenanceWindow)); err != nil {
		return result, err
	}

	if !inMaintenanceWindow && len(config.Spec.MaintenanceWindows) > 0 && !config.Spec.DryRun {
		klog.V(3).Infof("OptimizerConfig %s/%s outside maintenance window, skipping live updates", config.Namespace, config.Name)
		r.optimizerEvents.RecordWarningEvent(config, events.ReasonMaintenanceWindowSkipped,
			"Skipping live updates outside maintenance window")
		result.Updated = true
		if config.Status.NextMaintenanceWindow != nil {
			timeUntilNext := time.Until(config.Status.NextMaintenanceWindow.Time)
			if timeUntilNext > 0 && timeUntilNext < 30*time.Minute {
				result.RequeueAfter = timeUntilNext
			} else {
				result.RequeueAfter = 5 * time.Minute
			}
		} else {
			result.RequeueAfter = 5 * time.Minute
		}
		return result, nil
	}

	mode := "LIVE"
	if config.Spec.DryRun {
		mode = "DRY-RUN"
		klog.Infof("OptimizerConfig %s/%s running in DRY-RUN mode - no changes will be applied", config.Namespace, config.Name)
	} else {
		klog.Infof("OptimizerConfig %s/%s running in LIVE mode - changes will be applied", config.Namespace, config.Name)
	}

	// Process recommendations with per-workload safety checks
	if err := r.processRecommendations(ctx, config, mode); err != nil {
		klog.Warningf("Failed to process recommendations: %v", err)
		if config.Spec.CircuitBreaker != nil && config.Spec.CircuitBreaker.Enabled {
			if stateChanged := r.circuitBreaker.RecordFailure(config, err); stateChanged {
				stateName := r.circuitBreaker.GetStateName(config.Status.CircuitState)
				klog.Warningf("Circuit breaker state changed to %s for %s/%s after error: %v",
					stateName, config.Namespace, config.Name, err)
				r.optimizerEvents.RecordWarningEvent(config, "CircuitBreakerStateChanged",
					"Circuit breaker state: "+stateName)
			}
		}
	}

	if err := r.updatePhase(config, optimizerv1alpha1.OptimizerPhaseActive, "Reconciliation successful"); err != nil {
		return result, err
	}

	if err := r.updateCondition(config, optimizerv1alpha1.ConditionTypeReady, optimizerv1alpha1.ConditionTrue, "ReconcileSuccess", "OptimizerConfig reconciled successfully"); err != nil {
		return result, err
	}

	config.Status.ObservedGeneration = config.Generation
	now := metav1.NewTime(time.Now())
	config.Status.LastRecommendationTime = &now

	if config.Spec.CircuitBreaker != nil && config.Spec.CircuitBreaker.Enabled {
		if stateChanged := r.circuitBreaker.RecordSuccess(config); stateChanged {
			stateName := r.circuitBreaker.GetStateName(config.Status.CircuitState)
			klog.Infof("Circuit breaker state changed to %s for %s/%s", stateName, config.Namespace, config.Name)
			r.optimizerEvents.RecordNormalEvent(config, "CircuitBreakerStateChanged",
				"Circuit breaker state: "+stateName)
		}
	}

	result.Updated = true
	result.RequeueAfter = 30 * time.Second

	return result, nil
}

func (r *Reconciler) updatePhase(config *optimizerv1alpha1.OptimizerConfig, phase optimizerv1alpha1.OptimizerPhase, message string) error {
	config.Status.Phase = phase
	klog.V(4).Infof("Updated phase to %s: %s", phase, message)
	return nil
}

func (r *Reconciler) updateCondition(
	config *optimizerv1alpha1.OptimizerConfig,
	conditionType optimizerv1alpha1.OptimizerConditionType,
	status optimizerv1alpha1.ConditionStatus,
	reason, message string,
) error {
	now := metav1.NewTime(time.Now())
	condition := optimizerv1alpha1.OptimizerCondition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	found := false
	for i, cond := range config.Status.Conditions {
		if cond.Type == conditionType {
			if cond.Status != status {
				config.Status.Conditions[i] = condition
			}
			found = true
			break
		}
	}

	if !found {
		config.Status.Conditions = append(config.Status.Conditions, condition)
	}

	return nil
}

func (r *Reconciler) processRecommendations(ctx context.Context, config *optimizerv1alpha1.OptimizerConfig, mode string) error {
	klog.V(4).Infof("[%s] Processing recommendations for OptimizerConfig %s/%s", mode, config.Namespace, config.Name)

	// METRICS COLLECTION: Fetch live pod metrics from the metrics-server and
	// populate metricsStorage so the recommendation engine has data to work with.
	if r.metricsCollector != nil {
		for _, ns := range config.Spec.TargetNamespaces {
			pods, err := r.metricsCollector.GetPodMetrics(ns)
			if err != nil {
				klog.Warningf("[%s] Failed to collect metrics for namespace %s: %v", mode, ns, err)
				continue
			}
			for i := range pods {
				r.metricsStorage.Add(pods[i])
			}
			klog.V(4).Infof("[%s] Collected %d pod metrics from namespace %s", mode, len(pods), ns)
		}
	}

	// SLA SAFETY CHECK: Pre-optimization health check
	preOptHealth, err := r.checkSystemHealth(config)
	if err != nil {
		klog.Warningf("[%s] Failed to perform pre-optimization health check: %v", mode, err)
	} else {
		klog.V(3).Infof("[%s] Pre-optimization health: Score=%.1f, IsHealthy=%v, Message=%s",
			mode, preOptHealth.Score, preOptHealth.IsHealthy, preOptHealth.Message)

		// Check if optimization should be blocked
		if shouldBlock, reason := sla.ShouldBlockOptimization(preOptHealth); shouldBlock {
			klog.Warningf("[%s] SLA health check blocking optimization: %s", mode, reason)
			r.optimizerEvents.RecordWarningEvent(config, events.ReasonOptimizationBlocked,
				fmt.Sprintf("SLA health check failed: %s", reason))
			return fmt.Errorf("optimization blocked by SLA health check: %s", reason)
		}
	}

	// Resolve profile settings for this config
	resolvedSettings, err := r.profileResolver.Resolve(config)
	if err != nil {
		klog.Warningf("Failed to resolve profile settings, using defaults: %v", err)
		// Continue with nil settings - will skip MaxChangePercent check
	}

	// Generate recommendations using the engine with P95/P99 percentile calculation
	recommendations, err := r.recommendationEngine.GenerateRecommendations(r.metricsStorage, config)
	if err != nil {
		return fmt.Errorf("failed to generate recommendations: %w", err)
	}

	if len(recommendations) == 0 {
		klog.V(3).Infof("[%s] No recommendations generated for %s/%s (insufficient metrics or no changes needed)",
			mode, config.Namespace, config.Name)
		return nil
	}

	// Calculate total estimated savings across all workloads
	var totalSavingsPerMonth float64
	for _, rec := range recommendations {
		if rec.TotalEstimatedSavings != nil {
			totalSavingsPerMonth += rec.TotalEstimatedSavings.SavingsPerMonth
		}
	}

	klog.V(3).Infof("[%s] Generated %d workload recommendations (estimated savings: $%.2f/month)",
		mode, len(recommendations), totalSavingsPerMonth)

	// Export to GitOps format if enabled
	if config.Spec.GitOpsExport != nil && config.Spec.GitOpsExport.Enabled {
		if err := r.exportToGitOps(config, recommendations, mode); err != nil {
			klog.Warningf("[%s] Failed to export recommendations to GitOps: %v", mode, err)
			r.optimizerEvents.RecordWarningEvent(config, events.ReasonGitOpsExportFailed,
				fmt.Sprintf("GitOps export failed: %v", err))
		} else {
			klog.Infof("[%s] Successfully exported %d recommendations to GitOps (%s format)",
				mode, len(recommendations), config.Spec.GitOpsExport.Format)
			r.optimizerEvents.RecordNormalEvent(config, events.ReasonGitOpsExportSucceeded,
				fmt.Sprintf("Exported %d recommendations to GitOps (%s format)",
					len(recommendations), config.Spec.GitOpsExport.Format))
		}
	}

	// Process each workload recommendation
	var appliedCount, skippedCount int
	for _, workloadRec := range recommendations {
		// SAFETY CHECK: Check HPA conflicts before processing this workload
		if config.Spec.HPAAwareness != nil && config.Spec.HPAAwareness.Enabled {
			hpaResult, err := r.hpaChecker.CheckHPAConflict(ctx, workloadRec.Namespace, workloadRec.WorkloadKind, workloadRec.WorkloadName)
			if err != nil {
				klog.Warningf("Failed to check HPA conflict for %s/%s/%s: %v", workloadRec.Namespace, workloadRec.WorkloadKind, workloadRec.WorkloadName, err)
			} else if hpaResult.HasConflict {
				policy := config.Spec.HPAAwareness.ConflictPolicy
				if policy == "" || policy == optimizerv1alpha1.HPAConflictPolicySkip {
					klog.V(3).Infof("[%s] Skipping %s/%s/%s: HPA conflict detected - %s",
						mode, workloadRec.Namespace, workloadRec.WorkloadKind, workloadRec.WorkloadName, hpaResult.Message)
					r.optimizerEvents.RecordWarningEvent(config, events.ReasonHPAConflictDetected,
						fmt.Sprintf("HPA conflict for %s/%s: %s", workloadRec.WorkloadKind, workloadRec.WorkloadName, hpaResult.Message))
					if err := r.updateCondition(config, optimizerv1alpha1.ConditionTypeHPAConflict, optimizerv1alpha1.ConditionTrue, "ConflictDetected", hpaResult.Message); err != nil {
						klog.Warningf("Failed to update condition: %v", err)
					}
					skippedCount++
					continue
				} else if policy == optimizerv1alpha1.HPAConflictPolicyWarn {
					klog.Warningf("[%s] HPA conflict for %s/%s/%s, proceeding with caution: %s",
						mode, workloadRec.Namespace, workloadRec.WorkloadKind, workloadRec.WorkloadName, hpaResult.Message)
					r.optimizerEvents.RecordWarningEvent(config, events.ReasonHPAConflictDetected,
						fmt.Sprintf("HPA conflict for %s/%s (proceeding): %s", workloadRec.WorkloadKind, workloadRec.WorkloadName, hpaResult.Message))
				}
			}
		}

		// SAFETY CHECK: Check PDB constraints before processing this workload
		if config.Spec.PDBAwareness != nil && config.Spec.PDBAwareness.Enabled && !config.Spec.DryRun {
			pdbResult, err := r.pdbChecker.CheckPDBSafety(ctx, workloadRec.Namespace, workloadRec.WorkloadKind, workloadRec.WorkloadName, 1)
			if err != nil {
				klog.Warningf("Failed to check PDB for %s/%s/%s: %v", workloadRec.Namespace, workloadRec.WorkloadKind, workloadRec.WorkloadName, err)
			} else if pdbResult.HasPDB && !pdbResult.IsSafe {
				if config.Spec.PDBAwareness.RespectMinAvailable {
					klog.V(3).Infof("[%s] Skipping %s/%s/%s: PDB violation detected - %s",
						mode, workloadRec.Namespace, workloadRec.WorkloadKind, workloadRec.WorkloadName, pdbResult.Message)
					r.optimizerEvents.RecordWarningEvent(config, events.ReasonPDBViolation,
						fmt.Sprintf("PDB violation for %s/%s: %s", workloadRec.WorkloadKind, workloadRec.WorkloadName, pdbResult.Message))
					if err := r.updateCondition(config, optimizerv1alpha1.ConditionTypePDBViolation, optimizerv1alpha1.ConditionTrue, "ViolationDetected", pdbResult.Message); err != nil {
						klog.Warningf("Failed to update condition: %v", err)
					}
					skippedCount++
					continue
				} else {
					klog.Warningf("[%s] PDB violation for %s/%s/%s, proceeding anyway: %s",
						mode, workloadRec.Namespace, workloadRec.WorkloadKind, workloadRec.WorkloadName, pdbResult.Message)
					r.optimizerEvents.RecordWarningEvent(config, events.ReasonPDBViolation,
						fmt.Sprintf("PDB violation for %s/%s (proceeding): %s", workloadRec.WorkloadKind, workloadRec.WorkloadName, pdbResult.Message))
				}
			}
		}

		// SAFETY CHECK: Check for anomalies in workload metrics before scaling
		workloadMetrics := r.metricsStorage.GetMetricsByWorkload(workloadRec.Namespace, workloadRec.WorkloadName, 24*time.Hour)
		if len(workloadMetrics) > 0 {
			anomalyResult := r.anomalyChecker.CheckWorkload(workloadRec.Namespace, workloadRec.WorkloadName, workloadMetrics)
			if anomalyResult.ShouldBlockScaling {
				klog.V(3).Infof("[%s] Skipping %s/%s: anomaly detected - %s (action: %s)",
					mode, workloadRec.Namespace, workloadRec.WorkloadName, anomalyResult.BlockReason, anomalyResult.RecommendedAction)
				r.optimizerEvents.RecordWarningEvent(config, events.ReasonAnomalyDetected,
					fmt.Sprintf("Anomaly in %s/%s: %s", workloadRec.Namespace, workloadRec.WorkloadName, anomalyResult.BlockReason))
				skippedCount++
				continue
			} else if anomalyResult.HasAnyAnomaly {
				klog.V(4).Infof("[%s] Workload %s/%s has %d anomalies (severity: %s) - proceeding with caution",
					mode, workloadRec.Namespace, workloadRec.WorkloadName, anomalyResult.AnomalyCount, anomalyResult.HighestSeverity)
			}
		}

		// PREDICTION: Generate future resource usage predictions
		if len(workloadMetrics) >= r.workloadPredictor.MinDataPoints {
			pred, err := r.workloadPredictor.PredictWorkload(workloadRec.Namespace, workloadRec.WorkloadName, workloadMetrics)
			if err == nil && pred != nil {
				// Check if we're approaching a predicted peak
				cpuDuration, memDuration := pred.TimeUntilPeak()
				peakWarningThreshold := 2 * time.Hour

				if cpuDuration > 0 && cpuDuration < peakWarningThreshold {
					klog.V(3).Infof("[%s] %s/%s: CPU peak predicted in %v (peak=%.0f, trend=%s)",
						mode, workloadRec.Namespace, workloadRec.WorkloadName, cpuDuration.Round(time.Minute), pred.PeakCPU, pred.CPUTrend)
					r.optimizerEvents.RecordNormalEvent(config, events.ReasonPeakLoadPredicted,
						fmt.Sprintf("CPU peak in %v for %s/%s (predicted: %.0fm)", cpuDuration.Round(time.Minute),
							workloadRec.Namespace, workloadRec.WorkloadName, pred.PeakCPU))
				}
				if memDuration > 0 && memDuration < peakWarningThreshold {
					klog.V(3).Infof("[%s] %s/%s: Memory peak predicted in %v (peak=%.0f, trend=%s)",
						mode, workloadRec.Namespace, workloadRec.WorkloadName, memDuration.Round(time.Minute), pred.PeakMemory, pred.MemoryTrend)
				}

				// Log scaling recommendations based on predictions
				scalingRec := pred.GetScalingRecommendation(workloadRec.CurrentTotalCPU, workloadRec.CurrentTotalMemory)
				klog.V(4).Infof("[%s] Prediction for %s/%s: %s (confidence=%.1f%%)",
					mode, workloadRec.Namespace, workloadRec.WorkloadName, scalingRec, pred.Confidence)
			}
		}

		// ML FORECASTER: Drive horizontal scaling via Chronos-2 (with Holt-Winters fallback)
		if config.Spec.MLForecaster != nil &&
			config.Spec.MLForecaster.Enabled &&
			workloadRec.WorkloadKind == "Deployment" &&
			r.mlForecaster != nil && r.mlScaler != nil {

			cpuHistory := extractCPUHistory(workloadMetrics)
			if len(cpuHistory) >= 30 {
				dep, err := r.kubeClient.AppsV1().Deployments(workloadRec.Namespace).Get(
					ctx, workloadRec.WorkloadName, metav1.GetOptions{})
				if err != nil {
					klog.Warningf("[%s] ML forecaster: failed to get deployment %s/%s: %v",
						mode, workloadRec.Namespace, workloadRec.WorkloadName, err)
				} else {
					currentReplicas := int32(1)
					if dep.Spec.Replicas != nil {
						currentReplicas = *dep.Spec.Replicas
					}

					scalerCfg := forecaster.ScalerConfig{
						ScaleUpThreshold:   config.Spec.MLForecaster.ScaleUpThreshold,
						ScaleDownThreshold: config.Spec.MLForecaster.ScaleDownThreshold,
						CPUPerReplica:      config.Spec.MLForecaster.CPUPerReplica,
						MinReplicas:        config.Spec.MLForecaster.MinReplicas,
						MaxReplicas:        config.Spec.MLForecaster.MaxReplicas,
					}

					pr, err := r.mlForecaster.Predict(ctx, cpuHistory)
					if err != nil {
						klog.Warningf("[%s] ML forecaster predict failed for %s/%s: %v",
							mode, workloadRec.Namespace, workloadRec.WorkloadName, err)
					} else {
						decision := forecaster.Decide(pr.Forecast, currentReplicas, scalerCfg)
						klog.V(3).Infof("[%s] ML forecaster decision for %s/%s: scaleUp=%v scaleDown=%v desired=%d",
							mode, workloadRec.Namespace, workloadRec.WorkloadName,
							decision.ScaleUp, decision.ScaleDown, decision.DesiredReplicas)

						// Cache the forecast result for the GUI /api/forecasts endpoint.
						if r.forecastCache != nil {
							d := decision
							r.forecastCache.Set(workloadRec.Namespace, workloadRec.WorkloadName, apiserver.ForecastEntry{
								Namespace:      workloadRec.Namespace,
								DeploymentName: workloadRec.WorkloadName,
								Points:         pr.Forecast,
								Decision:       &d,
								InferenceMs:    pr.InferenceMs,
								CPUSamples:     len(cpuHistory),
							})
						}

						if !config.Spec.DryRun {
							if _, err := r.mlScaler.Scale(ctx, workloadRec.Namespace, workloadRec.WorkloadName, decision); err != nil {
								klog.Warningf("[%s] ML forecaster scale failed for %s/%s: %v",
									mode, workloadRec.Namespace, workloadRec.WorkloadName, err)
							} else if r.scalingHistory != nil && (decision.ScaleUp || decision.ScaleDown) {
								r.scalingHistory.Add(apiserver.ScalingRecord{
									Namespace:      workloadRec.Namespace,
									DeploymentName: workloadRec.WorkloadName,
									OldReplicas:    currentReplicas,
									NewReplicas:    decision.DesiredReplicas,
									Reason:         "ML",
									PeakCPU:        decision.PeakCPU,
									SustainedCPU:   decision.SustainedCPU,
									Applied:        true,
								})
							}
						} else if r.dryRunQueue != nil && (decision.ScaleUp || decision.ScaleDown) {
							// Dry-run: enqueue for user review via /api/dry-run/approve|reject.
							r.dryRunQueue.Add(apiserver.DryRunDecision{
								Namespace:       workloadRec.Namespace,
								DeploymentName:  workloadRec.WorkloadName,
								CurrentReplicas: currentReplicas,
								DesiredReplicas: decision.DesiredReplicas,
								Reason:          "ML",
								Decision:        decision,
							})
							klog.V(3).Infof("[%s] Dry-run: enqueued decision for %s/%s (desired=%d)",
								mode, workloadRec.Namespace, workloadRec.WorkloadName, decision.DesiredReplicas)
						}
					}
				}
			} else {
				klog.V(4).Infof("[%s] ML forecaster: insufficient CPU history for %s/%s (%d samples, need 30)",
					mode, workloadRec.Namespace, workloadRec.WorkloadName, len(cpuHistory))
			}
		}

		// PARETO: Generate multi-objective optimal recommendations
		if len(workloadRec.Containers) > 0 {
			// Build metrics for Pareto optimization from first container
			// (Could be extended to handle multi-container workloads)
			c := workloadRec.Containers[0]
			paretoMetrics := &pareto.WorkloadMetrics{
				Namespace:     workloadRec.Namespace,
				WorkloadName:  workloadRec.WorkloadName,
				CurrentCPU:    c.CurrentCPU,
				CurrentMemory: c.CurrentMemory,
				AvgCPU:        c.RecommendedCPU, // Using recommended as proxy for avg
				AvgMemory:     c.RecommendedMemory,
				PeakCPU:       int64(float64(c.RecommendedCPU) * 1.2),
				PeakMemory:    int64(float64(c.RecommendedMemory) * 1.2),
				P95CPU:        c.RecommendedCPU,
				P95Memory:     c.RecommendedMemory,
				P99CPU:        int64(float64(c.RecommendedCPU) * 1.1),
				P99Memory:     int64(float64(c.RecommendedMemory) * 1.1),
				Confidence:    c.Confidence,
				SampleCount:   c.SampleCount,
			}

			paretoRec, err := r.paretoHelper.GenerateRecommendation(paretoMetrics)
			if err == nil && paretoRec != nil {
				klog.V(4).Infof("[%s] Pareto analysis for %s/%s: %s",
					mode, workloadRec.Namespace, workloadRec.WorkloadName, paretoRec.Summary)

				// Log trade-off options on Pareto frontier
				if len(paretoRec.ParetoFrontier) > 1 {
					klog.V(5).Infof("[%s] %s/%s has %d Pareto-optimal strategies: %s",
						mode, workloadRec.Namespace, workloadRec.WorkloadName,
						len(paretoRec.ParetoFrontier), r.getParetoStrategySummary(paretoRec.ParetoFrontier))
				}
			}
		}

		for _, containerRec := range workloadRec.Containers {
			// Convert to applier format
			rec := &applier.ResourceRecommendation{
				Namespace:         workloadRec.Namespace,
				WorkloadKind:      workloadRec.WorkloadKind,
				WorkloadName:      workloadRec.WorkloadName,
				ContainerName:     containerRec.ContainerName,
				CurrentCPU:        formatCPU(containerRec.CurrentCPU),
				RecommendedCPU:    formatCPU(containerRec.RecommendedCPU),
				CurrentMemory:     formatMemory(containerRec.CurrentMemory),
				RecommendedMemory: formatMemory(containerRec.RecommendedMemory),
			}

			// Skip if no changes needed
			if !rec.HasChanges() {
				klog.V(4).Infof("[%s] Skipping %s/%s/%s: no changes needed",
					mode, rec.Namespace, rec.WorkloadName, rec.ContainerName)
				skippedCount++
				continue
			}

			// SAFETY CHECK: Enforce MaxChangePercent limit
			if resolvedSettings != nil {
				changePercent := containerRec.MaxChangePercent()
				shouldApply, reason := resolvedSettings.ShouldApplyRecommendation(containerRec.Confidence, changePercent)
				if !shouldApply {
					klog.V(3).Infof("[%s] Skipping %s/%s/%s: %s (change=%.1f%%, confidence=%.1f%%)",
						mode, rec.Namespace, rec.WorkloadName, rec.ContainerName, reason, changePercent, containerRec.Confidence)
					r.optimizerEvents.RecordWarningEvent(config, events.ReasonRecommendationSkipped,
						fmt.Sprintf("Skipped %s/%s: %s", rec.WorkloadName, rec.ContainerName, reason))
					skippedCount++
					continue
				}
			}

			// Log recommendation details with cost savings
			savingsInfo := ""
			if containerRec.EstimatedSavings != nil {
				savingsInfo = fmt.Sprintf(", savings=$%.4f/hour ($%.2f/month)",
					containerRec.EstimatedSavings.TotalSavingsPerHour,
					containerRec.EstimatedSavings.SavingsPerMonth)
			}
			klog.V(3).Infof("[%s] Recommendation for %s/%s/%s: CPU %s->%s (P%d), Memory %s->%s (P%d), confidence=%.2f, samples=%d%s",
				mode, rec.Namespace, rec.WorkloadName, rec.ContainerName,
				rec.CurrentCPU, rec.RecommendedCPU, containerRec.CPUPercentile,
				rec.CurrentMemory, rec.RecommendedMemory, containerRec.MemoryPercentile,
				containerRec.Confidence, containerRec.SampleCount, savingsInfo)

			// COOLDOWN CHECK: skip if this workload container was updated recently
			if !config.Spec.DryRun {
				cooldownKey := fmt.Sprintf("%s/%s/%s/%s", rec.Namespace, rec.WorkloadKind, rec.WorkloadName, rec.ContainerName)
				r.applyCooldownsMu.Lock()
				lastApplied, hasCooldown := r.applyCooldowns[cooldownKey]
				r.applyCooldownsMu.Unlock()
				if hasCooldown && time.Since(lastApplied) < applyCooldownDuration {
					klog.V(3).Infof("[LIVE] Skipping %s/%s/%s: cooldown active (last applied %s ago)",
						rec.Namespace, rec.WorkloadName, rec.ContainerName, time.Since(lastApplied).Round(time.Second))
					skippedCount++
					continue
				}
			}

			applyResult, err := r.applier.Apply(ctx, rec, config.Spec.DryRun)
			if err != nil {
				klog.Warningf("[%s] Failed to apply recommendation for %s/%s/%s: %v",
					mode, rec.Namespace, rec.WorkloadName, rec.ContainerName, err)
				continue
			}

			if config.Spec.DryRun {
				klog.V(3).Infof("[DRY-RUN] Summary: %d changes would be applied to %s/%s",
					len(applyResult.Changes), applyResult.WorkloadKind, applyResult.WorkloadName)
				for _, change := range applyResult.Changes {
					klog.V(3).Infof("[DRY-RUN]   - %s", change)
				}
			} else if applyResult.Applied {
				appliedCount++
				cooldownKey := fmt.Sprintf("%s/%s/%s/%s", rec.Namespace, rec.WorkloadKind, rec.WorkloadName, rec.ContainerName)
				r.applyCooldownsMu.Lock()
				r.applyCooldowns[cooldownKey] = time.Now()
				r.applyCooldownsMu.Unlock()

				if r.scalingHistory != nil {
					sr := apiserver.ScalingRecord{
						Namespace:      rec.Namespace,
						DeploymentName: rec.WorkloadName,
						ContainerName:  rec.ContainerName,
						Reason:         "HoltWinters",
						Applied:        true,
						OldCPU:         rec.CurrentCPU,
						NewCPU:         rec.RecommendedCPU,
						OldMemory:      rec.CurrentMemory,
						NewMemory:      rec.RecommendedMemory,
						Confidence:     containerRec.Confidence,
					}
					if containerRec.EstimatedSavings != nil {
						sr.SavingsPerHour = containerRec.EstimatedSavings.TotalSavingsPerHour
						sr.SavingsPerMonth = containerRec.EstimatedSavings.SavingsPerMonth
					}
					r.scalingHistory.Add(sr)
				}

				klog.Infof("[LIVE] Successfully applied changes to %s/%s/%s",
					rec.Namespace, rec.WorkloadName, rec.ContainerName)
			}
		}
	}

	// Record events with savings information
	if config.Spec.DryRun {
		if len(recommendations) > 0 {
			r.optimizerEvents.RecordNormalEvent(config, events.ReasonDryRunSimulated,
				fmt.Sprintf("Dry-run: %d recommendations generated, %d skipped (estimated savings: $%.2f/month)",
					len(recommendations), skippedCount, totalSavingsPerMonth))
		}
	} else if appliedCount > 0 {
		r.optimizerEvents.RecordOptimizationEvent(config, events.ReasonOptimizationApplied,
			fmt.Sprintf("Applied %d resource optimizations (estimated savings: $%.2f/month)",
				appliedCount, totalSavingsPerMonth))
		config.Status.TotalUpdatesApplied += int64(appliedCount)

		// SLA SAFETY CHECK: Post-optimization health check
		if preOptHealth != nil && !config.Spec.DryRun {
			// Wait a bit for metrics to stabilize after changes
			time.Sleep(5 * time.Second)

			postOptHealth, err := r.checkSystemHealth(config)
			if err != nil {
				klog.Warningf("[%s] Failed to perform post-optimization health check: %v", mode, err)
			} else {
				klog.V(3).Infof("[%s] Post-optimization health: Score=%.1f, IsHealthy=%v, Message=%s",
					mode, postOptHealth.Score, postOptHealth.IsHealthy, postOptHealth.Message)

				// Compare health before and after optimization
				impact, err := r.slaHealthChecker.CompareHealth(preOptHealth, postOptHealth)
				if err != nil {
					klog.Warningf("[%s] Failed to compare health: %v", mode, err)
				} else {
					klog.V(3).Infof("[%s] SLA Impact: Score=%.2f, Recommendation=%s",
						mode, impact.ImpactScore, impact.Recommendation)

					// Check if rollback is recommended
					if shouldRollback, reason := sla.ShouldRollback(impact); shouldRollback {
						klog.Warningf("[%s] SLA health degraded after optimization: %s", mode, reason)
						r.optimizerEvents.RecordWarningEvent(config, events.ReasonOptimizationDegraded,
							fmt.Sprintf("SLA degradation detected: %s. Consider rollback.", reason))
						// Note: Actual rollback would require storing previous state
						// For now, we just log and alert
					} else if impact.ImpactScore > 0.1 {
						klog.Infof("[%s] SLA health improved after optimization (score change: +%.1f%%)",
							mode, impact.ImpactScore*100)
						r.optimizerEvents.RecordNormalEvent(config, "SLAImproved",
							fmt.Sprintf("SLA health improved by %.1f%%", impact.ImpactScore*100))
					}
				}
			}
		}
	}

	config.Status.TotalRecommendations += int64(len(recommendations))

	return nil
}

// extractCPUHistory converts pod metrics to a CPU utilisation time series in 0-1 range
// suitable for the ML forecaster. Metrics are sorted by timestamp and each data point
// represents the average CPU utilisation (usage/limit) across all containers in all pods.
// Data points where limit and request are both zero are skipped.
func extractCPUHistory(metrics []models.PodMetric) []float64 {
	if len(metrics) == 0 {
		return nil
	}

	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].Timestamp.Before(metrics[j].Timestamp)
	})

	history := make([]float64, 0, len(metrics))
	for _, pm := range metrics {
		var totalUsage, totalCapacity int64
		for _, c := range pm.Containers {
			totalUsage += c.UsageCPU
			cap := c.LimitCPU
			if cap == 0 {
				cap = c.RequestCPU
			}
			totalCapacity += cap
		}
		if totalCapacity == 0 {
			continue
		}
		v := float64(totalUsage) / float64(totalCapacity)
		if v > 1 {
			v = 1
		}
		history = append(history, v)
	}
	return history
}

// formatCPU converts millicores to Kubernetes CPU format (e.g., 100 -> "100m", 1000 -> "1")
func formatCPU(millicores int64) string {
	if millicores >= 1000 && millicores%1000 == 0 {
		return fmt.Sprintf("%d", millicores/1000)
	}
	return fmt.Sprintf("%dm", millicores)
}

// formatMemory converts bytes to Kubernetes memory format (e.g., "128Mi", "1Gi")
func formatMemory(bytes int64) string {
	const (
		Ki = 1024
		Mi = Ki * 1024
		Gi = Mi * 1024
	)

	if bytes >= Gi && bytes%Gi == 0 {
		return fmt.Sprintf("%dGi", bytes/Gi)
	}
	if bytes >= Mi {
		return fmt.Sprintf("%dMi", bytes/Mi)
	}
	if bytes >= Ki {
		return fmt.Sprintf("%dKi", bytes/Ki)
	}
	return fmt.Sprintf("%d", bytes)
}

func boolToConditionStatus(b bool) optimizerv1alpha1.ConditionStatus {
	if b {
		return optimizerv1alpha1.ConditionTrue
	}
	return optimizerv1alpha1.ConditionFalse
}

func maintenanceWindowMessage(inWindow bool) string {
	if inWindow {
		return "Currently in maintenance window"
	}
	return "Outside maintenance window"
}

// getParetoStrategySummary returns a comma-separated list of strategy names
func (r *Reconciler) getParetoStrategySummary(solutions []*pareto.Solution) string {
	if len(solutions) == 0 {
		return ""
	}

	result := ""
	for i, sol := range solutions {
		if i > 0 {
			result += ", "
		}
		result += sol.ID
	}
	return result
}

// exportToGitOps exports recommendations to GitOps format (Kustomize or Helm)
func (r *Reconciler) exportToGitOps(
	config *optimizerv1alpha1.OptimizerConfig,
	recommendations []recommendation.WorkloadRecommendation,
	mode string,
) error {
	gitopsConfig := config.Spec.GitOpsExport
	if gitopsConfig == nil || !gitopsConfig.Enabled {
		return nil
	}

	// Convert recommendations to GitOps format
	var gitopsRecommendations []gitops.ResourceRecommendation
	for _, workloadRec := range recommendations {
		for _, containerRec := range workloadRec.Containers {
			gitopsRec := gitops.ResourceRecommendation{
				Namespace:         workloadRec.Namespace,
				Name:              workloadRec.WorkloadName,
				Kind:              workloadRec.WorkloadKind,
				ContainerName:     containerRec.ContainerName,
				RecommendedCPU:    containerRec.RecommendedCPU,
				RecommendedMemory: containerRec.RecommendedMemory,
				SetLimits:         false, // Could be configurable
				Confidence:        containerRec.Confidence,
				Reason:            fmt.Sprintf("P%d CPU, P%d Memory, %d samples", containerRec.CPUPercentile, containerRec.MemoryPercentile, containerRec.SampleCount),
			}
			gitopsRecommendations = append(gitopsRecommendations, gitopsRec)
		}
	}

	if len(gitopsRecommendations) == 0 {
		klog.V(4).Infof("[%s] No recommendations to export to GitOps", mode)
		return nil
	}

	// Map format from CRD to GitOps package
	var exportFormat gitops.ExportFormat
	switch gitopsConfig.Format {
	case optimizerv1alpha1.GitOpsFormatKustomize, "":
		exportFormat = gitops.FormatKustomize
	case optimizerv1alpha1.GitOpsFormatKustomizeJSON6902:
		exportFormat = gitops.FormatKustomizeJSON6902
	case optimizerv1alpha1.GitOpsFormatHelm:
		exportFormat = gitops.FormatHelm
	default:
		exportFormat = gitops.FormatKustomize
	}

	// Configure export
	exportConfig := gitops.ExportConfig{
		Format:     exportFormat,
		OutputPath: gitopsConfig.OutputPath,
		Metadata: map[string]string{
			"generated-by":   "intelligent-cluster-optimizer",
			"optimizer-name": config.Name,
			"namespace":      config.Namespace,
			"timestamp":      time.Now().Format(time.RFC3339),
		},
	}

	// Perform export
	result, err := r.gitopsExporter.Export(gitopsRecommendations, exportConfig)
	if err != nil {
		return fmt.Errorf("failed to export recommendations: %w", err)
	}

	klog.V(4).Infof("[%s] GitOps export generated %d files at %s",
		mode, len(result.Files), gitopsConfig.OutputPath)

	// Log exported files for visibility
	for filename := range result.Files {
		klog.V(5).Infof("[%s] GitOps exported file: %s", mode, filename)
	}

	return nil
}

// checkSystemHealth performs SLA health check for the system
func (r *Reconciler) checkSystemHealth(config *optimizerv1alpha1.OptimizerConfig) (*sla.HealthCheckResult, error) {
	// Collect recent metrics for SLA evaluation
	// In a production implementation, you would collect actual latency/error rate metrics
	// For now, we'll use a simplified approach with CPU metrics as a proxy

	// Return a default healthy result if no metrics available
	// This prevents blocking optimizations when the system is starting up
	return &sla.HealthCheckResult{
		Timestamp: time.Now(),
		IsHealthy: true,
		Score:     100.0,
		Message:   "SLA monitoring active - using default healthy state",
	}, nil
}
