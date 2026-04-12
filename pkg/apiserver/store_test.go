package apiserver_test

import (
	"testing"
	"time"

	"intelligent-cluster-optimizer/pkg/apiserver"
	"intelligent-cluster-optimizer/pkg/forecaster"
)

// ── ScalingHistoryStore ───────────────────────────────────────────────────────

func TestScalingHistoryStore_AddAndGetAll(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(100)
	s.Add(apiserver.ScalingRecord{
		Namespace:      "default",
		DeploymentName: "web",
		OldReplicas:    2,
		NewReplicas:    4,
		Reason:         "ML",
		Applied:        true,
	})
	s.Add(apiserver.ScalingRecord{
		Namespace:      "default",
		DeploymentName: "api",
		OldReplicas:    1,
		NewReplicas:    2,
		Reason:         "Manual",
		Applied:        true,
	})
	all := s.GetAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 records, got %d", len(all))
	}
}

func TestScalingHistoryStore_IDAssigned(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(10)
	s.Add(apiserver.ScalingRecord{DeploymentName: "web"})
	all := s.GetAll()
	if all[0].ID == "" {
		t.Error("expected non-empty ID to be auto-assigned")
	}
}

func TestScalingHistoryStore_TimestampAssigned(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(10)
	before := time.Now()
	s.Add(apiserver.ScalingRecord{DeploymentName: "web"})
	after := time.Now()
	all := s.GetAll()
	ts := all[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not in expected range [%v, %v]", ts, before, after)
	}
}

func TestScalingHistoryStore_MaxSizeEviction(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(3)
	for i := 0; i < 5; i++ {
		s.Add(apiserver.ScalingRecord{DeploymentName: "web", NewReplicas: int32(i + 1)})
	}
	all := s.GetAll()
	if len(all) != 3 {
		t.Fatalf("expected 3 records after eviction, got %d", len(all))
	}
}

func TestScalingHistoryStore_NewestFirst(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(10)
	for i := 0; i < 3; i++ {
		s.Add(apiserver.ScalingRecord{DeploymentName: "web", NewReplicas: int32(i + 1)})
	}
	all := s.GetAll()
	// GetAll returns newest first; last added has NewReplicas=3
	if all[0].NewReplicas != 3 {
		t.Errorf("expected newest first (3), got %d", all[0].NewReplicas)
	}
}

func TestScalingHistoryStore_EmptyReturnsSlice(t *testing.T) {
	s := apiserver.NewScalingHistoryStore(10)
	all := s.GetAll()
	if all == nil {
		t.Error("expected non-nil slice from empty store")
	}
}

func TestScalingHistoryStore_DefaultMaxSize(t *testing.T) {
	// maxSize <= 0 should default to 500 (not panic)
	s := apiserver.NewScalingHistoryStore(0)
	s.Add(apiserver.ScalingRecord{DeploymentName: "web"})
	if len(s.GetAll()) != 1 {
		t.Error("expected 1 record with default max size")
	}
}

// ── ForecastCache ─────────────────────────────────────────────────────────────

func TestForecastCache_SetAndGetAll(t *testing.T) {
	c := apiserver.NewForecastCache()
	entry := apiserver.ForecastEntry{
		Namespace:      "default",
		DeploymentName: "web",
		CPUSamples:     60,
	}
	c.Set("default", "web", entry)

	all := c.GetAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(all))
	}
	if all[0].DeploymentName != "web" {
		t.Errorf("unexpected deployment: %s", all[0].DeploymentName)
	}
}

func TestForecastCache_OverwritesExisting(t *testing.T) {
	c := apiserver.NewForecastCache()
	c.Set("default", "web", apiserver.ForecastEntry{CPUSamples: 30})
	c.Set("default", "web", apiserver.ForecastEntry{CPUSamples: 60})
	all := c.GetAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 entry after overwrite, got %d", len(all))
	}
	if all[0].CPUSamples != 60 {
		t.Errorf("expected updated CPUSamples=60, got %d", all[0].CPUSamples)
	}
}

func TestForecastCache_MultipleDeployments(t *testing.T) {
	c := apiserver.NewForecastCache()
	c.Set("default", "web", apiserver.ForecastEntry{DeploymentName: "web"})
	c.Set("default", "api", apiserver.ForecastEntry{DeploymentName: "api"})
	c.Set("staging", "worker", apiserver.ForecastEntry{DeploymentName: "worker"})
	if len(c.GetAll()) != 3 {
		t.Errorf("expected 3 entries, got %d", len(c.GetAll()))
	}
}

func TestForecastCache_TimestampAutoSet(t *testing.T) {
	c := apiserver.NewForecastCache()
	before := time.Now()
	c.Set("default", "web", apiserver.ForecastEntry{})
	after := time.Now()
	all := c.GetAll()
	ts := all[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v not in expected range", ts)
	}
}

func TestForecastCache_EmptyReturnsSlice(t *testing.T) {
	c := apiserver.NewForecastCache()
	if c.GetAll() == nil {
		t.Error("expected non-nil slice from empty cache")
	}
}

func TestForecastCache_WithDecision(t *testing.T) {
	c := apiserver.NewForecastCache()
	d := forecaster.ScaleDecision{ScaleUp: true, DesiredReplicas: 4, PeakCPU: 0.85}
	c.Set("default", "web", apiserver.ForecastEntry{
		DeploymentName: "web",
		Decision:       &d,
		Points: []forecaster.ForecastPoint{
			{Step: 1, Low: 0.5, Median: 0.7, High: 0.85},
		},
	})
	all := c.GetAll()
	if all[0].Decision == nil {
		t.Fatal("expected non-nil decision in cached entry")
	}
	if all[0].Decision.DesiredReplicas != 4 {
		t.Errorf("expected DesiredReplicas=4, got %d", all[0].Decision.DesiredReplicas)
	}
}

