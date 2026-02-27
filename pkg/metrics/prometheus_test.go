package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestPrometheusExporter_RecordReconciliation(t *testing.T) {
	// Create a new registry for isolated testing
	registry := prometheus.NewRegistry()

	// Create exporter with custom registry
	exporter := &PrometheusExporter{
		ReconciliationTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "reconciliation_total",
				Help:      "Test metric",
			},
			[]string{"config", "namespace", "result"},
		),
		ReconciliationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "test",
				Name:      "reconciliation_duration_seconds",
				Help:      "Test metric",
			},
			[]string{"config", "namespace"},
		),
	}

	registry.MustRegister(exporter.ReconciliationTotal)
	registry.MustRegister(exporter.ReconciliationDuration)

	// Record a reconciliation
	exporter.RecordReconciliation("test-config", "default", "success", 1.5)

	// Verify the counter was incremented
	count := testutil.ToFloat64(exporter.ReconciliationTotal.WithLabelValues("test-config", "default", "success"))
	if count != 1.0 {
		t.Errorf("Expected count 1.0, got %f", count)
	}

	// Record another
	exporter.RecordReconciliation("test-config", "default", "success", 2.0)
	count = testutil.ToFloat64(exporter.ReconciliationTotal.WithLabelValues("test-config", "default", "success"))
	if count != 2.0 {
		t.Errorf("Expected count 2.0, got %f", count)
	}
}

func TestPrometheusExporter_RecordSLAViolation(t *testing.T) {
	registry := prometheus.NewRegistry()

	exporter := &PrometheusExporter{
		SLAViolations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "sla_violations_total",
				Help:      "Test metric",
			},
			[]string{"config", "namespace", "sla_type"},
		),
	}

	registry.MustRegister(exporter.SLAViolations)

	// Record violations
	exporter.RecordSLAViolation("test-config", "default", "latency")
	exporter.RecordSLAViolation("test-config", "default", "latency")
	exporter.RecordSLAViolation("test-config", "default", "error_rate")

	// Verify counts
	latencyCount := testutil.ToFloat64(exporter.SLAViolations.WithLabelValues("test-config", "default", "latency"))
	if latencyCount != 2.0 {
		t.Errorf("Expected latency violations 2.0, got %f", latencyCount)
	}

	errorRateCount := testutil.ToFloat64(exporter.SLAViolations.WithLabelValues("test-config", "default", "error_rate"))
	if errorRateCount != 1.0 {
		t.Errorf("Expected error_rate violations 1.0, got %f", errorRateCount)
	}
}

func TestPrometheusExporter_RecordHealthScore(t *testing.T) {
	registry := prometheus.NewRegistry()

	exporter := &PrometheusExporter{
		HealthScore: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "health_score",
				Help:      "Test metric",
			},
			[]string{"config", "namespace"},
		),
	}

	registry.MustRegister(exporter.HealthScore)

	// Record health score
	exporter.RecordHealthScore("test-config", "default", 85.5)

	// Verify gauge value
	score := testutil.ToFloat64(exporter.HealthScore.WithLabelValues("test-config", "default"))
	if score != 85.5 {
		t.Errorf("Expected health score 85.5, got %f", score)
	}

	// Update score
	exporter.RecordHealthScore("test-config", "default", 92.0)
	score = testutil.ToFloat64(exporter.HealthScore.WithLabelValues("test-config", "default"))
	if score != 92.0 {
		t.Errorf("Expected health score 92.0, got %f", score)
	}
}

func TestPrometheusExporter_RecordResourceRecommendations(t *testing.T) {
	registry := prometheus.NewRegistry()

	exporter := &PrometheusExporter{
		CPURecommendation: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "cpu_recommendation_millicores",
				Help:      "Test metric",
			},
			[]string{"workload", "namespace", "container"},
		),
		MemoryRecommendation: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "memory_recommendation_mib",
				Help:      "Test metric",
			},
			[]string{"workload", "namespace", "container"},
		),
		ReplicaRecommendation: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "replica_recommendation",
				Help:      "Test metric",
			},
			[]string{"workload", "namespace"},
		),
	}

	registry.MustRegister(exporter.CPURecommendation)
	registry.MustRegister(exporter.MemoryRecommendation)
	registry.MustRegister(exporter.ReplicaRecommendation)

	// Record recommendations
	exporter.RecordCPURecommendation("nginx", "default", "app", 500.0)
	exporter.RecordMemoryRecommendation("nginx", "default", "app", 256.0)
	exporter.RecordReplicaRecommendation("nginx", "default", 3.0)

	// Verify values
	cpu := testutil.ToFloat64(exporter.CPURecommendation.WithLabelValues("nginx", "default", "app"))
	if cpu != 500.0 {
		t.Errorf("Expected CPU 500.0, got %f", cpu)
	}

	memory := testutil.ToFloat64(exporter.MemoryRecommendation.WithLabelValues("nginx", "default", "app"))
	if memory != 256.0 {
		t.Errorf("Expected memory 256.0, got %f", memory)
	}

	replicas := testutil.ToFloat64(exporter.ReplicaRecommendation.WithLabelValues("nginx", "default"))
	if replicas != 3.0 {
		t.Errorf("Expected replicas 3.0, got %f", replicas)
	}
}

