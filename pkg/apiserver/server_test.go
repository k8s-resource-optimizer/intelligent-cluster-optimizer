package apiserver_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"intelligent-cluster-optimizer/pkg/apiserver"
	"intelligent-cluster-optimizer/pkg/forecaster"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// newMinimalServer creates a Server with only in-memory stores wired up and a
// fake Kubernetes client. mlScaler and optimizerClient are nil so handlers that
// require them fall through to their nil-guard paths.
func newMinimalServer() *apiserver.Server {
	kubeClient := fake.NewSimpleClientset()
	return apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     kubeClient,
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})
}

func postJSON(srv *apiserver.Server, path string, body any) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func getReq(srv *apiserver.Server, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

// ── /health ───────────────────────────────────────────────────────────────────

func TestHandleHealth_Returns200(t *testing.T) {
	srv := newMinimalServer()
	w := getReq(srv, "/health")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandleHealth_ReturnsOKBody(t *testing.T) {
	srv := newMinimalServer()
	w := getReq(srv, "/health")
	var m map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", m["status"])
	}
}

func TestHandleHealth_ContentTypeJSON(t *testing.T) {
	srv := newMinimalServer()
	w := getReq(srv, "/health")
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected application/json, got %q", ct)
	}
}

// ── CORS middleware ───────────────────────────────────────────────────────────

func TestCORSPreflight_Returns204(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS, got %d", w.Code)
	}
}

func TestCORSPreflight_SetsAllowOrigin(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	origin := w.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:3000" {
		t.Errorf("expected origin reflected, got %q", origin)
	}
}

func TestCORSPreflight_SetsAllowMethods(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodOptions, "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	methods := w.Header().Get("Access-Control-Allow-Methods")
	if methods == "" {
		t.Error("expected Access-Control-Allow-Methods to be set")
	}
}

func TestCORSHeaders_SetOnNormalRequest(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Error("CORS origin header missing on normal GET")
	}
}

// ── /api/forecasts ────────────────────────────────────────────────────────────

func TestHandleForecasts_EmptyReturnsArray(t *testing.T) {
	srv := newMinimalServer()
	w := getReq(srv, "/api/forecasts")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var entries []apiserver.ForecastEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestHandleForecasts_ReturnsPopulatedEntries(t *testing.T) {
	fc := apiserver.NewForecastCache()
	fc.Set("default", "web", apiserver.ForecastEntry{
		DeploymentName: "web",
		CPUSamples:     10,
	})
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  fc,
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})

	w := getReq(srv, "/api/forecasts")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var entries []apiserver.ForecastEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].DeploymentName != "web" {
		t.Errorf("unexpected deployment: %s", entries[0].DeploymentName)
	}
}

func TestHandleForecasts_MethodNotAllowed(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodPost, "/api/forecasts", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleForecasts_WithDecision(t *testing.T) {
	fc := apiserver.NewForecastCache()
	d := forecaster.ScaleDecision{ScaleUp: true, DesiredReplicas: 5, PeakCPU: 0.9}
	fc.Set("default", "api", apiserver.ForecastEntry{
		DeploymentName: "api",
		Decision:       &d,
	})
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  fc,
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})
	w := getReq(srv, "/api/forecasts")
	var entries []apiserver.ForecastEntry
	_ = json.Unmarshal(w.Body.Bytes(), &entries)
	if entries[0].Decision == nil {
		t.Fatal("expected decision to be present")
	}
	if entries[0].Decision.DesiredReplicas != 5 {
		t.Errorf("expected DesiredReplicas=5, got %d", entries[0].Decision.DesiredReplicas)
	}
}

// ── /api/scaling-history ──────────────────────────────────────────────────────

func TestHandleScalingHistory_EmptyReturnsArray(t *testing.T) {
	srv := newMinimalServer()
	w := getReq(srv, "/api/scaling-history")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var records []apiserver.ScalingRecord
	if err := json.Unmarshal(w.Body.Bytes(), &records); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestHandleScalingHistory_ReturnsRecords(t *testing.T) {
	sh := apiserver.NewScalingHistoryStore(100)
	sh.Add(apiserver.ScalingRecord{DeploymentName: "web", OldReplicas: 2, NewReplicas: 4})
	sh.Add(apiserver.ScalingRecord{DeploymentName: "api", OldReplicas: 1, NewReplicas: 3})
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: sh,
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})
	w := getReq(srv, "/api/scaling-history")
	var records []apiserver.ScalingRecord
	if err := json.Unmarshal(w.Body.Bytes(), &records); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	// newest first: api was added last
	if records[0].DeploymentName != "api" {
		t.Errorf("expected newest first (api), got %s", records[0].DeploymentName)
	}
}

