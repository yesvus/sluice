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
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return data
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
	for _, row := range rows {
		if row.Utilization < 0 || row.Utilization > 1 || row.ObservedAt.IsZero() {
			t.Fatal("invalid normalized snapshot")
		}
		switch row.Provider {
		case "anthropic":
			if row.Utilization != 0.25 || row.Used != nil || row.Limit != nil || row.ObservedAt.Hour() != 9 {
				t.Fatal("anthropic percent or timestamp is wrong")
			}
		case "antigravity":
			if row.Window != "synthetic-weekly" || row.Utilization != 0.4 || row.ResetsAt == nil {
				t.Fatal("AGY bucket identity lost")
			}
		case "codex":
			if row.AccountKey != "101" && row.AccountKey != "102" {
				t.Fatal("account identity lost")
			}
		case "opencode":
			if row.Window == "five_hour" && (row.Used != nil || row.Limit != nil) {
				t.Fatal("percentage placeholders became counters")
			}
			if row.Window == "monthly" && (row.Used == nil || row.Limit == nil || *row.Used != 5 || *row.Limit != 10) {
				t.Fatal("real counters missing")
			}
		}
	}
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
	if err != nil || len(rows) != 2 || rows[0].AccountKey == rows[1].AccountKey {
		t.Fatalf("multi-account read failed: %v", err)
	}
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