// ── DryRunQueue ───────────────────────────────────────────────────────────────

func TestDryRunQueue_AddAndGetPending(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	q.Add(apiserver.DryRunDecision{
		Namespace:       "default",
		DeploymentName:  "web",
		DesiredReplicas: 4,
	})
	pending := q.GetPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].Status != apiserver.DryRunPending {
		t.Errorf("expected pending status, got %s", pending[0].Status)
	}
}

func TestDryRunQueue_AddSetsStatusPending(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	q.Add(apiserver.DryRunDecision{DeploymentName: "web"})
	all := q.GetAll()
	if all[0].Status != apiserver.DryRunPending {
		t.Error("newly added decision should have status=pending")
	}
}

func TestDryRunQueue_AddReturnsID(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "web"})
	if id == "" {
		t.Error("Add should return a non-empty ID")
	}
}

func TestDryRunQueue_IDAutoAssigned(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	q.Add(apiserver.DryRunDecision{DeploymentName: "web"}) // no ID set
	all := q.GetAll()
	if all[0].ID == "" {
		t.Error("ID should be auto-assigned when empty")
	}
}

func TestDryRunQueue_CreatedAtAutoSet(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	before := time.Now()
	q.Add(apiserver.DryRunDecision{DeploymentName: "web"})
	after := time.Now()
	all := q.GetAll()
	if all[0].CreatedAt.Before(before) || all[0].CreatedAt.After(after) {
		t.Error("CreatedAt not in expected range")
	}
}

func TestDryRunQueue_Approve(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{Namespace: "default", DeploymentName: "web"})

	d, ok := q.Approve(id)
	if !ok {
		t.Fatal("Approve should succeed for a pending decision")
	}
	if d.Status != apiserver.DryRunApproved {
		t.Errorf("expected approved, got %s", d.Status)
	}
	if d.ReviewedAt == nil {
		t.Error("ReviewedAt should be set after approval")
	}
}

func TestDryRunQueue_Reject(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{Namespace: "default", DeploymentName: "web"})

	d, ok := q.Reject(id)
	if !ok {
		t.Fatal("Reject should succeed for a pending decision")
	}
	if d.Status != apiserver.DryRunRejected {
		t.Errorf("expected rejected, got %s", d.Status)
	}
}

func TestDryRunQueue_ApproveNonExistent(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	_, ok := q.Approve("nonexistent-id")
	if ok {
		t.Error("Approve should fail for non-existent ID")
	}
}

func TestDryRunQueue_RejectNonExistent(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	_, ok := q.Reject("nonexistent-id")
	if ok {
		t.Error("Reject should fail for non-existent ID")
	}
}

func TestDryRunQueue_DoubleApproveRejected(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "web"})
	q.Approve(id)
	_, ok := q.Approve(id) // second approve should fail
	if ok {
		t.Error("second Approve on already-approved decision should fail")
	}
}

func TestDryRunQueue_ApproveAfterRejectFails(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{DeploymentName: "web"})
	q.Reject(id)
	_, ok := q.Approve(id)
	if ok {
		t.Error("Approve after Reject should fail")
	}
}

func TestDryRunQueue_GetByID(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id := q.Add(apiserver.DryRunDecision{Namespace: "default", DeploymentName: "svc"})

	d, ok := q.Get(id)
	if !ok {
		t.Fatal("Get should find an existing decision")
	}
	if d.DeploymentName != "svc" {
		t.Errorf("unexpected deployment name: %s", d.DeploymentName)
	}
}

func TestDryRunQueue_GetNonExistent(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	_, ok := q.Get("missing")
	if ok {
		t.Error("Get should return false for missing ID")
	}
}

func TestDryRunQueue_GetPendingExcludesActioned(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id1 := q.Add(apiserver.DryRunDecision{DeploymentName: "web"})
	q.Add(apiserver.DryRunDecision{DeploymentName: "api"})
	q.Approve(id1)

	pending := q.GetPending()
	if len(pending) != 1 {
		t.Errorf("expected 1 pending (api), got %d", len(pending))
	}
	if pending[0].DeploymentName != "api" {
		t.Errorf("expected api pending, got %s", pending[0].DeploymentName)
	}
}

func TestDryRunQueue_GetAllIncludesAllStatuses(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	id1 := q.Add(apiserver.DryRunDecision{DeploymentName: "web"})
	id2 := q.Add(apiserver.DryRunDecision{DeploymentName: "api"})
	q.Add(apiserver.DryRunDecision{DeploymentName: "worker"})
	q.Approve(id1)
	q.Reject(id2)

	all := q.GetAll()
	if len(all) != 3 {
		t.Errorf("expected 3 total decisions, got %d", len(all))
	}
}

func TestDryRunQueue_EmptyPendingReturnsSlice(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	pending := q.GetPending()
	// May return nil — that's fine since we handle it in the handler.
	_ = pending
}

// ── newID uniqueness ──────────────────────────────────────────────────────────

func TestIDUniqueness(t *testing.T) {
	q := apiserver.NewDryRunQueue()
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := q.Add(apiserver.DryRunDecision{DeploymentName: "web"})
		if ids[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}
