package quota

import "context"

// FakeSource is a Source for tests. A function field takes precedence over the
// matching data field.
type FakeSource struct {
	HealthFunc  func(context.Context) error
	CurrentFunc func(context.Context) ([]Snapshot, error)
	Err         error
	Snapshots   []Snapshot
}

// Health returns HealthFunc's result when set, otherwise nil.
func (f *FakeSource) Health(ctx context.Context) error {
	if f.HealthFunc != nil {
		return f.HealthFunc(ctx)
	}
	return nil
}

// Current returns CurrentFunc's result when set, otherwise the configured data.
func (f *FakeSource) Current(ctx context.Context) ([]Snapshot, error) {
	if f.CurrentFunc != nil {
		return f.CurrentFunc(ctx)
	}
	return f.Snapshots, f.Err
}
