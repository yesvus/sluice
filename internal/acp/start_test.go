package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
)

func TestChooseAtStart(t *testing.T) {
	allowed := []Backend{{ID: "first", Model: "example-a"}, {ID: "second", Model: "example-b"}}
	for _, tc := range []struct {
		name     string
		firstErr error
		calls    []string
		want     string
		wantErr  bool
	}{
		{"first succeeds", nil, []string{"first"}, "first", false},
		{"quota fallback", &sdk.RequestError{Code: -32603, Message: "USAGE LIMIT reached"}, []string{"first", "second"}, "second", false},
		{"non quota fails closed", errors.New("model mismatch"), []string{"first"}, "", true},
		{"local quota text", errors.New("usage limit"), []string{"first"}, "", true},
		{"SDK internal error data", sdk.NewInternalError(map[string]string{"error": "usage limit"}), []string{"first"}, "", true},
		{"invalid params message", &sdk.RequestError{Code: -32602, Message: "usage limit"}, []string{"first"}, "", true},
		{"auth message", &sdk.RequestError{Code: -32000, Message: "usage limit"}, []string{"first"}, "", true},
		{"cancelled message", &sdk.RequestError{Code: -32800, Message: "usage limit"}, []string{"first"}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			b, session, err := chooseAtStart(context.Background(), allowed, []string{"usage limit", ""}, func(_ context.Context, b Backend) (string, error) {
				calls = append(calls, b.ID)
				if b.ID == "first" && tc.firstErr != nil {
					return "", tc.firstErr
				}
				return b.ID, nil
			})
			if (err != nil) != tc.wantErr || b.ID != tc.want || session != tc.want || !reflect.DeepEqual(calls, tc.calls) {
				t.Fatalf("backend=%q session=%q calls=%v err=%v", b.ID, session, calls, err)
			}
		})
	}
}

func TestChooseAtStartExhaustedAndCancelled(t *testing.T) {
	calls := 0
	start := func(_ context.Context, _ Backend) (string, error) {
		calls++
		return "", &sdk.RequestError{Code: -32603, Message: "usage limit"}
	}
	allowed := []Backend{{ID: "first"}, {ID: "second"}}
	_, _, err := chooseAtStart(context.Background(), allowed, []string{"usage limit"}, start)
	if !errors.Is(err, ErrNoBackend) || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = chooseAtStart(ctx, allowed, []string{"usage limit"}, start)
	if !errors.Is(err, context.Canceled) || calls != 2 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestWriteAudit(t *testing.T) {
	var buf bytes.Buffer
	now := time.Unix(100, 0).UTC()
	a := Audit{Profile: "example", Backend: "first", Model: "example-a", ClientSessionID: "synthetic", Start: now, End: now.Add(time.Second), Turns: 1, EndReason: "completed"}
	if err := WriteAudit(&buf, a); err != nil {
		t.Fatal(err)
	}
	if bytes.Count(buf.Bytes(), []byte("\n")) != 1 {
		t.Fatal("expected exactly one JSON line")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"profile", "backend", "model", "client_session_id", "start", "end", "turn_count", "end_reason"} {
		if _, ok := fields[k]; !ok {
			t.Fatalf("missing field %s", k)
		}
	}
	if len(fields) != 8 {
		t.Fatalf("unexpected audit fields: %v", fields)
	}
	buf.Reset()
	a.EndReason = "arbitrary raw error"
	if err := WriteAudit(&buf, a); err == nil || buf.Len() != 0 {
		t.Fatal("unbounded end reason must not be written")
	}
}

func TestUsageLimitIgnoresGenericSDKMessage(t *testing.T) {
	if usageLimit(sdk.NewInternalError(map[string]string{"error": "usage limit"}), []string{"internal error", "usage limit"}) {
		t.Fatal("generic SDK error must not trigger fallback")
	}
}
