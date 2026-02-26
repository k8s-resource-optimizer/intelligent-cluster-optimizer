package notifications

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// Mock notifier for testing
type mockNotifier struct {
	name        string
	sendCalled  int32
	shouldFail  bool
	failCount   int
	currentFail int32
	mu          sync.Mutex
}

func (m *mockNotifier) Send(ctx context.Context, event *Event) error {
	atomic.AddInt32(&m.sendCalled, 1)

	if m.shouldFail {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.currentFail++
		if int(m.currentFail) <= m.failCount {
			return context.DeadlineExceeded
		}
	}

	return nil
}

func (m *mockNotifier) Name() string {
	return m.name
}

func (m *mockNotifier) getSendCount() int {
	return int(atomic.LoadInt32(&m.sendCalled))
}

func TestManager_AddNotifier(t *testing.T) {
	manager := NewManager(zap.NewNop())

	if manager.GetNotifierCount() != 0 {
		t.Errorf("Expected 0 notifiers, got %d", manager.GetNotifierCount())
	}

	notifier1 := &mockNotifier{name: "mock1"}
	manager.AddNotifier(notifier1)

	if manager.GetNotifierCount() != 1 {
		t.Errorf("Expected 1 notifier, got %d", manager.GetNotifierCount())
	}

	notifier2 := &mockNotifier{name: "mock2"}
	manager.AddNotifier(notifier2)

	if manager.GetNotifierCount() != 2 {
		t.Errorf("Expected 2 notifiers, got %d", manager.GetNotifierCount())
	}
}

func TestManager_Send(t *testing.T) {
	manager := NewManager(zap.NewNop())

	notifier1 := &mockNotifier{name: "mock1"}
	notifier2 := &mockNotifier{name: "mock2"}

	manager.AddNotifier(notifier1)
	manager.AddNotifier(notifier2)

	event := &Event{
		Type:      EventRecommendationApplied,
		Severity:  SeverityInfo,
		Namespace: "default",
		Message:   "Test message",
	}

	// Send is non-blocking, so we need to wait a bit
	manager.Send(event)
	time.Sleep(100 * time.Millisecond)

	if notifier1.getSendCount() == 0 {
		t.Error("Expected notifier1 to be called")
	}

	if notifier2.getSendCount() == 0 {
		t.Error("Expected notifier2 to be called")
	}
}

func TestManager_SendBlocking(t *testing.T) {
	manager := NewManager(zap.NewNop())

	notifier1 := &mockNotifier{name: "mock1"}
	notifier2 := &mockNotifier{name: "mock2"}

	manager.AddNotifier(notifier1)
	manager.AddNotifier(notifier2)

	event := &Event{
		Type:      EventOOMDetected,
		Severity:  SeverityCritical,
		Namespace: "production",
		Message:   "OOM detected",
	}

	err := manager.SendBlocking(event)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if notifier1.getSendCount() != 1 {
		t.Errorf("Expected notifier1 to be called once, got %d", notifier1.getSendCount())
	}

	if notifier2.getSendCount() != 1 {
		t.Errorf("Expected notifier2 to be called once, got %d", notifier2.getSendCount())
	}
}

func TestManager_SendWithRetry(t *testing.T) {
	manager := NewManager(zap.NewNop())
	manager.SetRetryConfig(3, 10*time.Millisecond)
	manager.SetSendTimeout(50 * time.Millisecond)

	// Notifier that fails first 2 times, then succeeds
	notifier := &mockNotifier{
		name:       "flaky",
		shouldFail: true,
		failCount:  2,
	}

	manager.AddNotifier(notifier)

	event := &Event{
		Type:      EventMemoryLeakDetected,
		Severity:  SeverityWarning,
		Namespace: "default",
		Message:   "Memory leak detected",
	}

	// Non-blocking send
	manager.Send(event)

	// Wait for retries to complete
	time.Sleep(500 * time.Millisecond)

	// Should be called 3 times (initial + 2 retries)
	if notifier.getSendCount() != 3 {
		t.Errorf("Expected 3 send attempts, got %d", notifier.getSendCount())
	}
}

func TestManager_SendBlockingWithFailure(t *testing.T) {
	manager := NewManager(zap.NewNop())
	manager.SetRetryConfig(2, 10*time.Millisecond)
	manager.SetSendTimeout(50 * time.Millisecond)

	// Notifier that always fails
	failingNotifier := &mockNotifier{
		name:       "failing",
		shouldFail: true,
		failCount:  10, // More than max retries
	}

	manager.AddNotifier(failingNotifier)

	event := &Event{
		Type:      EventCircuitBreakerTriggered,
		Severity:  SeverityCritical,
		Namespace: "production",
		Message:   "Circuit breaker triggered",
	}

	err := manager.SendBlocking(event)
	if err == nil {
		t.Error("Expected error from failing notifier")
	}
}

