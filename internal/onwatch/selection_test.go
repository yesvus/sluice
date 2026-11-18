package onwatch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSelectedProviders(t *testing.T) {
	const observed = `{"capturedAt":"2026-01-01T10:00:00Z","quotas":[{"name":"five_hour","utilization":25}]}`
	for _, tc := range []struct {
		name      string
		providers string
		body      string
		selected  []string
		want      bool
	}{
		{"mixed selection", `["anthropic","deepseek","both"]`, `{"anthropic":` + observed + `,"deepseek":{"balance":{}}}`, []string{"anthropic"}, true},
		{"ignore unselected malformed body", `["anthropic","codex","both"]`, `{"anthropic":` + observed + `,"codexAccounts":42}`, []string{"anthropic"}, true},
		{"strict discovery default", `["anthropic","deepseek","both"]`, `{"anthropic":` + observed + `}`, nil, false},
		{"selected provider missing from discovery", `["anthropic","both"]`, `{"anthropic":` + observed + `}`, []string{"anthropic", "codex"}, false},
		{"selected provider missing from telemetry", `["anthropic","codex","both"]`, `{"anthropic":` + observed + `}`, []string{"anthropic", "codex"}, false},
		{"default partial result rejected", `["anthropic","codex","both"]`, `{"anthropic":` + observed + `}`, nil, false},
		{"duplicate explicit selection", `["anthropic","both"]`, `{"anthropic":` + observed + `}`, []string{"anthropic", "anthropic"}, false},
		{"unsupported explicit selection", `["deepseek","both"]`, `{}`, []string{"deepseek"}, false},
		{"empty explicit selection", `["anthropic","both"]`, `{"anthropic":` + observed + `}`, []string{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/providers":
					_, _ = w.Write([]byte(`{"providers":` + tc.providers + `}`))
				case "/api/current":
					if r.URL.Query().Get("provider") != "both" {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					_, _ = w.Write([]byte(tc.body))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()
			var opts []Option
			if tc.selected != nil {
				opts = append(opts, WithProviders(tc.selected...))
			}
			rows, err := New(srv.URL, "", "", opts...).Current(context.Background())
			if tc.want {
				if err != nil || len(rows) != 1 || rows[0].Provider != "anthropic" {
					t.Fatalf("selected observation failed: %v", err)
				}
			} else if err == nil || rows != nil {
				t.Fatal("invalid or incomplete selection accepted")
			}
		})
	}
}

func TestObservationAge(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name string
		at   time.Time
		want bool
	}{
		{"recent", now.Add(-time.Minute), true},
		{"old", now.Add(-time.Hour), false},
		{"future", now.Add(time.Hour), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/providers" {
					_, _ = w.Write([]byte(`{"providers":["anthropic"]}`))
					return
				}
				_, _ = w.Write([]byte(`{"capturedAt":"` + tc.at.Format(time.RFC3339) + `","quotas":[{"name":"five_hour","utilization":20}]}`))
			}))
			defer srv.Close()
			rows, err := New(srv.URL, "", "", WithMaxAge(10*time.Minute)).Current(context.Background())
			if tc.want {
				if err != nil || len(rows) != 1 {
					t.Fatalf("recent observation rejected: %v", err)
				}
			} else if rows != nil || !errors.Is(err, errUnavailable) {
				t.Fatal("stale or future data accepted")
			}
		})
	}
}

func TestBoundedResponses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxBody+1)))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "", "").Current(context.Background())
	if err == nil || !strings.Contains(err.Error(), "oversized") {
		t.Fatalf("oversized body: %v", err)
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := New(srv.URL, "", "", WithTimeout(20*time.Millisecond)).Current(ctx)
	if err == nil || ctx.Err() != nil {
		t.Fatalf("HTTP timeout did not bound the request: %v", err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	_, err = New(srv.URL, "", "").Current(ctx2)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("context deadline lost: %v", err)
	}
}
