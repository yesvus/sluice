package onwatch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yesvus/sluice/internal/quota"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type snapshotKey struct {
	provider, account, window string
}

type snapshotWant struct {
	utilization     float64
	used, limit     float64
	hasCounters     bool
	observed, reset string
}

func assertSnapshots(t *testing.T, rows []quota.Snapshot, want map[snapshotKey]snapshotWant) {
	t.Helper()
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d", len(rows), len(want))
	}
	seen := make(map[snapshotKey]bool, len(want))
	for _, row := range rows {
		key := snapshotKey{row.Provider, row.AccountKey, row.Window}
		w, ok := want[key]
		if !ok || seen[key] {
			t.Fatalf("unexpected or duplicate row: %+v", key)
		}
		seen[key] = true
		at, err := time.Parse(time.RFC3339, w.observed)
		if err != nil || !row.ObservedAt.Equal(at) || row.Utilization != w.utilization {
			t.Fatalf("wrong observation or utilization: %+v", key)
		}
		if w.hasCounters {
			if row.Used == nil || row.Limit == nil || *row.Used != w.used || *row.Limit != w.limit {
				t.Fatalf("wrong finite counters: %+v", key)
			}
		} else if row.Used != nil || row.Limit != nil {
			t.Fatalf("unexpected absolute counters: %+v", key)
		}
		if w.reset == "" {
			if row.ResetsAt != nil {
				t.Fatalf("unexpected reset: %+v", key)
			}
		} else {
			reset, err := time.Parse(time.RFC3339, w.reset)
			if err != nil || row.ResetsAt == nil || !row.ResetsAt.Equal(reset) {
				t.Fatalf("wrong reset: %+v", key)
			}
		}
	}
}

func TestCurrentCombined(t *testing.T) {
	body := fixture(t, "combined")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "fixture-user" || pass != "fixture-password" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/prefix/health":
			w.WriteHeader(http.StatusNotFound)
		case "/prefix/api/providers":
			_, _ = w.Write([]byte(`{"providers":["anthropic","codex","opencode","antigravity","both"]}`))
		case "/prefix/api/current":
			if r.URL.Query().Get("provider") != "both" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write(body)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := New(srv.URL+"/prefix", "fixture-user", "fixture-password")
	if err := c.Health(context.Background()); err != nil {
		t.Fatal(err)
	}
	rows, err := c.Current(context.Background())
	if err != nil || len(rows) != 6 {
		t.Fatalf("got %d rows, err=%v", len(rows), err)
	}
	assertSnapshots(t, rows, map[snapshotKey]snapshotWant{
		{"anthropic", "", "five_hour"}:          {0.25, 0, 0, false, "2026-01-01T09:00:00Z", "2026-01-01T14:00:00Z"},
		{"antigravity", "", "synthetic-weekly"}: {0.4, 0, 0, false, "2026-01-01T10:00:00Z", "2026-01-08T10:00:00Z"},
		{"codex", "101", "five_hour"}:           {0.2, 0, 0, false, "2026-01-01T10:00:00Z", ""},
		{"codex", "102", "seven_day"}:           {0.8, 0, 0, false, "2026-01-01T10:00:00Z", ""},
		{"opencode", "", "five_hour"}:           {0.3, 0, 0, false, "2026-01-01T10:00:00Z", ""},
		{"opencode", "", "monthly"}:             {0.5, 5, 10, true, "2026-01-01T10:00:00Z", ""},
	})
}

func TestSingleCodexAccounts(t *testing.T) {
	body := fixture(t, "codex_accounts")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/providers":
			_, _ = w.Write([]byte(`{"providers":["codex"]}`))
		case "/api/codex/usage":
			if r.URL.Query().Get("all") != "true" {
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
		t.Fatalf("multi-account read failed: %v", err)
	}
	assertSnapshots(t, rows, map[snapshotKey]snapshotWant{
		{"codex", "101", "five_hour"}: {0.2, 0, 0, false, "2026-01-01T10:00:00Z", ""},
		{"codex", "102", "seven_day"}: {0.8, 0, 0, false, "2026-01-01T10:00:00Z", ""},
	})
}

func TestClientErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		body string
		want string
	}{
		{"401", 401, "", "HTTP 401"},
		{"bad JSON", 200, "<html>login</html>", "malformed JSON"},
		{"bad schema", 200, `{"providers":["anthropic"]}`, "schema"},
		{"redirect", 302, "", "HTTP 302"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/providers" {
					_, _ = w.Write([]byte(`{"providers":["anthropic"]}`))
					return
				}
				w.Header().Set("Location", "https://example.invalid/")
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := New(srv.URL, "", "").Current(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
		})
	}
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close()
	if err := New(srv.URL, "", "", WithTimeout(time.Second)).Health(context.Background()); err == nil {
		t.Fatal("unreachable server accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := New(srv.URL, "", "").Health(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
