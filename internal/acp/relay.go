package acp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/coder/acp-go-sdk"
)

type dialBackend func(context.Context, Backend) (io.ReadWriteCloser, error)

type activeSession struct {
	peer     *wire
	id       string
	response json.RawMessage
	model    string
}

func (s *activeSession) ReportedModel() string { return s.model }
func (s *activeSession) Close() error          { return s.peer.Close() }

type relay struct {
	ctx         context.Context
	front       *wire
	dial        dialBackend
	profile     string
	backends    []Backend
	patterns    []string
	backendDone chan struct{}

	mu       sync.Mutex
	init     json.RawMessage
	starting bool
	closing  bool
	prompt   bool
	session  *activeSession
	audit    Audit
}

// runRelay serves a single worker session per front connection. The dialer
// owns subprocess/config setup; the relay never accesses CLI credentials.
func runRelay(ctx context.Context, client io.ReadWriteCloser, profile string, allowed []Backend, patterns []string, dial dialBackend, audit io.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	r := &relay{ctx: ctx, dial: dial, profile: profile, backends: allowed, patterns: patterns, backendDone: make(chan struct{})}
	r.front = newWire(client, r.handle)
	r.front.activate()
	reason := "client_closed"
	select {
	case <-ctx.Done():
		reason = "cancelled"
	case <-r.front.connection.Done():
	case <-r.backendDone:
		reason = "backend_error"
	}
	cancel()
	_ = r.front.Close()
	r.mu.Lock()
	r.closing = true
	s := r.session
	entry := r.audit
	r.mu.Unlock()
	if s == nil {
		return nil
	}
	_ = s.Close()
	entry.End = time.Now().UTC()
	entry.EndReason = reason
	return WriteAudit(audit, entry)
}

func sessionID(params json.RawMessage) string {
	var p struct {
		ID string `json:"sessionId"`
	}
	_ = json.Unmarshal(params, &p)
	return p.ID
}

func (r *relay) handle(ctx context.Context, method string, params json.RawMessage) (any, *sdk.RequestError) {
	switch method {
	case "initialize":
		var init struct {
			Version int `json:"protocolVersion"`
		}
		if json.Unmarshal(params, &init) != nil || init.Version != 1 {
			return nil, sdk.NewInvalidParams(nil)
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.init != nil || r.closing {
			return nil, sdk.NewInvalidRequest(nil)
		}
		r.init = append(json.RawMessage(nil), params...)
		// Backend capabilities are not known until session/new. Advertise only
		// the pilot's common baseline, not unsupported optional capabilities.
		return json.RawMessage(`{"protocolVersion":1,"agentCapabilities":{},"agentInfo":{"name":"sluice","version":"dev"},"authMethods":[]}`), nil
	case "session/new":
		return r.newSession(ctx, params)
	case "session/prompt", "session/cancel", "session/set_model", "session/set_mode", "session/set_config_option":
	default:
		return nil, sdk.NewMethodNotFound(method)
	}

	r.mu.Lock()
	s := r.session
	if r.closing || s == nil || sessionID(params) != s.id {
		r.mu.Unlock()
		return nil, sdk.NewInvalidParams(nil)
	}
	if method == "session/prompt" {
		if r.prompt {
			r.mu.Unlock()
			return nil, sdk.NewInvalidRequest(nil)
		}
		r.prompt = true
		r.audit.Turns++
	}
	r.mu.Unlock()
	if method == "session/cancel" {
		return nil, protocolError(s.peer.notify(ctx, method, params))
	}
	if method == "session/prompt" {
		defer func() {
			r.mu.Lock()
			r.prompt = false
			r.mu.Unlock()
		}()
	}
	result, err := s.peer.call(ctx, method, params)
	return result, protocolError(err)
}

func (r *relay) newSession(ctx context.Context, params json.RawMessage) (any, *sdk.RequestError) {
	r.mu.Lock()
	if r.init == nil || r.starting || r.session != nil || r.closing {
		r.mu.Unlock()
		return nil, sdk.NewInvalidRequest(nil)
	}
	r.starting = true
	init := r.init
	r.mu.Unlock()

	started := time.Now().UTC()
	chosen, s, err := startPinned(ctx, r.backends, r.patterns, func(ctx context.Context, b Backend) (*activeSession, error) {
		return r.openSession(ctx, b, init, params)
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	r.starting = false
	if err != nil {
		if errors.Is(err, ErrModelMismatch) {
			return nil, sdk.NewInternalError(map[string]string{"reason": "model_mismatch"})
		}
		if errors.Is(err, ErrNoBackend) {
			return nil, sdk.NewInternalError(map[string]string{"reason": "no_backend_available"})
		}
		return nil, sdk.NewInternalError(nil)
	}
	if r.closing || ctx.Err() != nil || r.ctx.Err() != nil {
		_ = s.Close()
		return nil, sdk.NewRequestCancelled(nil)
	}
	r.session = s
	r.audit = Audit{Profile: r.profile, Backend: chosen.ID, Model: chosen.Model, ClientSessionID: s.id, Start: started}
	go func() {
		<-s.peer.connection.Done()
		close(r.backendDone)
	}()
	return s.response, nil
}

func (r *relay) openSession(ctx context.Context, b Backend, init, params json.RawMessage) (*activeSession, error) {
	stream, err := r.dial(r.ctx, b)
	if err != nil {
		return nil, err
	}
	var earlyCallback atomic.Bool
	peer := newWire(stream, func(ctx context.Context, method string, params json.RawMessage) (any, *sdk.RequestError) {
		r.mu.Lock()
		s := r.session
		valid := !r.closing && s != nil && s.model == b.Model && sessionID(params) == s.id
		r.mu.Unlock()
		if !valid {
			earlyCallback.Store(true)
			return nil, sdk.NewInvalidRequest(nil)
		}
		return r.callback(ctx, method, params)
	})
	peer.activate()
	keep := false
	defer func() {
		if !keep {
			_ = peer.Close()
		}
	}()
	initialized, err := peer.call(ctx, "initialize", init)
	if err != nil {
		return nil, err
	}
	var version struct {
		Version int `json:"protocolVersion"`
	}
	if json.Unmarshal(initialized, &version) != nil || version.Version != 1 {
		return nil, errInvalidSession
	}
	raw, err := peer.call(ctx, "session/new", params)
	if err != nil {
		return nil, err
	}
	projected, id, err := projectSession(raw, b.Model)
	if err != nil {
		return nil, err
	}
	if earlyCallback.Load() {
		return nil, errInvalidSession
	}
	keep = true
	return &activeSession{peer: peer, id: id, response: projected, model: b.Model}, nil
}

func (r *relay) callback(ctx context.Context, method string, params json.RawMessage) (any, *sdk.RequestError) {
	switch method {
	case "session/update":
		return nil, protocolError(r.front.notify(ctx, method, params))
	case "session/request_permission", "fs/read_text_file", "fs/write_text_file", "terminal/create", "terminal/output", "terminal/release", "terminal/wait_for_exit", "terminal/kill":
		result, err := r.front.call(ctx, method, params)
		return result, protocolError(err)
	default:
		return nil, sdk.NewMethodNotFound(method)
	}
}
