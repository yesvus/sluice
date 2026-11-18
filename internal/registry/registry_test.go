package registry

import (
	"testing"
)

func TestNewCopiesInput(t *testing.T) {
	accounts := []Account{{
		ID:          "acct-1",
		Provider:    "synthetic",
		OnwatchKey:  "key-1",
		Kind:        "api_key",
		Tier:        "free",
		Deployments: []string{"deploy-1"},
	}}
	r := New(accounts)

	accounts[0].ID = "mutated"
	accounts[0].Deployments[0] = "mutated"

	got := r.Accounts()
	if len(got) != 1 {
		t.Fatalf("Accounts() returned %d accounts, want 1", len(got))
	}
	if got[0].ID != "acct-1" || got[0].Deployments[0] != "deploy-1" {
		t.Fatalf("registry changed after mutating New's input: %+v", got[0])
	}
}

func TestAccountsReturnsCopy(t *testing.T) {
	r := New([]Account{{
		ID:          "acct-1",
		Provider:    "synthetic",
		OnwatchKey:  "key-1",
		Deployments: []string{"deploy-1"},
	}})

	got := r.Accounts()
	got[0].ID = "mutated"
	got[0].Deployments[0] = "mutated"

	still := r.Accounts()
	if still[0].ID != "acct-1" || still[0].Deployments[0] != "deploy-1" {
		t.Fatalf("mutating Accounts() result changed the registry: %+v", still[0])
	}
}

func TestLookup(t *testing.T) {
	r := New([]Account{
		{ID: "acct-1", Provider: "synthetic", OnwatchKey: "key-1", Kind: "api_key", Tier: "free", Deployments: []string{"deploy-1"}},
		{ID: "acct-2", Provider: "other", OnwatchKey: "key-2", Kind: "free_tier", Tier: "cheap"},
	})

	got, ok := r.Lookup("synthetic", "key-1")
	if !ok {
		t.Fatal("Lookup by provider and onwatchKey should hit")
	}
	if got.ID != "acct-1" || got.Provider != "synthetic" || got.OnwatchKey != "key-1" {
		t.Fatalf("Lookup returned %+v, want acct-1", got)
	}

	got.Deployments[0] = "mutated"
	again, ok := r.Lookup("synthetic", "key-1")
	if !ok || again.Deployments[0] != "deploy-1" {
		t.Fatalf("mutating Lookup result changed the registry: %v", again.Deployments)
	}

	if _, ok := r.Lookup("synthetic", "wrong-key"); ok {
		t.Fatal("Lookup with a wrong onwatchKey should miss")
	}
	if _, ok := r.Lookup("wrong-provider", "key-1"); ok {
		t.Fatal("Lookup with a wrong provider should miss")
	}
}
