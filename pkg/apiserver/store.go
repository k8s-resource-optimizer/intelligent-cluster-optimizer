package apiserver

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"intelligent-cluster-optimizer/pkg/forecaster"
)

// ── ID generation ─────────────────────────────────────────────────────────────

var idCounter atomic.Int64

func newID() string {
	return fmt.Sprintf("%x-%d", time.Now().UnixNano(), idCounter.Add(1))
}

// ── Scaling History Store ─────────────────────────────────────────────────────

// ScalingRecord holds one scaling decision captured by the reconciler.
type ScalingRecord struct {
	ID             string    `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Namespace      string    `json:"namespace"`
	DeploymentName string    `json:"deployment_name"`
	ContainerName  string    `json:"container_name,omitempty"`
	// Reason is one of "ML", "HoltWinters", "Manual"
	Reason string `json:"reason"`
	// Applied is false when the decision was produced in dry-run mode.
	Applied bool `json:"applied"`

	// Horizontal scaling fields (ML forecaster)
	OldReplicas  int32   `json:"old_replicas,omitempty"`
	NewReplicas  int32   `json:"new_replicas,omitempty"`
	PeakCPU      float64 `json:"peak_cpu,omitempty"`
	SustainedCPU float64 `json:"sustained_cpu,omitempty"`

	// Vertical scaling fields (Holt-Winters)
	OldCPU     string  `json:"old_cpu,omitempty"`
	NewCPU     string  `json:"new_cpu,omitempty"`
	OldMemory  string  `json:"old_memory,omitempty"`
	NewMemory  string  `json:"new_memory,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`

	// Cost savings
	SavingsPerHour  float64 `json:"savings_per_hour,omitempty"`
	SavingsPerMonth float64 `json:"savings_per_month,omitempty"`
}

// ScalingHistoryStore keeps the last maxSize scaling decisions in memory.
type ScalingHistoryStore struct {
	mu      sync.RWMutex
	records []ScalingRecord
	maxSize int
}

// NewScalingHistoryStore creates a store that keeps at most maxSize records.
func NewScalingHistoryStore(maxSize int) *ScalingHistoryStore {
	if maxSize <= 0 {
		maxSize = 500
	}
	return &ScalingHistoryStore{maxSize: maxSize}
}

// Add appends a record; oldest entries are dropped when capacity is exceeded.
func (s *ScalingHistoryStore) Add(r ScalingRecord) {
	if r.ID == "" {
		r.ID = newID()
	}
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, r)
	if len(s.records) > s.maxSize {
		s.records = s.records[len(s.records)-s.maxSize:]
	}
}

// GetSnapshotJSON returns a JSON-encoded array of all records for backup purposes.
func (s *ScalingHistoryStore) GetSnapshotJSON() ([]byte, error) {
	return json.Marshal(s.GetAll())
}

// RestoreFromJSON loads records from a JSON-encoded array (used during backup restore).
func (s *ScalingHistoryStore) RestoreFromJSON(data []byte) error {
	var records []ScalingRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return err
	}
	for _, r := range records {
		s.Add(r)
	}
	return nil
}

// GetAll returns a copy of all recorded decisions, newest first.
func (s *ScalingHistoryStore) GetAll() []ScalingRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ScalingRecord, len(s.records))
	copy(out, s.records)
	// reverse so most-recent comes first
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// ── Forecast Cache ────────────────────────────────────────────────────────────

// ForecastEntry holds the latest forecast result for one deployment.
type ForecastEntry struct {
	Timestamp      time.Time                  `json:"timestamp"`
	Namespace      string                     `json:"namespace"`
	DeploymentName string                     `json:"deployment_name"`
	Points         []forecaster.ForecastPoint `json:"points"`
	Decision       *forecaster.ScaleDecision  `json:"decision,omitempty"`
	InferenceMs    float64                    `json:"inference_ms"`
	CPUSamples     int                        `json:"cpu_samples"`
}

// ForecastCache stores the latest forecast per namespace/deployment pair.
type ForecastCache struct {
	mu      sync.RWMutex
	entries map[string]*ForecastEntry // key: "namespace/deployment"
}

// NewForecastCache creates an empty ForecastCache.
func NewForecastCache() *ForecastCache {
	return &ForecastCache{entries: make(map[string]*ForecastEntry)}
}

// Set overwrites the cached forecast for the given deployment.
// Values are clamped to [0, 1] to guard against model outliers.
func (c *ForecastCache) Set(ns, deploy string, entry ForecastEntry) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	for i := range entry.Points {
		entry.Points[i].Low    = clamp(entry.Points[i].Low)
		entry.Points[i].Median = clamp(entry.Points[i].Median)
		entry.Points[i].High   = clamp(entry.Points[i].High)
	}
	key := ns + "/" + deploy
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &entry
}

// GetAll returns a snapshot of all cached forecasts.
func (c *ForecastCache) GetAll() []ForecastEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]ForecastEntry, 0, len(c.entries))
	for _, e := range c.entries {
		out = append(out, *e)
	}
	return out
}

// ── Dry-run Queue ─────────────────────────────────────────────────────────────

// DryRunStatus represents the lifecycle state of a dry-run decision.
type DryRunStatus string

const (
	DryRunPending  DryRunStatus = "pending"
	DryRunApproved DryRunStatus = "approved"
	DryRunRejected DryRunStatus = "rejected"
)

