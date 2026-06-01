// Package apiserver exposes a lightweight REST API so that the GUI frontend
// can read live cluster state and issue actions (manual scale, dry-run approval).
package apiserver

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	optimizerv1alpha1 "intelligent-cluster-optimizer/pkg/apis/optimizer/v1alpha1"
	"intelligent-cluster-optimizer/pkg/applier"
	"intelligent-cluster-optimizer/pkg/forecaster"
	"intelligent-cluster-optimizer/pkg/storage"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// ── Server ────────────────────────────────────────────────────────────────────

// Server is the GUI backend HTTP server.
type Server struct {
	addr            string
	kubeClient      kubernetes.Interface
	optimizerClient *optimizerv1alpha1.OptimizerConfigClient
	metricsStorage  *storage.InMemoryStorage
	scalingHistory  *ScalingHistoryStore
	forecastCache   *ForecastCache
	dryRunQueue     *DryRunQueue
	mlScaler        *forecaster.HorizontalScaler
	verticalApplier *applier.Applier
	namespace       string
	httpServer      *http.Server
}

// Config holds the dependencies for the API server.
type Config struct {
	Addr            string
	Namespace       string
	KubeClient      kubernetes.Interface
	OptimizerClient *optimizerv1alpha1.OptimizerConfigClient
	MetricsStorage  *storage.InMemoryStorage
	ScalingHistory  *ScalingHistoryStore
	ForecastCache   *ForecastCache
	DryRunQueue     *DryRunQueue
	MLScaler        *forecaster.HorizontalScaler
	VerticalApplier *applier.Applier
}

// NewServer constructs a Server and registers all routes.
func NewServer(cfg Config) *Server {
	s := &Server{
		addr:            cfg.Addr,
		kubeClient:      cfg.KubeClient,
		optimizerClient: cfg.OptimizerClient,
		metricsStorage:  cfg.MetricsStorage,
		scalingHistory:  cfg.ScalingHistory,
		forecastCache:   cfg.ForecastCache,
		dryRunQueue:     cfg.DryRunQueue,
		mlScaler:        cfg.MLScaler,
		verticalApplier: cfg.VerticalApplier,
		namespace:       cfg.Namespace,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/pods", s.handlePods)
	mux.HandleFunc("/api/forecasts", s.handleForecasts)
	mux.HandleFunc("/api/scaling-history", s.handleScalingHistory)
	mux.HandleFunc("/api/optimizer-configs", s.handleOptimizerConfigs)
	mux.HandleFunc("/api/scale", s.handleScale)
	mux.HandleFunc("/api/dry-run/pending", s.handleDryRunPending)
	mux.HandleFunc("/api/dry-run/approve", s.handleDryRunApprove)
	mux.HandleFunc("/api/dry-run/reject", s.handleDryRunReject)
	mux.HandleFunc("/api/metrics-history", s.handleMetricsHistory)

	s.httpServer = &http.Server{
		Addr:         s.addr,
		Handler:      corsMiddleware(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s
}

// ServeHTTP implements http.Handler so the server can be used directly in tests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.httpServer.Handler.ServeHTTP(w, r)
}

// Start begins serving HTTP requests in a background goroutine and returns when
// ctx is cancelled (after a graceful shutdown).
func (s *Server) Start(ctx context.Context) {
	go func() {
		klog.Infof("API server listening on %s", s.addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			klog.Errorf("API server exited with error: %v", err)
		}
	}()

	<-ctx.Done()

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.httpServer.Shutdown(shutCtx); err != nil {
		klog.Errorf("API server shutdown error: %v", err)
	}
	klog.Info("API server stopped")
}

// ── CORS middleware ───────────────────────────────────────────────────────────

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Vary", "Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		klog.Errorf("failed to write JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ── /health ───────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ── /api/pods ─────────────────────────────────────────────────────────────────

// PodInfo is the response shape for a single pod.
type PodInfo struct {
	Name         string     `json:"name"`
	Namespace    string     `json:"namespace"`
	Status       string     `json:"status"`
	RestartCount int32      `json:"restart_count"`
	CPUUsage     int64      `json:"cpu_usage_millicores"`
	MemoryUsage  int64      `json:"memory_usage_bytes"`
	Node         string     `json:"node"`
	StartTime    *time.Time `json:"start_time,omitempty"`
}

func (s *Server) handlePods(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// List pods from all namespaces so the UI shows the full cluster picture.
	podList, err := s.kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pods: "+err.Error())
		return
	}

	// Collect latest CPU/memory from metrics storage (best-effort).
	metricsBuf := map[string]struct {
		cpu int64
		mem int64
	}{}
	if s.metricsStorage != nil {
		for _, metrics := range s.metricsStorage.GetAllMetrics() {
			if len(metrics) == 0 {
				continue
			}
			latest := metrics[len(metrics)-1]
			var cpu, mem int64
			for _, c := range latest.Containers {
				cpu += c.UsageCPU
				mem += c.UsageMemory
			}
			metricsBuf[latest.PodName] = struct {
				cpu int64
				mem int64
			}{cpu, mem}
		}
	}

	infos := make([]PodInfo, 0, len(podList.Items))
	for _, pod := range podList.Items {
		info := PodInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    podStatus(pod),
			Node:      pod.Spec.NodeName,
		}
		if pod.Status.StartTime != nil {
			t := pod.Status.StartTime.Time
			info.StartTime = &t
		}
		// Sum restart counts across all containers.
		for _, cs := range pod.Status.ContainerStatuses {
			info.RestartCount += cs.RestartCount
		}
		if m, ok := metricsBuf[pod.Name]; ok {
			info.CPUUsage = m.cpu
			info.MemoryUsage = m.mem
		}
		infos = append(infos, info)
	}

	writeJSON(w, http.StatusOK, infos)
}

// podStatus derives a simple status string from a Pod object.
func podStatus(pod corev1.Pod) string {
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}
	return string(pod.Status.Phase)
}

