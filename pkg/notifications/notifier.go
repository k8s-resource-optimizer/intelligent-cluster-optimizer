package notifications

import "context"

// Notifier is the interface that all notifiers must implement
type Notifier interface {
	// Send sends a notification for the given event
	// Returns an error if the notification fails
	Send(ctx context.Context, event *Event) error

	// Name returns the name of the notifier (for logging)
	Name() string
}
