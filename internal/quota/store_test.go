package quota

import (
	"fmt"
	"sync"
	"testing"
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
}
