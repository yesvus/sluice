package onwatch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdditionalQuotaProviders(t *testing.T) {
	for _, tc := range []struct {
		provider string
		want     int
		counts   int
	}{
		{"gemini", 1, 0},
		{"copilot", 1, 1},
		{"cursor", 2, 1},
		{"ollama", 1, 0},
		{"commandcode", 1, 1},
		{"muse", 1, 0},
		{"mistral", 2, 1},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			body := fixture(t, tc.provider)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/providers":
					_ = json.NewEncoder(w).Encode(map[string][]string{"providers": {tc.provider}})
				case "/api/current":
					if r.URL.Query().Get("provider") != tc.provider {
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
			counts := 0
			for _, row := range rows {
				if row.Provider != tc.provider || row.Window == "" || row.ObservedAt.IsZero() {
					t.Fatal("identity or capture lost")
				}
				if row.Used != nil && row.Limit != nil {
					counts++
				}
			}
			if counts != tc.counts {
				t.Fatalf("got %d finite absolute counter pairs, want %d", counts, tc.counts)
			}
			if tc.provider == "gemini" && (rows[0].Window != "synthetic-family" || rows[0].Utilization != 0.42 || rows[0].ResetsAt == nil) {
				t.Fatal("Gemini family quota lost")
			}
			if tc.provider == "copilot" && (*rows[0].Used != 30 || *rows[0].Limit != 100) {
				t.Fatal("Copilot usage incorrectly derived")
			}
		})
	}
}

func TestUnavailableAdditionalWindows(t *testing.T) {
	for _, tc := range []struct {
		provider string
		body     string
	}{
		{"muse", `{"capturedAt":"2026-01-01T10:00:00Z","cliActive":true,"quotas":[{"name":"weekly","utilization":10,"format":"percent"}]}`},
		{"mistral", `{"capturedAt":"2026-01-01T10:00:00Z","status":"reconnect","quotas":[{"name":"weekly","utilization":10,"format":"percent"}]}`},
		{"ollama", `{"capturedAt":"2026-01-01T10:00:00Z","quotas":[{"name":"monthly","utilization":0,"format":"currency","limitUnknown":true,"limit":0}]}`},
		{"copilot", `{"capturedAt":"2026-01-01T10:00:00Z","quotas":[{"name":"unlimited","usagePercent":0,"unlimited":true,"entitlement":0,"remaining":0}]}`},
	} {
		rows, err := decodeProvider(tc.provider, json.RawMessage(tc.body), false)
		if rows != nil || !errors.Is(err, errUnavailable) {
			t.Fatalf("unavailable quota %s accepted: %v", tc.provider, err)
		}
	}
}
