// Package notify defines Cronus's notification seam. Notifications are out of
// scope for v1 (§6); this interface exists so Shoutrrr-based drift/unreachable
// alerts can be added later without refactoring callers. The default
// implementation is a no-op.
package notify

import (
	"context"
	"time"
)

// Kind classifies a notification event.
type Kind string

const (
	// KindUnreachable indicates a monitored server became unreachable.
	KindUnreachable Kind = "unreachable"
	// KindDrift indicates a monitored server's drift crossed a threshold.
	KindDrift Kind = "drift"
	// KindOutlier indicates a monitored server was flagged as a falseticker.
	KindOutlier Kind = "outlier"
)

// Event is a single notification.
type Event struct {
	Kind    Kind
	Server  string
	Message string
	At      time.Time
}

// Notifier delivers notification events. Implementations must be safe for
// concurrent use and best-effort: a delivery error must never abort the caller.
type Notifier interface {
	Notify(ctx context.Context, ev Event) error
}

// Nop is the default no-op Notifier used in v1.
type Nop struct{}

// Notify does nothing and always succeeds.
func (Nop) Notify(context.Context, Event) error { return nil }
