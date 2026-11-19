package acp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"

	sdk "github.com/coder/acp-go-sdk"
)

type gatedReader struct {
	ready <-chan struct{}
	input io.Reader
}

func (r gatedReader) Read(p []byte) (int, error) {
	<-r.ready
	return r.input.Read(p)
}

type wire struct {
	connection *sdk.Connection
	stream     io.ReadWriteCloser
	ready      chan struct{}
}

func newWire(stream io.ReadWriteCloser, handler sdk.MethodHandler) *wire {
	w := &wire{stream: stream, ready: make(chan struct{})}
	w.connection = sdk.NewConnection(handler, stream, gatedReader{w.ready, stream})
	// The SDK may log malformed raw messages. Set the discard logger before
	// allowing the reader to receive any protocol traffic.
	w.connection.SetLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	return w
}

func (w *wire) activate() { close(w.ready) }

func (w *wire) Close() error { return w.stream.Close() }

func (w *wire) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	return sdk.SendRequest[json.RawMessage](w.connection, ctx, method, params)
}

func (w *wire) notify(ctx context.Context, method string, params json.RawMessage) error {
	return w.connection.SendNotification(ctx, method, params)
}

func protocolError(err error) *sdk.RequestError {
	if err == nil {
		return nil
	}
	var requestError *sdk.RequestError
	if errors.As(err, &requestError) {
		return requestError
	}
	if errors.Is(err, context.Canceled) {
		return sdk.NewRequestCancelled(nil)
	}
	return sdk.NewInternalError(nil)
}