// ── /api/forecasts ────────────────────────────────────────────────────────────

func (s *Server) handleForecasts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	entries := s.forecastCache.GetAll()
	writeJSON(w, http.StatusOK, entries)
}

// ── /api/scaling-history ──────────────────────────────────────────────────────

func (s *Server) handleScalingHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	records := s.scalingHistory.GetAll()
	writeJSON(w, http.StatusOK, records)
}

// ── /api/optimizer-configs ────────────────────────────────────────────────────

// OptimizerConfigSummary is the slimmed-down view exposed to the GUI.
type OptimizerConfigSummary struct {
	Name                string `json:"name"`
	Namespace           string `json:"namespace"`
	Enabled             bool   `json:"enabled"`
	DryRun              bool   `json:"dry_run"`
	Phase               string `json:"phase"`
	MLForecasterEnabled bool   `json:"ml_forecaster_enabled"`
}

func (s *Server) handleOptimizerConfigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	if s.optimizerClient == nil {
		writeJSON(w, http.StatusOK, []OptimizerConfigSummary{})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	list, err := s.optimizerClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list optimizer configs: "+err.Error())
		return
	}

	summaries := make([]OptimizerConfigSummary, 0, len(list.Items))
	for _, cfg := range list.Items {
		mlEnabled := cfg.Spec.MLForecaster != nil && cfg.Spec.MLForecaster.Enabled
		summaries = append(summaries, OptimizerConfigSummary{
			Name:                cfg.Name,
			Namespace:           cfg.Namespace,
			Enabled:             cfg.Spec.Enabled,
			DryRun:              cfg.Spec.DryRun,
			Phase:               string(cfg.Status.Phase),
			MLForecasterEnabled: mlEnabled,
		})
	}
	writeJSON(w, http.StatusOK, summaries)
}

// ── /api/scale ────────────────────────────────────────────────────────────────

// ManualScaleRequest is the body for POST /api/scale.
type ManualScaleRequest struct {
	Namespace      string `json:"namespace"`
	DeploymentName string `json:"deployment_name"`
	Replicas       int32  `json:"replicas"`
	Reason         string `json:"reason"`
}

