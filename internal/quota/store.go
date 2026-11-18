package quota

import "sync"

// Store is the in-memory latest state per account_id, safe for concurrent use.
type Store struct {
	mu   sync.RWMutex
	data map[string][]Snapshot
}

// Put replaces the latest snapshots for accountID.
func (s *Store) Put(accountID string, snaps []Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = make(map[string][]Snapshot)
	}
	s.data[accountID] = cloneSnapshots(snaps)
}

// Get returns the latest snapshots for accountID.
func (s *Store) Get(accountID string) ([]Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snaps, ok := s.data[accountID]
	if !ok {
		return nil, false
	}
	return cloneSnapshots(snaps), true
}

// All returns a copy of the latest state for every account.
func (s *Store) All() map[string][]Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string][]Snapshot, len(s.data))
	for accountID, snaps := range s.data {
		out[accountID] = cloneSnapshots(snaps)
	}
	return out
}

func cloneSnapshots(snaps []Snapshot) []Snapshot {
	out := make([]Snapshot, len(snaps))
	for i, snap := range snaps {
		out[i] = cloneSnapshot(snap)
	}
	return out
}

// cloneSnapshot copies the pointees too: Store promises its state cannot change
// without a Put, so neither callers nor readers may reach store-owned values.
func cloneSnapshot(snap Snapshot) Snapshot {
	if snap.Used != nil {
		used := *snap.Used
		snap.Used = &used
	}
	if snap.Limit != nil {
		limit := *snap.Limit
		snap.Limit = &limit
	}
	if snap.ResetsAt != nil {
		resetsAt := *snap.ResetsAt
		snap.ResetsAt = &resetsAt
	}
	return snap
}
