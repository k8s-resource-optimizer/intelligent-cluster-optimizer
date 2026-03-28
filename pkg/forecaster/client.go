// Package forecaster provides a Go client for the Chronos-2 FastAPI forecasting
// service and a scaler that drives horizontal pod autoscaling based on predicted
// CPU load.
//
// CPU values exchanged with the FastAPI service are in 0-1 range
// (matching Azra's data pipeline — NOT percentages).
//
// Flow:
//
//	MetricsStore → ForecastClient.Predict → Decide → HorizontalScaler.Scale
package forecaster

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ── HTTP client ────────────────────────────────────────────────────────────────

// ForecastClient calls the FastAPI /predict endpoint.
type ForecastClient struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewForecastClient constructs a ForecastClient.
// baseURL example: "http://ml-service.ml-system.svc.cluster.local:8080"
func NewForecastClient(baseURL string, timeout time.Duration, logger *zap.Logger) *ForecastClient {
	return &ForecastClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger,
	}
}

// PredictRequest mirrors the FastAPI PredictRequest schema.
// CPUValues must be in 0-1 range.
type PredictRequest struct {
	CPUValues  []float64 `json:"cpu_values"`
	NumSamples int       `json:"num_samples,omitempty"`
}

// ForecastPoint is a single future step.
type ForecastPoint struct {
	Step   int     `json:"step"`
	Low    float64 `json:"low"`    // p10
	Median float64 `json:"median"` // p50
	High   float64 `json:"high"`   // p90
}

// PredictResponse mirrors the FastAPI PredictResponse schema.
type PredictResponse struct {
	Forecast         []ForecastPoint `json:"forecast"`
	ContextLength    int             `json:"context_length"`
	PredictionLength int             `json:"prediction_length"`
	InferenceMs      float64         `json:"inference_ms"`
}

