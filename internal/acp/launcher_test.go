package acp

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexLauncher(t *testing.T) {
	p, err := decodeProfile(strings.NewReader(exampleProfile), "worker")
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := os.Chmod(home, 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "codex-acp")
	if err := os.WriteFile(binary, []byte("synthetic executable"), 0700); err != nil {
		t.Fatal(err)
	}
	p.Backends[0].Home = home
	p.Backends[0].Command = []string{binary}
	sentinel := filepath.Join(home, "synthetic-unrelated-file")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_CONFIG", "synthetic-override")
	t.Setenv("MODEL_PROVIDER", "synthetic-override")
	t.Setenv("OPENAI_API_KEY", "synthetic-override")
	t.Setenv("PATH", "/synthetic/untrusted")
	workdir := t.TempDir()
	calls := 0
	dial := newLauncher(p, workdir, func(_ context.Context, executable string, args, env []string, dir string) (io.ReadWriteCloser, error) {
		calls++
		if executable != binary || len(args) != 0 || dir != workdir {
			t.Fatal("unexpected launch arguments")
		}
		got := strings.Join(env, "\n")
		if strings.Contains(got, "synthetic-override") || strings.Contains(got, "/synthetic/untrusted") || !strings.Contains(got, "CODEX_HOME="+home+"\n") {
			t.Fatal("backend environment is not isolated from parent overrides")
		}
		config, err := os.ReadFile(filepath.Join(home, "config.toml"))
		if err != nil || string(config) != "model = \"example-a\"\nmodel_reasoning_effort = \"high\"\n" {
			t.Fatal("config not prepared before process start")
		}
		first, second := net.Pipe()
		_ = second.Close()
		return first, nil
	})
	child, err := dial(context.Background(), p.selection()[0])
	if err != nil || calls != 1 {
		t.Fatalf("backend not started: %v", err)
	}
	_ = child.Close()
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "unchanged" {
		t.Fatal("unrelated file changed")
	}

	if _, err := dial(context.Background(), p.selection()[1]); !errors.Is(err, errIsolationRequired) || calls != 1 {
		t.Fatal("OpenCode was not blocked before process start")
	}
	if _, err := dial(context.Background(), Backend{ID: p.Backends[0].ID, Model: "unlisted-model"}); !errors.Is(err, errBackendLaunch) || calls != 1 {
		t.Fatal("unlisted backend/model was allowed")
	}
}

func TestPrepareCodexRejectsSharedHome(t *testing.T) {
	p, err := decodeProfile(strings.NewReader(exampleProfile), "worker")
	if err != nil {
		t.Fatal(err)
	}
	b := p.Backends[0]
	b.Home = t.TempDir()
	if err := os.Chmod(b.Home, 0755); err != nil {
		t.Fatal(err)
	}
	if err := prepareCodex(b); !errors.Is(err, errPinnedConfigWrite) {
		t.Fatalf("shared home accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.Home, "config.toml")); !os.IsNotExist(err) {
		t.Fatal("shared directory changed")
	}
	b.Home = filepath.Join(t.TempDir(), "synthetic-link")
	target := t.TempDir()
	if err := os.Chmod(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, b.Home); err != nil {
		t.Fatal(err)
	}
	if err := prepareCodex(b); !errors.Is(err, errPinnedConfigWrite) {
		t.Fatalf("symlink home accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "config.toml")); !os.IsNotExist(err) {
		t.Fatal("symlink directory changed")
	}
}

func TestIsolationErrorNeverTriggersFallback(t *testing.T) {
	allowed := []Backend{{ID: "first"}, {ID: "second"}}
	count := 0
	_, _, err := chooseAtStart(context.Background(), allowed, []string{"isolation is not verified"}, func(context.Context, Backend) (string, error) {
		count++
		return "", errIsolationRequired
	})
	if !errors.Is(err, errIsolationRequired) || count != 1 {
		t.Fatal("non-quota isolation error triggered fallback")
	}
}