func TestHandleScalingHistory_MethodNotAllowed(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodDelete, "/api/scaling-history", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ── /api/pods ─────────────────────────────────────────────────────────────────

func TestHandlePods_EmptyCluster(t *testing.T) {
	srv := newMinimalServer()
	w := getReq(srv, "/api/pods")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var pods []apiserver.PodInfo
	if err := json.Unmarshal(w.Body.Bytes(), &pods); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(pods) != 0 {
		t.Errorf("expected 0 pods, got %d", len(pods))
	}
}

func TestHandlePods_ReturnsFakePods(t *testing.T) {
	kubeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-abc-xyz",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-def-uvw",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	)
	srv := apiserver.NewServer(apiserver.Config{
		Addr:           ":0",
		Namespace:      "default",
		KubeClient:     kubeClient,
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})

	w := getReq(srv, "/api/pods")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var pods []apiserver.PodInfo
	if err := json.Unmarshal(w.Body.Bytes(), &pods); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods, got %d", len(pods))
	}
}

func TestHandlePods_PodStatusRunning(t *testing.T) {
	kubeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-1-a", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	srv := apiserver.NewServer(apiserver.Config{
		Addr: ":0", Namespace: "default",
		KubeClient:     kubeClient,
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})

	w := getReq(srv, "/api/pods")
	var pods []apiserver.PodInfo
	_ = json.Unmarshal(w.Body.Bytes(), &pods)
	if pods[0].Status != "Running" {
		t.Errorf("expected status Running, got %s", pods[0].Status)
	}
}

func TestHandlePods_NamespaceQueryParam(t *testing.T) {
	kubeClient := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-x-y", Namespace: "staging"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	srv := apiserver.NewServer(apiserver.Config{
		Addr: ":0", Namespace: "default",
		KubeClient:     kubeClient,
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    apiserver.NewDryRunQueue(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/pods?namespace=staging", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	var pods []apiserver.PodInfo
	_ = json.Unmarshal(w.Body.Bytes(), &pods)
	if len(pods) != 1 {
		t.Fatalf("expected 1 pod in staging, got %d", len(pods))
	}
	if pods[0].Namespace != "staging" {
		t.Errorf("expected namespace=staging, got %s", pods[0].Namespace)
	}
}

func TestHandlePods_MethodNotAllowed(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodPost, "/api/pods", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ── /api/optimizer-configs ────────────────────────────────────────────────────

func TestHandleOptimizerConfigs_NilClientReturnsEmpty(t *testing.T) {
	srv := newMinimalServer() // optimizerClient is nil
	w := getReq(srv, "/api/optimizer-configs")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var summaries []apiserver.OptimizerConfigSummary
	if err := json.Unmarshal(w.Body.Bytes(), &summaries); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected empty list, got %d", len(summaries))
	}
}

func TestHandleOptimizerConfigs_MethodNotAllowed(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodPost, "/api/optimizer-configs", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ── /api/scale ────────────────────────────────────────────────────────────────

func TestHandleScale_NilScalerReturns503(t *testing.T) {
	srv := newMinimalServer() // mlScaler is nil
	w := postJSON(srv, "/api/scale", map[string]any{
		"namespace":       "default",
		"deployment_name": "web",
		"replicas":        3,
	})
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when mlScaler=nil, got %d", w.Code)
	}
}

func TestHandleScale_MissingDeploymentName(t *testing.T) {
	srv := newMinimalServer()
	w := postJSON(srv, "/api/scale", map[string]any{
		"replicas": 3,
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing deployment_name, got %d", w.Code)
	}
}

func TestHandleScale_ZeroReplicas(t *testing.T) {
	srv := newMinimalServer()
	w := postJSON(srv, "/api/scale", map[string]any{
		"deployment_name": "web",
		"replicas":        0,
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for replicas=0, got %d", w.Code)
	}
}

func TestHandleScale_InvalidJSON(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodPost, "/api/scale", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestHandleScale_MethodNotAllowed(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodGet, "/api/scale", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ── /api/dry-run/pending ──────────────────────────────────────────────────────

func TestHandleDryRunPending_Empty(t *testing.T) {
	srv := newMinimalServer()
	w := getReq(srv, "/api/dry-run/pending")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var pending []apiserver.DryRunDecision
	if err := json.Unmarshal(w.Body.Bytes(), &pending); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending, got %d", len(pending))
	}
}

func TestHandleDryRunPending_ReturnsPendingDecisions(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	q.Add(apiserver.DryRunDecision{Namespace: "default", DeploymentName: "web", DesiredReplicas: 4})
	q.Add(apiserver.DryRunDecision{Namespace: "default", DeploymentName: "api", DesiredReplicas: 2})

	srv := apiserver.NewServer(apiserver.Config{
		Addr: ":0", Namespace: "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
	})

	w := getReq(srv, "/api/dry-run/pending")
	var pending []apiserver.DryRunDecision
	if err := json.Unmarshal(w.Body.Bytes(), &pending); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(pending))
	}
}

func TestHandleDryRunPending_ExcludesApproved(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "web"})
	q.Add(apiserver.DryRunDecision{DeploymentName: "api"})
	q.Approve(id)

	srv := apiserver.NewServer(apiserver.Config{
		Addr: ":0", Namespace: "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
	})

	w := getReq(srv, "/api/dry-run/pending")
	var pending []apiserver.DryRunDecision
	_ = json.Unmarshal(w.Body.Bytes(), &pending)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending after approve, got %d", len(pending))
	}
	if pending[0].DeploymentName != "api" {
		t.Errorf("expected api pending, got %s", pending[0].DeploymentName)
	}
}

