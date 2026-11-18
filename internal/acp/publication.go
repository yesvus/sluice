package acp

import (
	"encoding/json"
	"errors"
	"io"
	"sync"
)

const (
	maxStartupUpdates = 64
	maxStartupBytes   = 256 * 1024
)

var errStartupUpdates = errors.New("ACP startup notification buffer exceeded")

// The SDK serializes full newline-delimited writes. Hold only validated
// session updates until the frontend has received the session/new response.
type publicationStream struct {
	io.ReadWriteCloser
	mu        sync.Mutex
	id        string
	published bool
	pending   [][]byte
	size      int
}

func (s *publicationStream) expect(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.id = id
}

func (s *publicationStream) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.published {
		return s.ReadWriteCloser.Write(p)
	}
	var message struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(p, &message) != nil {
		return 0, errInvalidSession
	}
	if message.Method == "session/update" {
		if s.id == "" || sessionID(message.Params) != s.id {
			return 0, errInvalidSession
		}
		if len(s.pending) >= maxStartupUpdates || s.size+len(p) > maxStartupBytes {
			return 0, errStartupUpdates
		}
		s.pending = append(s.pending, append([]byte(nil), p...))
		s.size += len(p)
		return len(p), nil
	}
	n, err := s.ReadWriteCloser.Write(p)
	if err != nil {
		return n, err
	}
	if s.id != "" && message.Method == "" && sessionID(message.Result) == s.id {
		s.published = true
		for _, raw := range s.pending {
			if _, err := s.ReadWriteCloser.Write(raw); err != nil {
				return n, err
			}
		}
		s.pending = nil
		s.size = 0
	}
	return n, nil
}

type startupUpdates struct {
	mu      sync.Mutex
	items   []json.RawMessage
	size    int
	invalid bool
}