// Predict sends recent CPU samples (0-1 range) to /predict and returns the forecast.
func (c *ForecastClient) Predict(ctx context.Context, cpuValues []float64) (*PredictResponse, error) {
	if len(cpuValues) < 30 {
		return nil, fmt.Errorf("forecaster: need at least 30 CPU samples, got %d", len(cpuValues))
	}

	body, err := json.Marshal(PredictRequest{CPUValues: cpuValues, NumSamples: 20})
	if err != nil {
		return nil, fmt.Errorf("forecaster: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/predict", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("forecaster: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("forecaster: http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("forecaster: /predict returned %d: %s", resp.StatusCode, raw)
	}

	var pr PredictResponse
	if err := json.NewDecoder(resp.Body).Decode(&pr); err != nil {
		return nil, fmt.Errorf("forecaster: decode: %w", err)
	}

	c.logger.Info("forecast received",
		zap.Int("context_len", pr.ContextLength),
		zap.Int("pred_len", pr.PredictionLength),
		zap.Float64("inference_ms", pr.InferenceMs),
	)
	return &pr, nil
}

// ── Scale decision ─────────────────────────────────────────────────────────────

// ScaleDecision is the autoscaling recommendation derived from a forecast.
type ScaleDecision struct {
	ScaleUp         bool
	ScaleDown       bool
	DesiredReplicas int32
	PeakCPU         float64 // max p90 across forecast steps (0-1)
	SustainedCPU    float64 // mean p50 across forecast steps (0-1)
}

// ScalerConfig holds thresholds. All CPU values are in 0-1 range.
type ScalerConfig struct {
	ScaleUpThreshold   float64 // scale up when p90 peak > this.  Default 0.75
	ScaleDownThreshold float64 // scale down when p50 mean < this. Default 0.30
	CPUPerReplica      float64 // assumed load per replica (0-1).  Default 0.25
	MinReplicas        int32   // default 1
	MaxReplicas        int32   // default 10
}

func (c ScalerConfig) withDefaults() ScalerConfig {
	if c.ScaleUpThreshold == 0 {
		c.ScaleUpThreshold = 0.75
	}
	if c.ScaleDownThreshold == 0 {
		c.ScaleDownThreshold = 0.30
	}
	if c.CPUPerReplica == 0 {
		c.CPUPerReplica = 0.25
	}
	if c.MinReplicas == 0 {
		c.MinReplicas = 1
	}
	if c.MaxReplicas == 0 {
		c.MaxReplicas = 10
	}
	return c
}

// Decide converts a forecast into a scaling recommendation.
func Decide(forecast []ForecastPoint, currentReplicas int32, cfg ScalerConfig) ScaleDecision {
	cfg = cfg.withDefaults()
	if len(forecast) == 0 {
		return ScaleDecision{DesiredReplicas: currentReplicas}
	}

	var peakHigh, sumMedian float64
	for _, pt := range forecast {
		if pt.High > peakHigh {
			peakHigh = pt.High
		}
		sumMedian += pt.Median
	}
	sustained := sumMedian / float64(len(forecast))

	scaleUp := peakHigh > cfg.ScaleUpThreshold
	scaleDown := !scaleUp && sustained < cfg.ScaleDownThreshold

	var desired int32
	switch {
	case scaleUp:
		desired = int32(peakHigh/cfg.CPUPerReplica) + 1
	case scaleDown:
		desired = int32(sustained/cfg.CPUPerReplica) + 1
	default:
		desired = currentReplicas
	}

	if desired < cfg.MinReplicas {
		desired = cfg.MinReplicas
	}
	if desired > cfg.MaxReplicas {
		desired = cfg.MaxReplicas
	}

	return ScaleDecision{
		ScaleUp:         scaleUp,
		ScaleDown:       scaleDown,
		DesiredReplicas: desired,
		PeakCPU:         peakHigh,
		SustainedCPU:    sustained,
	}
}

// ── Horizontal scaler ──────────────────────────────────────────────────────────

// HorizontalScaler applies scale decisions to Kubernetes Deployments.
type HorizontalScaler struct {
	kubeClient kubernetes.Interface
	logger     *zap.Logger
}

// NewHorizontalScaler constructs a HorizontalScaler.
func NewHorizontalScaler(kubeClient kubernetes.Interface, logger *zap.Logger) *HorizontalScaler {
	return &HorizontalScaler{kubeClient: kubeClient, logger: logger}
}

// Scale sets the Deployment replica count. Returns (changed, error).
func (s *HorizontalScaler) Scale(
	ctx context.Context,
	namespace, name string,
	d ScaleDecision,
) (bool, error) {
	dep, err := s.kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return false, fmt.Errorf("forecaster: deployment %s/%s not found", namespace, name)
		}
		return false, fmt.Errorf("forecaster: get deployment: %w", err)
	}

	current := int32(1)
	if dep.Spec.Replicas != nil {
		current = *dep.Spec.Replicas
	}
	if d.DesiredReplicas == current {
		s.logger.Info("no scale needed",
			zap.String("deployment", name),
			zap.Int32("replicas", current),
		)
		return false, nil
	}

	dep.Spec.Replicas = &d.DesiredReplicas
	if _, err = s.kubeClient.AppsV1().Deployments(namespace).Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
		return false, fmt.Errorf("forecaster: update deployment: %w", err)
	}

	dir := "up"
	if d.DesiredReplicas < current {
		dir = "down"
	}
	s.logger.Info("scaled",
		zap.String("direction", dir),
		zap.String("deployment", name),
		zap.Int32("from", current),
		zap.Int32("to", d.DesiredReplicas),
		zap.Float64("peak_cpu_p90", d.PeakCPU),
	)
	return true, nil
}

// ── Reconciler ─────────────────────────────────────────────────────────────────

// Reconciler ties together the forecast client and horizontal scaler.
type Reconciler struct {
	forecast *ForecastClient
	scaler   *HorizontalScaler
	cfg      ScalerConfig
	logger   *zap.Logger
}

// NewReconciler builds a Reconciler.
func NewReconciler(fc *ForecastClient, hs *HorizontalScaler, cfg ScalerConfig, logger *zap.Logger) *Reconciler {
	return &Reconciler{forecast: fc, scaler: hs, cfg: cfg, logger: logger}
}

// Run fetches a forecast for cpuHistory (0-1 values) and applies horizontal scaling.
func (r *Reconciler) Run(
	ctx context.Context,
	namespace, deployment string,
	cpuHistory []float64,
	currentReplicas int32,
) error {
	pr, err := r.forecast.Predict(ctx, cpuHistory)
	if err != nil {
		return fmt.Errorf("forecaster reconciler predict: %w", err)
	}

	d := Decide(pr.Forecast, currentReplicas, r.cfg)
	r.logger.Info("decision",
		zap.String("deployment", deployment),
		zap.Bool("scale_up", d.ScaleUp),
		zap.Bool("scale_down", d.ScaleDown),
		zap.Int32("desired", d.DesiredReplicas),
		zap.Float64("peak_p90", d.PeakCPU),
	)

	_, err = r.scaler.Scale(ctx, namespace, deployment, d)
	return err
}
