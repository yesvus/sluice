package acp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

var ErrNoBackend = errors.New("no ACP backend available")

type Backend struct {
	ID    string
	Model string
}

type Audit struct {
	Profile         string    `json:"profile"`
	Backend         string    `json:"backend"`
	Model           string    `json:"model"`
	ClientSessionID string    `json:"client_session_id"`
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	Turns           uint64    `json:"turn_count"`
	EndReason       string    `json:"end_reason"`
}

func WriteAudit(w io.Writer, a Audit) error {
	switch a.EndReason {
	case "completed", "cancelled", "backend_error", "model_mismatch", "client_closed", "usage_limit":
	default:
		return errors.New("invalid audit end reason")
	}
	return json.NewEncoder(w).Encode(a)
}

func usageLimit(err error, patterns []string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" && strings.Contains(message, p) {
			return true
		}
	}
	return false
}

// chooseAtStart is only for session creation. start must close any failed
// backend and return an error before a session is exposed to the client.
func chooseAtStart[T any](ctx context.Context, allowed []Backend, patterns []string, start func(context.Context, Backend) (T, error)) (Backend, T, error) {
	var zero T
	for _, b := range allowed {
		if err := ctx.Err(); err != nil {
			return Backend{}, zero, err
		}
		session, err := start(ctx, b)
		if err == nil {
			return b, session, nil
		}
		if ctx.Err() != nil {
			return Backend{}, zero, ctx.Err()
		}
		if !usageLimit(err, patterns) {
			return Backend{}, zero, err
		}
	}
	return Backend{}, zero, ErrNoBackend
}