func (s *Server) handleScale(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req ManualScaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.DeploymentName == "" {
		writeError(w, http.StatusBadRequest, "deployment_name is required")
		return
	}
	if req.Namespace == "" {
		req.Namespace = s.namespace
	}
	if req.Replicas <= 0 {
		writeError(w, http.StatusBadRequest, "replicas must be > 0")
		return
	}
	if req.Reason == "" {
		req.Reason = "Manual"
	}

	if s.mlScaler == nil {
		writeError(w, http.StatusServiceUnavailable, "mlScaler not available")
		return
	}

	// Fetch current replica count for the history record.
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	dep, err := s.kubeClient.AppsV1().Deployments(req.Namespace).Get(ctx, req.DeploymentName, metav1.GetOptions{})
	if err != nil {
		writeError(w, http.StatusNotFound, "deployment not found: "+err.Error())
		return
	}
	currentReplicas := int32(1)
	if dep.Spec.Replicas != nil {
		currentReplicas = *dep.Spec.Replicas
	}

	decision := forecaster.ScaleDecision{
		DesiredReplicas: req.Replicas,
		ScaleUp:         req.Replicas > currentReplicas,
		ScaleDown:       req.Replicas < currentReplicas,
	}

	changed, err := s.mlScaler.Scale(ctx, req.Namespace, req.DeploymentName, decision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scale failed: "+err.Error())
		return
	}

	// Record to history.
	s.scalingHistory.Add(ScalingRecord{
		Namespace:      req.Namespace,
		DeploymentName: req.DeploymentName,
		OldReplicas:    currentReplicas,
		NewReplicas:    req.Replicas,
		Reason:         req.Reason,
		Applied:        true,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"changed":      changed,
		"old_replicas": currentReplicas,
		"new_replicas": req.Replicas,
	})
}

// ── /api/dry-run/* ────────────────────────────────────────────────────────────

func (s *Server) handleDryRunPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	pending := s.dryRunQueue.GetPending()
	if pending == nil {
		pending = []DryRunDecision{}
	}
	writeJSON(w, http.StatusOK, pending)
}

// DryRunActionRequest is the body for approve/reject endpoints.
type DryRunActionRequest struct {
	ID          string `json:"id"`
	ApplyCPU    *bool  `json:"apply_cpu,omitempty"`
	ApplyMemory *bool  `json:"apply_memory,omitempty"`
}

