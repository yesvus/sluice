package onwatch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCounterProviders(t *testing.T) {
	for _, tc := range []struct {
		provider string
		want     int
	}{
		{"synthetic", 2},
		{"zai", 2},
		{"openrouter", 1},
		{"minimax", 3},
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
			if err != nil || len(rows) != tc.want {
				t.Fatalf("got %d rows: %v", len(rows), err)
			}
			for _, row := range rows {
				if row.Provider != tc.provider || row.Used == nil || row.Limit == nil || *row.Limit <= 0 {
					t.Fatal("counter identity or values lost")
				}
				if tc.provider == "zai" && row.Window == "tokensLimit" && row.ResetsAt != nil {
					t.Fatal("N/A reset promoted to real timestamp")
				}
			}
			if tc.provider == "minimax" && (rows[0].AccountKey != "103" || rows[2].AccountKey != "104" || rows[1].Window != "wkly_synthetic-pool") {
				t.Fatal("MiniMax account or weekly identity lost")
			}
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
	if err != nil || len(rows) != 3 || rows[2].AccountKey != "104" {
		t.Fatalf("combined accounts: %v", err)
	}
	if _, err := decodeCombined(map[string]json.RawMessage{}, map[string]bool{"minimax": true}); !errors.Is(err, errUnavailable) {
		t.Fatal("empty combined data accepted as fresh")
	}
}
