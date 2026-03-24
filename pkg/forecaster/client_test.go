package forecaster_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"go.uber.org/zap/zaptest"

	"intelligent-cluster-optimizer/pkg/forecaster"
)

// ── helpers ────────────────────────────────────────────────────────────────────

func flatForecast(n int, median, high float64) []forecaster.ForecastPoint {
	pts := make([]forecaster.ForecastPoint, n)
	for i := range pts {
		pts[i] = forecaster.ForecastPoint{
			Step: i + 1, Low: median * 0.8, Median: median, High: high,
		}
	}
	return pts
}

func mockServer(t *testing.T, code int, body any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(body)
	}))
}

func cpuSamples(n int, val float64) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = val
	}
	return s
}

func newDeployment(ns, name string, replicas int32) *appsv1.Deployment {
	r := replicas
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       appsv1.DeploymentSpec{Replicas: &r},
	}
}

// ── ForecastClient.Predict ─────────────────────────────────────────────────────

func TestPredict_OK(t *testing.T) {
	want := forecaster.PredictResponse{
		Forecast:         flatForecast(15, 0.45, 0.60),
		ContextLength:    60,
		PredictionLength: 15,
		InferenceMs:      38.2,
	}
	srv := mockServer(t, http.StatusOK, want)
	defer srv.Close()

	c   := forecaster.NewForecastClient(srv.URL, 5*time.Second, zaptest.NewLogger(t))
	got, err := c.Predict(context.Background(), cpuSamples(60, 0.4))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Forecast) != 15 {
		t.Errorf("want 15 forecast points, got %d", len(got.Forecast))
	}
	if got.InferenceMs != 38.2 {
		t.Errorf("unexpected InferenceMs: %v", got.InferenceMs)
	}
}

func TestPredict_TooFewSamples(t *testing.T) {
	c := forecaster.NewForecastClient("http://unused", 5*time.Second, zaptest.NewLogger(t))
	_, err := c.Predict(context.Background(), cpuSamples(10, 0.5))
	if err == nil {
		t.Fatal("expected error for <30 samples")
	}
}

func TestPredict_ServerError(t *testing.T) {
	srv := mockServer(t, http.StatusServiceUnavailable, map[string]string{"detail": "model not loaded"})
	defer srv.Close()

	c := forecaster.NewForecastClient(srv.URL, 5*time.Second, zaptest.NewLogger(t))
	_, err := c.Predict(context.Background(), cpuSamples(60, 0.4))
	if err == nil {
		t.Fatal("expected error for 503 response")
	}
}

func TestPredict_Timeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer slow.Close()

	c := forecaster.NewForecastClient(slow.URL, 50*time.Millisecond, zaptest.NewLogger(t))
	_, err := c.Predict(context.Background(), cpuSamples(60, 0.4))
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// ── Decide ─────────────────────────────────────────────────────────────────────

var defaultCfg = forecaster.ScalerConfig{
	ScaleUpThreshold:   0.75,
	ScaleDownThreshold: 0.30,
	CPUPerReplica:      0.25,
	MinReplicas:        1,
	MaxReplicas:        10,
}

func TestDecide_ScaleUp(t *testing.T) {
	d := forecaster.Decide(flatForecast(15, 0.70, 0.85), 2, defaultCfg)
	if !d.ScaleUp {
		t.Error("expected ScaleUp=true (p90=0.85 > 0.75)")
	}
	if d.ScaleDown {
		t.Error("expected ScaleDown=false")
	}
	if d.DesiredReplicas <= 2 {
		t.Errorf("expected more than 2 replicas, got %d", d.DesiredReplicas)
	}
}

func TestDecide_ScaleDown(t *testing.T) {
	d := forecaster.Decide(flatForecast(15, 0.15, 0.20), 5, defaultCfg)
	if !d.ScaleDown {
		t.Error("expected ScaleDown=true (p50=0.15 < 0.30)")
	}
	if d.DesiredReplicas >= 5 {
		t.Errorf("expected fewer than 5 replicas, got %d", d.DesiredReplicas)
	}
}

func TestDecide_NoChange(t *testing.T) {
	d := forecaster.Decide(flatForecast(15, 0.50, 0.65), 3, defaultCfg)
	if d.ScaleUp || d.ScaleDown {
		t.Errorf("expected no action, got ScaleUp=%v ScaleDown=%v", d.ScaleUp, d.ScaleDown)
	}
	if d.DesiredReplicas != 3 {
		t.Errorf("expected 3 replicas, got %d", d.DesiredReplicas)
	}
}

func TestDecide_CapsAtMaxReplicas(t *testing.T) {
	d := forecaster.Decide(flatForecast(15, 0.99, 0.99), 1, defaultCfg)
	if d.DesiredReplicas > 10 {
		t.Errorf("expected ≤ 10 replicas, got %d", d.DesiredReplicas)
	}
}

func TestDecide_FloorAtMinReplicas(t *testing.T) {
	cfg := defaultCfg
	cfg.MinReplicas = 2
	d := forecaster.Decide(flatForecast(15, 0.01, 0.02), 5, cfg)
	if d.DesiredReplicas < 2 {
		t.Errorf("expected ≥ 2 replicas, got %d", d.DesiredReplicas)
	}
}

func TestDecide_EmptyForecast(t *testing.T) {
	d := forecaster.Decide(nil, 3, defaultCfg)
	if d.DesiredReplicas != 3 {
		t.Errorf("expected no-op (3), got %d", d.DesiredReplicas)
	}
}

// ── HorizontalScaler ───────────────────────────────────────────────────────────

func TestScale_Up(t *testing.T) {
	kube := fake.NewSimpleClientset(newDeployment("default", "app", 2))
	s    := forecaster.NewHorizontalScaler(kube, zaptest.NewLogger(t))

	changed, err := s.Scale(context.Background(), "default", "app",
		forecaster.ScaleDecision{ScaleUp: true, DesiredReplicas: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	dep, _ := kube.AppsV1().Deployments("default").Get(context.Background(), "app", metav1.GetOptions{})
	if *dep.Spec.Replicas != 5 {
		t.Errorf("expected 5 replicas, got %d", *dep.Spec.Replicas)
	}
}

func TestScale_NoOp(t *testing.T) {
	kube := fake.NewSimpleClientset(newDeployment("default", "app", 3))
	s    := forecaster.NewHorizontalScaler(kube, zaptest.NewLogger(t))

	changed, err := s.Scale(context.Background(), "default", "app",
		forecaster.ScaleDecision{DesiredReplicas: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false when replica count unchanged")
	}
}

func TestScale_DeploymentNotFound(t *testing.T) {
	kube := fake.NewSimpleClientset()
	s    := forecaster.NewHorizontalScaler(kube, zaptest.NewLogger(t))

	_, err := s.Scale(context.Background(), "default", "missing",
		forecaster.ScaleDecision{DesiredReplicas: 3})
	if err == nil {
		t.Fatal("expected error for missing deployment")
	}
}
