package notifications

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Manager manages multiple notifiers and fans out events to all configured channels
type Manager struct {
	notifiers   []Notifier
	logger      *zap.Logger
	maxRetries  int
	retryDelay  time.Duration
	sendTimeout time.Duration
	mu          sync.RWMutex
}

// NewManager creates a new notification manager
func NewManager(logger *zap.Logger) *Manager {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Manager{
		notifiers:   make([]Notifier, 0),
		logger:      logger,
		maxRetries:  3,
		retryDelay:  2 * time.Second,
		sendTimeout: 30 * time.Second,
	}
}

// AddNotifier adds a notifier to the manager
func (m *Manager) AddNotifier(notifier Notifier) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifiers = append(m.notifiers, notifier)
	m.logger.Info("notifier added", zap.String("name", notifier.Name()))
}

// RemoveAllNotifiers removes all notifiers (useful for testing)
func (m *Manager) RemoveAllNotifiers() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifiers = make([]Notifier, 0)
}

// SetRetryConfig sets the retry configuration
func (m *Manager) SetRetryConfig(maxRetries int, retryDelay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maxRetries = maxRetries
	m.retryDelay = retryDelay
}

// SetSendTimeout sets the timeout for sending notifications
func (m *Manager) SetSendTimeout(timeout time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendTimeout = timeout
}

// Send sends an event to all configured notifiers
// This is a non-blocking call - notifications are sent in background goroutines
// Notification failures do NOT crash or block the caller
func (m *Manager) Send(event *Event) {
	m.mu.RLock()
	notifiers := make([]Notifier, len(m.notifiers))
	copy(notifiers, m.notifiers)
	m.mu.RUnlock()

	if len(notifiers) == 0 {
		m.logger.Debug("no notifiers configured, skipping notification",
			zap.String("event_type", string(event.Type)),
			zap.String("namespace", event.Namespace))
		return
	}

	// Make a copy of the event to avoid race conditions when called concurrently
	eventCopy := *event

	// Set timestamp if not already set
	if eventCopy.Timestamp.IsZero() {
		eventCopy.Timestamp = time.Now()
	}

	m.logger.Info("sending notification",
		zap.String("event_type", string(eventCopy.Type)),
		zap.String("severity", string(eventCopy.Severity)),
		zap.String("namespace", eventCopy.Namespace),
		zap.Int("notifier_count", len(notifiers)))

	// Fan out to all notifiers in parallel
	for _, notifier := range notifiers {
		// Launch each notifier in a goroutine to avoid blocking
		go m.sendWithRetry(notifier, &eventCopy)
	}
}

// sendWithRetry sends a notification with retry logic
func (m *Manager) sendWithRetry(notifier Notifier, event *Event) {
	for attempt := 1; attempt <= m.maxRetries; attempt++ {
		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), m.sendTimeout)

		// Try to send
		err := notifier.Send(ctx, event)
		cancel()

		if err == nil {
			m.logger.Info("notification sent successfully",
				zap.String("notifier", notifier.Name()),
				zap.String("event_type", string(event.Type)),
				zap.Int("attempt", attempt))
			return
		}

		// Log error
		m.logger.Error("failed to send notification",
			zap.String("notifier", notifier.Name()),
			zap.String("event_type", string(event.Type)),
			zap.Int("attempt", attempt),
			zap.Int("max_retries", m.maxRetries),
			zap.Error(err))

		// If this was the last attempt, give up
		if attempt == m.maxRetries {
			m.logger.Error("notification failed after all retries",
				zap.String("notifier", notifier.Name()),
				zap.String("event_type", string(event.Type)),
				zap.Int("max_retries", m.maxRetries))
			return
		}

		// Wait before retrying with exponential backoff
		backoffDelay := m.retryDelay * time.Duration(1<<uint(attempt-1))
		m.logger.Info("retrying notification after backoff",
			zap.String("notifier", notifier.Name()),
			zap.Duration("backoff", backoffDelay))
		time.Sleep(backoffDelay)
	}
}

// SendBlocking sends an event to all configured notifiers and waits for all to complete
// Returns an error if any notifier fails after all retries
// This should only be used when you need to ensure delivery (e.g., critical events)
func (m *Manager) SendBlocking(event *Event) error {
	m.mu.RLock()
	notifiers := make([]Notifier, len(m.notifiers))
	copy(notifiers, m.notifiers)
	m.mu.RUnlock()

	if len(notifiers) == 0 {
		return fmt.Errorf("no notifiers configured")
	}

	// Set timestamp if not already set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	var wg sync.WaitGroup
	errors := make([]error, len(notifiers))

	// Send to all notifiers in parallel
	for i, notifier := range notifiers {
		wg.Add(1)
		go func(idx int, n Notifier) {
			defer wg.Done()

			// Try with retries
			for attempt := 1; attempt <= m.maxRetries; attempt++ {
				ctx, cancel := context.WithTimeout(context.Background(), m.sendTimeout)
				err := n.Send(ctx, event)
				cancel()

				if err == nil {
					return
				}

				if attempt == m.maxRetries {
					errors[idx] = fmt.Errorf("notifier %s failed after %d retries: %w", n.Name(), m.maxRetries, err)
					return
				}

				backoffDelay := m.retryDelay * time.Duration(1<<uint(attempt-1))
				time.Sleep(backoffDelay)
			}
		}(i, notifier)
	}

	wg.Wait()

	// Collect errors
	var failedNotifiers []string
	for i, err := range errors {
		if err != nil {
			failedNotifiers = append(failedNotifiers, notifiers[i].Name())
			m.logger.Error("notifier failed",
				zap.String("notifier", notifiers[i].Name()),
				zap.Error(err))
		}
	}

	if len(failedNotifiers) > 0 {
		return fmt.Errorf("notification failed for %d/%d notifiers: %v", len(failedNotifiers), len(notifiers), failedNotifiers)
	}

	return nil
}

// GetNotifierCount returns the number of configured notifiers
func (m *Manager) GetNotifierCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.notifiers)
}
