package connector

import (
	"context"
	"errors"
	"time"

	"github.com/Ahmed20011994/anton/internal/canonical"
)

type HealthStatus struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latency_ms"`
	Detail    string `json:"detail,omitempty"`
}

// WorkItemConnector pulls work items from an external system. Implementations
// stream items via the callback so memory stays bounded regardless of result
// size. Returning a non-nil error from fn aborts the stream — the worker uses
// this to honor ctx cancellation.
type WorkItemConnector interface {
	SourceType() string
	Connect(ctx context.Context) error
	HealthCheck(ctx context.Context) HealthStatus
	StreamSync(ctx context.Context, since time.Time, fn func(canonical.WorkItem) error) (fetchErrors int, err error)
	StreamBackfill(ctx context.Context, from time.Time, fn func(canonical.WorkItem) error) (fetchErrors int, err error)
}

type SignalConnector interface {
	SourceType() string
	Connect(ctx context.Context) error
	HealthCheck(ctx context.Context) HealthStatus
	StreamSync(ctx context.Context, since time.Time, fn func(canonical.CustomerSignal) error) (fetchErrors int, err error)
	StreamBackfill(ctx context.Context, from time.Time, fn func(canonical.CustomerSignal) error) (fetchErrors int, err error)
}

var (
	ErrAuth      = errors.New("connector: authentication failed")
	ErrTransient = errors.New("connector: transient error")
)

type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return "connector: rate limited; retry after " + e.RetryAfter.String()
}
