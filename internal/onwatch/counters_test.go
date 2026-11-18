package onwatch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func miniMaxExpected() map[snapshotKey]snapshotWant {
	return map[snapshotKey]snapshotWant{
		{"minimax", "103", "synthetic-pool"}:      {0.2, 20, 100, true, "2026-01-01T10:00:00Z", "2026-01-01T15:00:00Z"},
		{"minimax", "103", "wkly_synthetic-pool"}: {0.4, 80, 200, true, "2026-01-01T10:00:00Z", ""},
		{"minimax", "104", "synthetic-pool"}:      {0.1, 10, 100, true, "2026-01-01T10:00:00Z", ""},
	}
}

func TestCounterProviders(t *testing.T) {
	for _, tc := range []struct {
		provider string
		want     map[snapshotKey]snapshotWant
	}{
		{"synthetic", map[snapshotKey]snapshotWant{
			{"synthetic", "", "subscription"}: {0.6, 60, 100, true, "2026-01-01T10:00:00Z", "2026-01-01T15:00:00Z"},
			{"synthetic", "", "toolCalls"}:    {0.2, 2, 10, true, "2026-01-01T10:00:00Z", "2026-01-01T15:00:00Z"},
		}},
		{"zai", map[snapshotKey]snapshotWant{
			{"zai", "", "tokensLimit"}: {0.4, 40, 100, true, "2026-01-01T10:00:00Z", ""},
			{"zai", "", "timeLimit"}:   {0.5, 5, 10, true, "2026-01-01T10:00:00Z", "2026-01-01T15:00:00Z"},
		}},
		{"openrouter", map[snapshotKey]snapshotWant{
			{"openrouter", "", "credits"}: {0.4, 2, 5, true, "2026-01-01T10:00:00Z", ""},
		}},
		{"minimax", miniMaxExpected()},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			body := fixture(t, tc.provider)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/providers":
					_ = json.NewEncoder(w).Encode(map[string][]string{"providers": {tc.provider}})
				case "/api/minimax/accounts/usage":
					if tc.provider != "minimax" {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					_, _ = w.Write(body)
				case "/api/current":
					if tc.provider == "minimax" || r.URL.Query().Get("provider") != tc.provider {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					_, _ = w.Write(body)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()
			rows, err := New(srv.URL, "", "").Current(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			assertSnapshots(t, rows, tc.want)
		})
	}
}

func TestUnavailableObservations(t *testing.T) {
	for _, tc := range []struct {
		provider string
		body     string
	}{
		{"codex", `{"capturedAt":"2026-01-01T10:00:00Z","paused":true,"quotas":[{"name":"five_hour","utilization":10}]}`},
		{"codex", `{"capturedAt":"2026-01-01T10:00:00Z","authRequired":true,"quotas":[{"name":"five_hour","utilization":10}]}`},
		{"anthropic", `{"capturedAt":"2026-01-01T10:00:00Z","quotas":[{"name":"five_hour","utilization":10,"isStale":true}]}`},
		{"opencode", `{"capturedAt":"2026-01-01T10:00:00Z","quotas":[]}`},
		{"openrouter", `{"capturedAt":"2026-01-01T10:00:00Z","credits":{"usage":0,"limit":null,"percent":0}}`},
	} {
		rows, err := decodeProvider(tc.provider, json.RawMessage(tc.body), false)
		if !errors.Is(err, errUnavailable) || rows != nil {
			t.Fatalf("unavailable %s treated as observed: %v", tc.provider, err)
		}
	}
}

func TestCombinedMiniMaxAccounts(t *testing.T) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(fixture(t, "minimax"), &data); err != nil {
		t.Fatal(err)
	}
	rows, err := decodeCombined(map[string]json.RawMessage{"minimaxAccounts": data["accounts"]}, map[string]bool{"minimax": true})
	if err != nil {
		t.Fatalf("combined accounts: %v", err)
	}
	assertSnapshots(t, rows, miniMaxExpected())
	if _, err := decodeCombined(map[string]json.RawMessage{}, map[string]bool{"minimax": true}); !errors.Is(err, errUnavailable) {
		t.Fatal("empty combined data accepted as fresh")
	}
}

func TestCounterUnavailableFlags(t *testing.T) {
	for _, provider := range []string{"synthetic", "zai", "openrouter"} {
		for _, flag := range []string{"paused", "authRequired", "isStale"} {
			t.Run(provider+"/"+flag, func(t *testing.T) {
				var data map[string]json.RawMessage
				if err := json.Unmarshal(fixture(t, provider), &data); err != nil {
					t.Fatal(err)
				}
				data[flag] = json.RawMessage("true")
				body, err := json.Marshal(data)
				if err != nil {
					t.Fatal(err)
				}
				rows, err := decodeProvider(provider, body, false)
				if rows != nil || !errors.Is(err, errUnavailable) {
					t.Fatal("unavailable counter provider returned observations")
				}
			})
		}
	}
}

func TestUnknownCounterLimitPreservesFiniteSibling(t *testing.T) {
	for _, tc := range []struct {
		provider string
		window   string
		want     map[snapshotKey]snapshotWant
	}{
		{"synthetic", "search", map[snapshotKey]snapshotWant{
			{"synthetic", "", "subscription"}: {0.6, 60, 100, true, "2026-01-01T10:00:00Z", "2026-01-01T15:00:00Z"},
			{"synthetic", "", "toolCalls"}:    {0.2, 2, 10, true, "2026-01-01T10:00:00Z", "2026-01-01T15:00:00Z"},
		}},
		{"zai", "tokensLimit", map[snapshotKey]snapshotWant{
			{"zai", "", "timeLimit"}: {0.5, 5, 10, true, "2026-01-01T10:00:00Z", "2026-01-01T15:00:00Z"},
		}},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			var data map[string]json.RawMessage
			if err := json.Unmarshal(fixture(t, tc.provider), &data); err != nil {
				t.Fatal(err)
			}
			data[tc.window] = json.RawMessage(`{"limit":null}`)
			body, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := decodeProvider(tc.provider, body, false)
			if err != nil {
				t.Fatal(err)
			}
			assertSnapshots(t, rows, tc.want)
		})
	}
}
