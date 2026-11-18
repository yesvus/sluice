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
	s.data[accountID] = append([]Snapshot(nil), snaps...)
}

// Get returns the latest snapshots for accountID.
func (s *Store) Get(accountID string) ([]Snapshot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snaps, ok := s.data[accountID]
	if !ok {
		return nil, false
	}
	return append([]Snapshot(nil), snaps...), true
}

// All returns a copy of the latest state for every account.
func (s *Store) All() map[string][]Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string][]Snapshot, len(s.data))
	for accountID, snaps := range s.data {
		out[accountID] = append([]Snapshot(nil), snaps...)
	}
	return out
}
