package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
)

func TestRelayFakeBackend(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	frontServer, frontClient := net.Pipe()
	_ = frontServer.SetDeadline(time.Now().Add(5 * time.Second))
	_ = frontClient.SetDeadline(time.Now().Add(5 * time.Second))

	var dials atomic.Int32
	var prompts atomic.Int32
	cancelled := make(chan struct{}, 1)
	updates := make(chan json.RawMessage, 4)
	dial := func(_ context.Context, b Backend) (io.ReadWriteCloser, error) {
		dials.Add(1)
		parent, child := net.Pipe()
		_ = parent.SetDeadline(time.Now().Add(5 * time.Second))
		_ = child.SetDeadline(time.Now().Add(5 * time.Second))
		var fake *wire
		fake = newWire(child, func(ctx context.Context, method string, params json.RawMessage) (any, *sdk.RequestError) {
			switch method {
			case "initialize":
				return json.RawMessage(`{"protocolVersion":1}`), nil
			case "session/new":
				if b.ID == "first" {
					if err := fake.notify(ctx, "session/update", json.RawMessage(`{"sessionId":"rejected-session","update":{"sessionUpdate":"available_commands_update","availableCommands":[]}}`)); err != nil {
						return nil, protocolError(err)
					}
					return nil, &sdk.RequestError{Code: -32603, Message: "usage limit"}
				}
				if err := fake.notify(ctx, "session/update", json.RawMessage(`{"sessionId":"synthetic-session","update":{"sessionUpdate":"available_commands_update","availableCommands":[]}}`)); err != nil {
					return nil, protocolError(err)
				}
				return json.RawMessage(`{"sessionId":"synthetic-session","models":{"currentModelId":"example-b","availableModels":[{"modelId":"example-b","name":"B"},{"modelId":"other","name":"Other"}]}}`), nil
			case "session/set_config_option":
				return json.RawMessage(`{"configOptions":[{"id":"model","category":"model","type":"select","currentValue":"observed-drift","options":[{"value":"other","name":"Other"}]}]}`), nil
			case "session/prompt":
				if prompts.Add(1) == 2 {
					return nil, &sdk.RequestError{Code: -32603, Message: "usage limit"}
				}
				result, err := fake.call(ctx, "fs/read_text_file", json.RawMessage(`{"sessionId":"synthetic-session","path":"/synthetic.txt"}`))
				if err != nil || !strings.Contains(string(result), "synthetic file") {
					return nil, sdk.NewInternalError(nil)
				}
				err = fake.notify(ctx, "session/update", json.RawMessage(`{"sessionId":"synthetic-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"synthetic update"}}}`))
				if err != nil {
					return nil, protocolError(err)
				}
				return json.RawMessage(`{"stopReason":"end_turn"}`), nil
			case "session/cancel":
				cancelled <- struct{}{}
				return nil, nil
			default:
				return nil, sdk.NewMethodNotFound(method)
			}
		})
		fake.activate()
		go func() {
			<-fake.connection.Done()
			_ = child.Close()
		}()
		return parent, nil
	}

	client := newWire(frontClient, func(_ context.Context, method string, params json.RawMessage) (any, *sdk.RequestError) {
		switch method {
		case "fs/read_text_file":
			if sessionID(params) != "synthetic-session" {
				return nil, sdk.NewInvalidParams(nil)
			}
			return json.RawMessage(`{"content":"synthetic file"}`), nil
		case "session/update":
			updates <- append(json.RawMessage(nil), params...)
			return nil, nil
		default:
			return nil, sdk.NewMethodNotFound(method)
		}
	})
	client.activate()
	defer client.Close()

	var audit bytes.Buffer
	done := make(chan error, 1)
	go func() {
		done <- runRelay(ctx, frontServer, "synthetic-profile", []Backend{{ID: "first", Model: "example-a"}, {ID: "second", Model: "example-b"}}, []string{"usage limit"}, dial, &audit)
	}()

	if _, err := client.call(ctx, "initialize", json.RawMessage(`{"protocolVersion":1,"clientCapabilities":{"fs":{"readTextFile":true}}}`)); err != nil {
		t.Fatal(err)
	}
	session, err := client.call(ctx, "session/new", json.RawMessage(`{"cwd":"/synthetic","mcpServers":[]}`))
	if err != nil || strings.Contains(string(session), "other") || !strings.Contains(string(session), "example-b") || dials.Load() != 2 {
		t.Fatalf("session=%s dials=%d err=%v", session, dials.Load(), err)
	}
	select {
	case update := <-updates:
		if sessionID(update) != "synthetic-session" || !strings.Contains(string(update), "available_commands_update") {
			t.Fatal("wrong startup notification forwarded")
		}
	case <-ctx.Done():
		t.Fatal("startup notification lost")
	}
	config, err := client.call(ctx, "session/set_config_option", json.RawMessage(`{"sessionId":"synthetic-session","configId":"model","value":"observed-drift"}`))
	if err != nil || strings.Contains(string(config), "other") || !strings.Contains(string(config), "observed-drift") || !strings.Contains(string(config), "example-b") {
		t.Fatalf("dynamic config projection: %s %v", config, err)
	}
	prompt := json.RawMessage(`{"sessionId":"synthetic-session","prompt":[{"type":"text","text":"synthetic prompt"}]}`)
	result, err := client.call(ctx, "session/prompt", prompt)
	if err != nil || !strings.Contains(string(result), "end_turn") {
		t.Fatalf("prompt=%s err=%v", result, err)
	}
	select {
	case update := <-updates:
		if !strings.Contains(string(update), "synthetic update") {
			t.Fatal("notification content changed")
		}
	case <-ctx.Done():
		t.Fatal("notification not relayed")
	}
	if _, err := client.call(ctx, "session/prompt", prompt); err == nil || dials.Load() != 2 {
		t.Fatal("mid-session quota failure must not switch backend")
	}
	if err := client.notify(ctx, "session/cancel", json.RawMessage(`{"sessionId":"synthetic-session"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("cancel not relayed")
	}
	_ = client.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("relay did not exit")
	}
	var entry Audit
	if err := json.Unmarshal(audit.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Backend != "second" || entry.Model != "example-b" || entry.Turns != 2 || entry.ClientSessionID != "synthetic-session" || bytes.Count(audit.Bytes(), []byte("\n")) != 1 || bytes.Contains(audit.Bytes(), []byte("synthetic prompt")) {
		t.Fatal("unexpected audit")
	}
}

func TestProjectSession(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		wantErr        bool
	}{
		{"legacy", `{"sessionId":"s","models":{"currentModelId":"wanted","availableModels":[{"modelId":"other","name":"other"},{"modelId":"wanted","name":"Wanted"}]}}`, false},
		{"modern", `{"sessionId":"s","configOptions":[{"id":"model","category":"model","type":"select","currentValue":"wanted","name":"Model","options":[{"value":"other","name":"other"},{"value":"wanted","name":"Wanted"}]},{"id":"mode","type":"select","currentValue":"keep","options":[]}]}`, false},
		{"conflicting formats", `{"sessionId":"s","models":{"currentModelId":"wanted"},"configOptions":[{"id":"model","type":"select","currentValue":"other"}]}`, true},
		{"missing model", `{"sessionId":"s"}`, true},
		{"wrong model", `{"sessionId":"s","models":{"currentModelId":"other"}}`, true},
		{"missing session", `{"models":{"currentModelId":"wanted"}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projected, id, err := projectSession(json.RawMessage(tc.response), "wanted")
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.wantErr && (id != "s" || strings.Contains(string(projected), "other") || !strings.Contains(string(projected), "wanted")) {
				t.Fatalf("invalid projection: %s", projected)
			}
			if tc.name == "modern" && !strings.Contains(string(projected), "keep") {
				t.Fatal("non-model config option dropped")
			}
		})
	}
}