func (s *Server) handleDryRunApprove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req DryRunActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	d, ok := s.dryRunQueue.Approve(req.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "decision not found or not pending")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch d.ScalingType {
	case "vertical":
		if s.verticalApplier == nil {
			writeError(w, http.StatusServiceUnavailable, "vertical applier not available")
			return
		}
		applyCPU := req.ApplyCPU == nil || *req.ApplyCPU
		applyMemory := req.ApplyMemory == nil || *req.ApplyMemory
		rec := &applier.ResourceRecommendation{
			Namespace:         d.Namespace,
			WorkloadKind:      d.WorkloadKind,
			WorkloadName:      d.DeploymentName,
			ContainerName:     d.ContainerName,
			CurrentCPU:        d.CurrentCPU,
			RecommendedCPU:    func() string { if applyCPU { return d.RecommendedCPU }; return d.CurrentCPU }(),
			CurrentMemory:     d.CurrentMemory,
			RecommendedMemory: func() string { if applyMemory { return d.RecommendedMemory }; return d.CurrentMemory }(),
		}
		result, err := s.verticalApplier.Apply(ctx, rec, false)
		if err != nil || !result.Applied {
			s.dryRunQueue.RevertToPending(req.ID)
			msg := "vertical scale failed"
			if err != nil {
				msg += ": " + err.Error()
			}
			klog.Warningf("dry-run approve: %s for %s/%s", msg, d.Namespace, d.DeploymentName)
			writeError(w, http.StatusInternalServerError, msg)
			return
		}
		if s.scalingHistory != nil {
			s.scalingHistory.Add(ScalingRecord{
				Namespace:       d.Namespace,
				DeploymentName:  d.DeploymentName,
				ContainerName:   d.ContainerName,
				Reason:          d.Reason + " (approved)",
				Applied:         true,
				OldCPU:          d.CurrentCPU,
				NewCPU:          d.RecommendedCPU,
				OldMemory:       d.CurrentMemory,
				NewMemory:       d.RecommendedMemory,
				Confidence:      d.Confidence,
				SavingsPerHour:  d.SavingsPerHour,
				SavingsPerMonth: d.SavingsPerMonth,
			})
		}
	default: // "horizontal" or legacy
		if s.mlScaler != nil {
			if _, err := s.mlScaler.Scale(ctx, d.Namespace, d.DeploymentName, d.Decision); err != nil {
				klog.Warningf("dry-run approve: scale failed for %s/%s: %v", d.Namespace, d.DeploymentName, err)
				writeError(w, http.StatusInternalServerError, "scale failed: "+err.Error())
				return
			}
			if s.scalingHistory != nil {
				s.scalingHistory.Add(ScalingRecord{
					Namespace:      d.Namespace,
					DeploymentName: d.DeploymentName,
					OldReplicas:    d.CurrentReplicas,
					NewReplicas:    d.DesiredReplicas,
					Reason:         d.Reason + " (approved)",
					PeakCPU:        d.Decision.PeakCPU,
					SustainedCPU:   d.Decision.SustainedCPU,
					Applied:        true,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDryRunReject(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req DryRunActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.ID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	d, ok := s.dryRunQueue.Reject(req.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "decision not found or not pending")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// ── /api/metrics-history ──────────────────────────────────────────────────────

// MetricsDataPoint is a single time-series point for the resource chart.
type MetricsDataPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	UsageCPU      int64     `json:"usage_cpu"`
	RequestCPU    int64     `json:"request_cpu"`
	UsageMemory   int64     `json:"usage_memory"`
	RequestMemory int64     `json:"request_memory"`
}

func (s *Server) handleMetricsHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	namespace := r.URL.Query().Get("namespace")
	deployment := r.URL.Query().Get("deployment")
	if namespace == "" || deployment == "" {
		writeError(w, http.StatusBadRequest, "namespace and deployment query params required")
		return
	}

	metrics := s.metricsStorage.GetMetricsByWorkload(namespace, deployment, 30*24*time.Hour)

	type bucket struct {
		cpuUsageSum   int64
		cpuRequestSum int64
		memUsageSum   int64
		memRequestSum int64
		count         int64
	}
	buckets := make(map[time.Time]*bucket)

	for _, pm := range metrics {
		t := pm.Timestamp.Truncate(30 * time.Second)
		if _, ok := buckets[t]; !ok {
			buckets[t] = &bucket{}
		}
		for _, cm := range pm.Containers {
			buckets[t].cpuUsageSum += cm.UsageCPU
			buckets[t].cpuRequestSum += cm.RequestCPU
			buckets[t].memUsageSum += cm.UsageMemory
			buckets[t].memRequestSum += cm.RequestMemory
			buckets[t].count++
		}
	}

	points := make([]MetricsDataPoint, 0, len(buckets))
	for t, b := range buckets {
		if b.count == 0 {
			continue
		}
		points = append(points, MetricsDataPoint{
			Timestamp:     t,
			UsageCPU:      b.cpuUsageSum / b.count,
			RequestCPU:    b.cpuRequestSum / b.count,
			UsageMemory:   b.memUsageSum / b.count,
			RequestMemory: b.memRequestSum / b.count,
		})
	}

	for i := 1; i < len(points); i++ {
		for j := i; j > 0 && points[j].Timestamp.Before(points[j-1].Timestamp); j-- {
			points[j], points[j-1] = points[j-1], points[j]
		}
	}

	writeJSON(w, http.StatusOK, points)
}
