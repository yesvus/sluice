package quota

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestStorePutGetAll(t *testing.T) {
	s := &Store{}
	if _, ok := s.Get("missing"); ok {
		t.Fatal("Get on empty store should report a miss")
	}

	snaps := []Snapshot{{Provider: "synthetic", Window: "7d", Utilization: 0.5}}
	s.Put("acct-a", snaps)

	got, ok := s.Get("acct-a")
	if !ok {
		t.Fatal("Get after Put should report a hit")
	}
	if len(got) != 1 || got[0].Provider != "synthetic" || got[0].Utilization != 0.5 {
		t.Fatalf("unexpected snapshots: %+v", got)
	}

	all := s.All()
	if len(all) != 1 || len(all["acct-a"]) != 1 {
		t.Fatalf("unexpected All result: %+v", all)
	}
}

func TestStoreConcurrentPutGetAll(t *testing.T) {
	const (
		writers    = 8
		readers    = 8
		iterations = 200
		accounts   = 4
	)

	s := &Store{}
	var wg sync.WaitGroup

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				id := fmt.Sprintf("acct-%d", (w+i)%accounts)
				s.Put(id, []Snapshot{{
					Provider:    "synthetic",
					AccountKey:  id,
					Window:      "5h",
					Utilization: float64(i) / iterations,
				}})
			}
		}(w)
	}

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				s.Get(fmt.Sprintf("acct-%d", i%accounts))
				s.All()
			}
		}()
	}

	wg.Wait()

	all := s.All()
	for i := 0; i < accounts; i++ {
		if _, ok := all[fmt.Sprintf("acct-%d", i)]; !ok {
			t.Fatalf("acct-%d missing after concurrent Put", i)
		}
	}
}

func TestStoreSnapshotOwnership(t *testing.T) {
	s := &Store{}
	used, limit := 10.0, 100.0
	resetsAt := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	s.Put("acct-a", []Snapshot{{
		Provider: "synthetic",
		Used:     &used,
		Limit:    &limit,
		ResetsAt: &resetsAt,
	}})

	used = 20
	limit = 200
	resetsAt = resetsAt.Add(time.Hour)
	assertPublished(t, s, "after mutating the Put input")

	got, ok := s.Get("acct-a")
	if !ok {
		t.Fatal("Get after Put should report a hit")
	}
	*got[0].Used = 30
	*got[0].Limit = 300
	*got[0].ResetsAt = time.Time{}
	assertPublished(t, s, "after mutating a Get result")

	all := s.All()
	*all["acct-a"][0].Used = 40
	*all["acct-a"][0].Limit = 400
	*all["acct-a"][0].ResetsAt = time.Time{}
	assertPublished(t, s, "after mutating an All result")
}

func assertPublished(t *testing.T, s *Store, when string) {
	t.Helper()
	snaps, ok := s.Get("acct-a")
	if !ok {
		t.Fatalf("Get %s: acct-a missing", when)
	}
	want := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	snap := snaps[0]
	if *snap.Used != 10 || *snap.Limit != 100 || !snap.ResetsAt.Equal(want) {
		t.Fatalf("store changed %s: used=%v limit=%v resetsAt=%v",
			when, *snap.Used, *snap.Limit, snap.ResetsAt)
	}
}
