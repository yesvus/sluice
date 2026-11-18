package quota

import (
	"context"
	"time"
)

// Snapshot is one provider's reported quota at a point in time.
type Snapshot struct {
	Provider    string     // onWatch provider name
	AccountKey  string     // onWatch's per-account key; "" if the provider has one account
	Window      string     // e.g. "5h", "7d", "monthly"
	Utilization float64    // 0..1
	Used, Limit *float64   // nil when the provider reports utilization only
	ResetsAt    *time.Time // nil when unknown
	ObservedAt  time.Time
}

// Source supplies quota snapshots from a provider.
type Source interface {
	Health(ctx context.Context) error
	Current(ctx context.Context) ([]Snapshot, error)
}
