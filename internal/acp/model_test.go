package acp

import (
	"context"
	"errors"
	"testing"

	sdk "github.com/coder/acp-go-sdk"
)

type fakeModelBackend struct {
	model  string
	closed bool
}

func (b *fakeModelBackend) ReportedModel() string { return b.model }
func (b *fakeModelBackend) Close() error {
	b.closed = true
	return nil
}

func TestStartPinned(t *testing.T) {
	allowed := []Backend{{ID: "first", Model: "example-a"}, {ID: "second", Model: "example-b"}}
	for _, tc := range []struct {
		name, reported string
		wantErr        bool
	}{
		{"matching", "example-a", false},
		{"missing", "", true},
		{"mismatched", "other-model", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			session := &fakeModelBackend{model: tc.reported}
			b, got, err := startPinned(context.Background(), allowed, []string{"model", "usage limit"}, func(_ context.Context, _ Backend) (*fakeModelBackend, error) {
				calls++
				return session, nil
			})
			if calls != 1 {
				t.Fatalf("backend selected %d times", calls)
			}
			if tc.wantErr {
				if !errors.Is(err, ErrModelMismatch) || !session.closed || got != nil {
					t.Fatalf("mismatch exposed a session or skipped cleanup: %v", err)
				}
			} else if err != nil || b.ID != "first" || got != session || session.closed {
				t.Fatalf("matching session failed: %v", err)
			}
		})
	}
}

func TestStartPinnedQuotaFallback(t *testing.T) {
	allowed := []Backend{{ID: "first", Model: "example-a"}, {ID: "second", Model: "example-b"}}
	session := &fakeModelBackend{model: "example-b"}
	b, got, err := startPinned(context.Background(), allowed, []string{"usage limit"}, func(_ context.Context, b Backend) (*fakeModelBackend, error) {
		if b.ID == "first" {
			return nil, &sdk.RequestError{Code: -32603, Message: "usage limit"}
		}
		return session, nil
	})
	if err != nil || b.ID != "second" || got != session || session.closed {
		t.Fatalf("fallback failed: %v", err)
	}
}
