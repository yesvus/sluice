package acp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type memoryStream struct {
	bytes.Buffer
	closed bool
}

func (s *memoryStream) Close() error { s.closed = true; return nil }

func TestPublicationOrder(t *testing.T) {
	out := &memoryStream{}
	s := &publicationStream{ReadWriteCloser: out}
	s.expect("synthetic")
	update := []byte("{\"jsonrpc\":\"2.0\",\"method\":\"session/update\",\"params\":{\"sessionId\":\"synthetic\"}}\n")
	response := []byte("{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"sessionId\":\"synthetic\"}}\n")
	if _, err := s.Write(update); err != nil || out.Len() != 0 {
		t.Fatal("update exposed before response")
	}
	if _, err := s.Write(response); err != nil || out.String() != string(response)+string(update) {
		t.Fatal("incorrect publication order")
	}
	if _, err := s.Write(update); err != nil || out.String() != string(response)+string(update)+string(update) {
		t.Fatal("published update not forwarded")
	}
}

func TestPublicationOverflow(t *testing.T) {
	out := &memoryStream{}
	s := &publicationStream{ReadWriteCloser: out}
	s.expect("synthetic")
	update := []byte(`{"method":"session/update","params":{"sessionId":"synthetic"}}`)
	for i := 0; i < maxStartupUpdates; i++ {
		if _, err := s.Write(update); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Write(update); err != errStartupUpdates || !out.closed || out.Len() != 0 {
		t.Fatal("overflow must close without leaking unpublished updates")
	}
}

func TestDynamicProjection(t *testing.T) {
	raw := json.RawMessage(`{"sessionId":"synthetic","update":{"sessionUpdate":"config_option_update","configOptions":[{"id":"model","category":"model","type":"select","currentValue":"drift","options":[{"value":"other","name":"Other"}]},{"id":"mode","currentValue":"keep"}]}}`)
	result, err := projectUpdate(raw, "pinned")
	if err != nil || strings.Contains(string(result), "other") || !strings.Contains(string(result), "drift") || !strings.Contains(string(result), "pinned") || !strings.Contains(string(result), "keep") {
		t.Fatalf("incorrect projection: %s %v", result, err)
	}
}
