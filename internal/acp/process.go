package acp

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type processStream struct {
	stdin  io.WriteCloser
	stdout io.ReadCloser
	cmd    *exec.Cmd
	cancel context.CancelFunc
	once   sync.Once
}

// startProcess owns one direct child. The runtime must provide a separate
// process/container boundary for isolation from other tasks and descendants.
func startProcess(ctx context.Context, executable string, args, env []string, dir string) (io.ReadWriteCloser, error) {
	if !filepath.IsAbs(executable) || !filepath.IsAbs(dir) || env == nil {
		return nil, errBackendLaunch
	}
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	in, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, errBackendLaunch
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		cancel()
		return nil, errBackendLaunch
	}
	if err := cmd.Start(); err != nil {
		_ = in.Close()
		_ = out.Close()
		cancel()
		return nil, errBackendLaunch
	}
	return &processStream{stdin: in, stdout: out, cmd: cmd, cancel: cancel}, nil
}

func (p *processStream) Read(b []byte) (int, error)  { return p.stdout.Read(b) }
func (p *processStream) Write(b []byte) (int, error) { return p.stdin.Write(b) }

func (p *processStream) Close() error {
	p.once.Do(func() {
		p.cancel()
		_ = p.stdin.Close()
		_ = p.stdout.Close()
		// Wait exactly once and only after the output stream has been closed.
		_ = p.cmd.Wait()
	})
	return nil
}

var errIsolationRequired = errors.New("worker-specific managed configuration isolation is not verified")