func TestHandleDryRunPending_MethodNotAllowed(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/pending", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ── /api/dry-run/approve ─────────────────────────────────────────────────────

func TestHandleDryRunApprove_NilScalerStillApproves(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{Namespace: "default", DeploymentName: "web", DesiredReplicas: 3})

	srv := apiserver.NewServer(apiserver.Config{
		Addr: ":0", Namespace: "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
		// mlScaler is nil
	})

	w := postJSON(srv, "/api/dry-run/approve", map[string]string{"id": id})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var d apiserver.DryRunDecision
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if d.Status != apiserver.DryRunApproved {
		t.Errorf("expected approved status, got %s", d.Status)
	}
}

func TestHandleDryRunApprove_NotFound(t *testing.T) {
	srv := newMinimalServer()
	w := postJSON(srv, "/api/dry-run/approve", map[string]string{"id": "nonexistent"})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDryRunApprove_MissingID(t *testing.T) {
	srv := newMinimalServer()
	w := postJSON(srv, "/api/dry-run/approve", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing id, got %d", w.Code)
	}
}

func TestHandleDryRunApprove_InvalidJSON(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/approve", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestHandleDryRunApprove_AlreadyApprovedReturns404(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "web"})
	q.Approve(id) // first approval

	srv := apiserver.NewServer(apiserver.Config{
		Addr: ":0", Namespace: "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
	})

	w := postJSON(srv, "/api/dry-run/approve", map[string]string{"id": id})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for second approve, got %d", w.Code)
	}
}

func TestHandleDryRunApprove_MethodNotAllowed(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodGet, "/api/dry-run/approve", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ── /api/dry-run/reject ───────────────────────────────────────────────────────

func TestHandleDryRunReject_Success(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{Namespace: "default", DeploymentName: "web", DesiredReplicas: 3})

	srv := apiserver.NewServer(apiserver.Config{
		Addr: ":0", Namespace: "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
	})

	w := postJSON(srv, "/api/dry-run/reject", map[string]string{"id": id})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}
	var d apiserver.DryRunDecision
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if d.Status != apiserver.DryRunRejected {
		t.Errorf("expected rejected status, got %s", d.Status)
	}
}

func TestHandleDryRunReject_NotFound(t *testing.T) {
	srv := newMinimalServer()
	w := postJSON(srv, "/api/dry-run/reject", map[string]string{"id": "ghost"})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleDryRunReject_MissingID(t *testing.T) {
	srv := newMinimalServer()
	w := postJSON(srv, "/api/dry-run/reject", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing id, got %d", w.Code)
	}
}

func TestHandleDryRunReject_InvalidJSON(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodPost, "/api/dry-run/reject", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestHandleDryRunReject_AfterApprove(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "web"})
	q.Approve(id)

	srv := apiserver.NewServer(apiserver.Config{
		Addr: ":0", Namespace: "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: apiserver.NewScalingHistoryStore(100),
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
	})

	w := postJSON(srv, "/api/dry-run/reject", map[string]string{"id": id})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when rejecting already-approved decision, got %d", w.Code)
	}
}

func TestHandleDryRunReject_MethodNotAllowed(t *testing.T) {
	srv := newMinimalServer()
	req := httptest.NewRequest(http.MethodGet, "/api/dry-run/reject", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// ── Approve records to scaling history ───────────────────────────────────────

func TestHandleDryRunApprove_DoesNotRecordWhenScalerNil(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "web", DesiredReplicas: 3})
	sh := apiserver.NewScalingHistoryStore(100)

	srv := apiserver.NewServer(apiserver.Config{
		Addr: ":0", Namespace: "default",
		KubeClient:     fake.NewSimpleClientset(),
		ScalingHistory: sh,
		ForecastCache:  apiserver.NewForecastCache(),
		DryRunQueue:    q,
		// mlScaler nil — no history record expected
	})

	postJSON(srv, "/api/dry-run/approve", map[string]string{"id": id})
	if len(sh.GetAll()) != 0 {
		t.Error("expected no history record when mlScaler is nil")
	}
}

// ── Error response shape ──────────────────────────────────────────────────────

func TestErrorResponseShape(t *testing.T) {
	srv := newMinimalServer()
	w := postJSON(srv, "/api/dry-run/approve", map[string]string{"id": "missing"})
	var errResp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("error response is not valid JSON: %v", err)
	}
	if _, ok := errResp["error"]; !ok {
		t.Error("error response missing 'error' field")
	}
}