// DryRunDecision holds a pending scaling decision awaiting user action.
type DryRunDecision struct {
	ID        string       `json:"id"`
	CreatedAt time.Time    `json:"created_at"`
	Namespace string       `json:"namespace"`
	Reason    string       `json:"reason"`
	Status    DryRunStatus `json:"status"`
	// ScalingType is "horizontal" or "vertical"
	ScalingType    string     `json:"scaling_type"`
	DeploymentName string     `json:"deployment_name"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`

	// Horizontal scaling fields (ML forecaster)
	CurrentReplicas int32                    `json:"current_replicas,omitempty"`
	DesiredReplicas int32                    `json:"desired_replicas,omitempty"`
	Decision        forecaster.ScaleDecision `json:"decision,omitempty"`

	// Vertical scaling fields (Holt-Winters)
	ContainerName     string  `json:"container_name,omitempty"`
	WorkloadKind      string  `json:"workload_kind,omitempty"`
	CurrentCPU        string  `json:"current_cpu,omitempty"`
	RecommendedCPU    string  `json:"recommended_cpu,omitempty"`
	CurrentMemory     string  `json:"current_memory,omitempty"`
	RecommendedMemory string  `json:"recommended_memory,omitempty"`
	Confidence        float64 `json:"confidence,omitempty"`
	SavingsPerHour    float64 `json:"savings_per_hour,omitempty"`
	SavingsPerMonth   float64 `json:"savings_per_month,omitempty"`
}

const approveCooldown = 2 * time.Minute

// DryRunQueue is a thread-safe store for dry-run decisions.
type DryRunQueue struct {
	mu           sync.RWMutex
	decisions    map[string]*DryRunDecision
	lastApproved map[string]time.Time // key: namespace/deployment/container
}

// NewDryRunQueue creates an empty DryRunQueue.
func NewDryRunQueue() *DryRunQueue {
	return &DryRunQueue{
		decisions:    make(map[string]*DryRunDecision),
		lastApproved: make(map[string]time.Time),
	}
}

// Add enqueues a new dry-run decision. Returns "" if the workload was recently
// approved and is still in the cooldown window.
func (q *DryRunQueue) Add(d DryRunDecision) string {
	if d.ID == "" {
		d.ID = newID()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	d.Status = DryRunPending
	q.mu.Lock()
	defer q.mu.Unlock()
	cooldownKey := d.Namespace + "/" + d.DeploymentName + "/" + d.ContainerName
	if t, ok := q.lastApproved[cooldownKey]; ok && time.Since(t) < approveCooldown {
		return ""
	}
	q.decisions[d.ID] = &d
	return d.ID
}

// GetPending returns all decisions that have not yet been approved or rejected.
func (q *DryRunQueue) GetPending() []DryRunDecision {
	q.mu.RLock()
	defer q.mu.RUnlock()
	var out []DryRunDecision
	for _, d := range q.decisions {
		if d.Status == DryRunPending {
			out = append(out, *d)
		}
	}
	return out
}

// GetAll returns all decisions regardless of status.
func (q *DryRunQueue) GetAll() []DryRunDecision {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make([]DryRunDecision, 0, len(q.decisions))
	for _, d := range q.decisions {
		out = append(out, *d)
	}
	return out
}

// Get returns a single decision by ID.
func (q *DryRunQueue) Get(id string) (*DryRunDecision, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	d, ok := q.decisions[id]
	if !ok {
		return nil, false
	}
	cp := *d
	return &cp, true
}

// Approve marks a pending decision as approved and returns it.
// All other pending decisions for the same deployment are rejected to prevent
// duplicate applies that would conflict with the in-progress rollout.
// Returns (nil, false) if the ID does not exist or is not pending.
func (q *DryRunQueue) Approve(id string) (*DryRunDecision, bool) {
	d, ok := q.setStatus(id, DryRunApproved)
	if !ok {
		return nil, false
	}
	// Record approval time and reject all other pending items for the same
	// workload so they can't be approved during the rollout.
	q.mu.Lock()
	defer q.mu.Unlock()
	cooldownKey := d.Namespace + "/" + d.DeploymentName + "/" + d.ContainerName
	q.lastApproved[cooldownKey] = time.Now()
	now := time.Now()
	for _, other := range q.decisions {
		if other.ID != id &&
			other.Status == DryRunPending &&
			other.Namespace == d.Namespace &&
			other.DeploymentName == d.DeploymentName &&
			other.ContainerName == d.ContainerName {
			other.Status = DryRunRejected
			other.ReviewedAt = &now
		}
	}
	return d, true
}

// Reject marks a pending decision as rejected and returns it.
// Returns (nil, false) if the ID does not exist or is not pending.
func (q *DryRunQueue) Reject(id string) (*DryRunDecision, bool) {
	return q.setStatus(id, DryRunRejected)
}

func (q *DryRunQueue) setStatus(id string, status DryRunStatus) (*DryRunDecision, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	d, ok := q.decisions[id]
	if !ok || d.Status != DryRunPending {
		return nil, false
	}
	now := time.Now()
	d.Status = status
	d.ReviewedAt = &now
	cp := *d
	return &cp, true
}

// RevertToPending resets a decision back to pending if the live apply failed.
// It also clears the cooldown so new recommendations are not blocked.
func (q *DryRunQueue) RevertToPending(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if d, ok := q.decisions[id]; ok {
		d.Status = DryRunPending
		d.ReviewedAt = nil
		cooldownKey := d.Namespace + "/" + d.DeploymentName + "/" + d.ContainerName
		delete(q.lastApproved, cooldownKey)
	}
}