func TestPrometheusExporter_RecordPolicyMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()

	exporter := &PrometheusExporter{
		PolicyEvaluations: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "policy_evaluations_total",
				Help:      "Test metric",
			},
			[]string{"config", "namespace", "policy_name", "result"},
		),
		PolicyBlockedChanges: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "policy_blocked_changes_total",
				Help:      "Test metric",
			},
			[]string{"config", "namespace", "policy_name"},
		),
	}

	registry.MustRegister(exporter.PolicyEvaluations)
	registry.MustRegister(exporter.PolicyBlockedChanges)

	// Record policy evaluations
	exporter.RecordPolicyEvaluation("test-config", "default", "min-cpu", "allowed")
	exporter.RecordPolicyEvaluation("test-config", "default", "max-cpu", "blocked")
	exporter.RecordPolicyBlockedChange("test-config", "default", "max-cpu")

	// Verify counts
	allowedCount := testutil.ToFloat64(exporter.PolicyEvaluations.WithLabelValues("test-config", "default", "min-cpu", "allowed"))
	if allowedCount != 1.0 {
		t.Errorf("Expected allowed count 1.0, got %f", allowedCount)
	}

	blockedCount := testutil.ToFloat64(exporter.PolicyBlockedChanges.WithLabelValues("test-config", "default", "max-cpu"))
	if blockedCount != 1.0 {
		t.Errorf("Expected blocked count 1.0, got %f", blockedCount)
	}
}

func TestPrometheusExporter_RecordGitOpsMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()

	exporter := &PrometheusExporter{
		GitOpsExports: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "gitops_exports_total",
				Help:      "Test metric",
			},
			[]string{"config", "namespace", "format"},
		),
		GitOpsExportErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "gitops_export_errors_total",
				Help:      "Test metric",
			},
			[]string{"config", "namespace", "format"},
		),
	}

	registry.MustRegister(exporter.GitOpsExports)
	registry.MustRegister(exporter.GitOpsExportErrors)

	// Record exports
	exporter.RecordGitOpsExport("test-config", "default", "kustomize")
	exporter.RecordGitOpsExport("test-config", "default", "helm")
	exporter.RecordGitOpsExportError("test-config", "default", "kustomize")

	// Verify counts
	kustomizeCount := testutil.ToFloat64(exporter.GitOpsExports.WithLabelValues("test-config", "default", "kustomize"))
	if kustomizeCount != 1.0 {
		t.Errorf("Expected kustomize exports 1.0, got %f", kustomizeCount)
	}

	errorCount := testutil.ToFloat64(exporter.GitOpsExportErrors.WithLabelValues("test-config", "default", "kustomize"))
	if errorCount != 1.0 {
		t.Errorf("Expected kustomize errors 1.0, got %f", errorCount)
	}
}

func TestPrometheusExporter_RecordParetoMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()

	exporter := &PrometheusExporter{
		ParetoFrontSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "pareto_front_size",
				Help:      "Test metric",
			},
			[]string{"config", "namespace"},
		),
		ParetoSolutions: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "pareto_solutions_total",
				Help:      "Test metric",
			},
			[]string{"config", "namespace"},
		),
	}

	registry.MustRegister(exporter.ParetoFrontSize)
	registry.MustRegister(exporter.ParetoSolutions)

	// Record Pareto metrics
	exporter.RecordParetoFrontSize("test-config", "default", 5)
	exporter.RecordParetoSolution("test-config", "default")
	exporter.RecordParetoSolution("test-config", "default")

	// Verify values
	frontSize := testutil.ToFloat64(exporter.ParetoFrontSize.WithLabelValues("test-config", "default"))
	if frontSize != 5.0 {
		t.Errorf("Expected Pareto front size 5.0, got %f", frontSize)
	}

	solutionCount := testutil.ToFloat64(exporter.ParetoSolutions.WithLabelValues("test-config", "default"))
	if solutionCount != 2.0 {
		t.Errorf("Expected Pareto solutions 2.0, got %f", solutionCount)
	}
}