func TestManager_NoNotifiersConfigured(t *testing.T) {
	manager := NewManager(zap.NewNop())

	event := &Event{
		Type:      EventRecommendationApplied,
		Severity:  SeverityInfo,
		Namespace: "default",
		Message:   "Test",
	}

	// Non-blocking send should not panic
	manager.Send(event)

	// Blocking send should return error
	err := manager.SendBlocking(event)
	if err == nil {
		t.Error("Expected error when no notifiers configured")
	}
}

func TestManager_RemoveAllNotifiers(t *testing.T) {
	manager := NewManager(zap.NewNop())

	notifier := &mockNotifier{name: "mock"}
	manager.AddNotifier(notifier)

	if manager.GetNotifierCount() != 1 {
		t.Errorf("Expected 1 notifier, got %d", manager.GetNotifierCount())
	}

	manager.RemoveAllNotifiers()

	if manager.GetNotifierCount() != 0 {
		t.Errorf("Expected 0 notifiers after removal, got %d", manager.GetNotifierCount())
	}
}

func TestManager_ConcurrentSend(t *testing.T) {
	manager := NewManager(zap.NewNop())

	notifier := &mockNotifier{name: "concurrent"}
	manager.AddNotifier(notifier)

	event := &Event{
		Type:      EventSLAViolation,
		Severity:  SeverityWarning,
		Namespace: "default",
		Message:   "SLA violation",
	}

	// Send multiple events concurrently
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			manager.Send(event)
		}()
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	// Should have been called for each event
	if notifier.getSendCount() < numGoroutines {
		t.Errorf("Expected at least %d send calls, got %d", numGoroutines, notifier.getSendCount())
	}
}

func TestManager_SetRetryConfig(t *testing.T) {
	manager := NewManager(zap.NewNop())

	manager.SetRetryConfig(5, 100*time.Millisecond)

	if manager.maxRetries != 5 {
		t.Errorf("Expected maxRetries=5, got %d", manager.maxRetries)
	}

	if manager.retryDelay != 100*time.Millisecond {
		t.Errorf("Expected retryDelay=100ms, got %v", manager.retryDelay)
	}
}

func TestManager_SetSendTimeout(t *testing.T) {
	manager := NewManager(zap.NewNop())

	manager.SetSendTimeout(5 * time.Second)

	if manager.sendTimeout != 5*time.Second {
		t.Errorf("Expected sendTimeout=5s, got %v", manager.sendTimeout)
	}
}

func TestManager_EventTimestamp(t *testing.T) {
	manager := NewManager(zap.NewNop())

	notifier := &mockNotifier{name: "timestamp"}
	manager.AddNotifier(notifier)

	// Event without timestamp
	event := &Event{
		Type:      EventRecommendationApplied,
		Severity:  SeverityInfo,
		Namespace: "default",
		Message:   "Test",
		// No Timestamp set
	}

	before := time.Now()
	err := manager.SendBlocking(event)
	after := time.Now()

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Timestamp should have been set
	if event.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}

	// Timestamp should be between before and after
	if event.Timestamp.Before(before) || event.Timestamp.After(after) {
		t.Errorf("Timestamp %v not within expected range [%v, %v]", event.Timestamp, before, after)
	}
}

func TestManager_MixedSuccessFailure(t *testing.T) {
	manager := NewManager(zap.NewNop())
	manager.SetRetryConfig(2, 10*time.Millisecond)

	successNotifier := &mockNotifier{name: "success"}
	failingNotifier := &mockNotifier{
		name:       "failing",
		shouldFail: true,
		failCount:  10,
	}

	manager.AddNotifier(successNotifier)
	manager.AddNotifier(failingNotifier)

	event := &Event{
		Type:      EventRollbackExecuted,
		Severity:  SeverityWarning,
		Namespace: "default",
		Message:   "Rollback executed",
	}

	err := manager.SendBlocking(event)

	// Should get error due to failing notifier
	if err == nil {
		t.Error("Expected error from mixed success/failure")
	}

	// Success notifier should still have been called
	if successNotifier.getSendCount() == 0 {
		t.Error("Expected success notifier to be called")
	}
}