func TestNewPrometheusExporter(t *testing.T) {
	// Test creating a new exporter
	exporter := NewPrometheusExporter("test_optimizer")

	if exporter == nil {
		t.Fatal("Expected non-nil exporter")
		return
	}

	// Verify all metrics are initialized
	if exporter.ReconciliationTotal == nil {
		t.Error("ReconciliationTotal not initialized")
	}
	if exporter.CPURecommendation == nil {
		t.Error("CPURecommendation not initialized")
	}
	if exporter.HealthScore == nil {
		t.Error("HealthScore not initialized")
	}
	if exporter.PolicyEvaluations == nil {
		t.Error("PolicyEvaluations not initialized")
	}
	if exporter.GitOpsExports == nil {
		t.Error("GitOpsExports not initialized")
	}
	if exporter.ParetoFrontSize == nil {
		t.Error("ParetoFrontSize not initialized")
	}
	if exporter.PodStartupDuration == nil {
		t.Error("PodStartupDuration not initialized")
	}
	if exporter.PodStartupDurationAvg == nil {
		t.Error("PodStartupDurationAvg not initialized")
	}
	if exporter.PodStartupDurationP95 == nil {
		t.Error("PodStartupDurationP95 not initialized")
	}
	if exporter.SlowStartupsTotal == nil {
		t.Error("SlowStartupsTotal not initialized")
	}
}

func TestPrometheusExporter_RecordPodStartupMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()

	exporter := &PrometheusExporter{
		PodStartupDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "test",
				Name:      "pod_startup_duration_seconds",
				Help:      "Test metric",
				Buckets:   []float64{1, 2, 5, 10, 15, 20, 30, 45, 60, 90, 120},
			},
			[]string{"workload", "namespace", "container"},
		),
		PodStartupDurationAvg: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "pod_startup_duration_avg_seconds",
				Help:      "Test metric",
			},
			[]string{"workload", "namespace"},
		),
		PodStartupDurationP95: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "test",
				Name:      "pod_startup_duration_p95_seconds",
				Help:      "Test metric",
			},
			[]string{"workload", "namespace"},
		),
		SlowStartupsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "test",
				Name:      "slow_startups_total",
				Help:      "Test metric",
			},
			[]string{"workload", "namespace", "threshold"},
		),
	}

	registry.MustRegister(exporter.PodStartupDuration)
	registry.MustRegister(exporter.PodStartupDurationAvg)
	registry.MustRegister(exporter.PodStartupDurationP95)
	registry.MustRegister(exporter.SlowStartupsTotal)

	// Record startup times
	exporter.RecordPodStartupDuration("nginx", "default", "app", 12.5)
	exporter.RecordPodStartupDuration("nginx", "default", "app", 8.0)
	exporter.RecordPodStartupDuration("nginx", "default", "app", 25.0)

	// Record average and P95
	exporter.RecordPodStartupDurationAvg("nginx", "default", 15.17)
	exporter.RecordPodStartupDurationP95("nginx", "default", 24.0)

	// Record slow startups
	exporter.RecordSlowStartup("nginx", "default", 20.0)

	// Verify average gauge
	avg := testutil.ToFloat64(exporter.PodStartupDurationAvg.WithLabelValues("nginx", "default"))
	if avg != 15.17 {
		t.Errorf("Expected average 15.17, got %f", avg)
	}

	// Verify P95 gauge
	p95 := testutil.ToFloat64(exporter.PodStartupDurationP95.WithLabelValues("nginx", "default"))
	if p95 != 24.0 {
		t.Errorf("Expected P95 24.0, got %f", p95)
	}

	// Verify slow startup counter
	slow := testutil.ToFloat64(exporter.SlowStartupsTotal.WithLabelValues("nginx", "default", "20s"))
	if slow != 1.0 {
		t.Errorf("Expected slow startups 1.0, got %f", slow)
	}
}
